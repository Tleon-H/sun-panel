package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRestoreRoundTripMatchesTheSnapshotAndLeavesTheConnectionOpen(t *testing.T) {
	base, db := restoreFixture(t)
	result := buildPackage(t, base, db, "1.3.0+leon.7")
	mutateLive(t, base, db)
	if err := Restore(context.Background(), restoreOpt(base, db, ""), bytes.NewReader(result)); err != nil {
		t.Fatal(err)
	}
	assertLive(t, base, db, false)
	if _, err := db.Exec(`insert into marks(name) values ('after-restore')`); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`select count(*) from marks where name = 'after-restore'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("connection was not left usable: %v count=%d", err, count)
	}
	if leftovers(t, base) != 0 {
		t.Fatal("restore left temporary files in the application directory")
	}
}

func TestRestoreAcceptsHistoricalManifestAndRejectsIncompatiblePackages(t *testing.T) {
	base, db := restoreFixture(t)
	raw := buildPackage(t, base, db, "1.3.0+leon.4")
	historical := rewriteZip(t, raw, func(files map[string][]byte) {
		var manifest Manifest
		if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
			t.Fatal(err)
		}
		manifest.Restore = restoreHistorical
		encoded, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		files["manifest.json"] = encoded
	})
	mutateLive(t, base, db)
	if err := Restore(context.Background(), restoreOpt(base, db, ""), bytes.NewReader(historical)); err != nil {
		t.Fatal(err)
	}
	assertLive(t, base, db, false)

	previous := buildPackage(t, base, db, "1.3.0+leon.9")
	mutateLive(t, base, db)
	if err := Restore(context.Background(), restoreOpt(base, db, ""), bytes.NewReader(previous)); err != nil {
		t.Fatal(err)
	}
	assertLive(t, base, db, false)

	mutateLive(t, base, db)
	cases := []struct {
		name string
		edit func(*Manifest)
	}{
		{"official", func(m *Manifest) {
			m.Application = "Sun-Panel"
			m.Version = "1.8.1"
			m.VersionCode = 18
		}},
		{"newer", func(m *Manifest) { m.Version = "1.3.0+leon.11" }},
		{"schema", func(m *Manifest) { m.VersionCode = 11 }},
		{"official flag", func(m *Manifest) { m.CompatibleWithOfficial181 = true }},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			bad := rewriteZip(t, raw, func(files map[string][]byte) {
				var manifest Manifest
				if err := json.Unmarshal(files["manifest.json"], &manifest); err != nil {
					t.Fatal(err)
				}
				item.edit(&manifest)
				encoded, err := json.MarshalIndent(manifest, "", "  ")
				if err != nil {
					t.Fatal(err)
				}
				files["manifest.json"] = encoded
			})
			err := Restore(context.Background(), restoreOpt(base, db, ""), bytes.NewReader(bad))
			if !errors.Is(err, ErrIncompatible) {
				t.Fatalf("err = %v", err)
			}
			assertLive(t, base, db, true)
		})
	}
}

func TestRestoreRejectsChecksumTraversalAndSymlink(t *testing.T) {
	base, db := restoreFixture(t)
	raw := buildPackage(t, base, db, "1.3.0+leon.8")
	mutateLive(t, base, db)

	tampered := rewriteZip(t, raw, func(files map[string][]byte) {
		files["uploads/keep.txt"] = []byte("TAMPERED")
	})
	if err := Restore(context.Background(), restoreOpt(base, db, ""), bytes.NewReader(tampered)); !errors.Is(err, ErrChecksum) {
		t.Fatalf("checksum err = %v", err)
	}
	assertLive(t, base, db, true)

	missing := rewriteZip(t, raw, func(files map[string][]byte) {
		delete(files, "manifest.json")
	})
	if err := Restore(context.Background(), restoreOpt(base, db, ""), bytes.NewReader(missing)); !errors.Is(err, ErrPackage) {
		t.Fatalf("missing manifest err = %v", err)
	}

	slipped := writeZip(t, map[string][]byte{"../secret.txt": []byte("OUTSIDE-MARKER-7d2b")}, nil)
	if err := Restore(context.Background(), restoreOpt(base, db, ""), bytes.NewReader(slipped)); !errors.Is(err, ErrOutside) {
		t.Fatalf("traversal err = %v", err)
	}
	linked := writeZip(t, map[string][]byte{}, func(zw *zip.Writer) {
		header := &zip.FileHeader{Name: "uploads/escape.txt"}
		header.SetMode(os.ModeSymlink | 0o777)
		writer, err := zw.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte("/tmp/secret")); err != nil {
			t.Fatal(err)
		}
	})
	if err := Restore(context.Background(), restoreOpt(base, db, ""), bytes.NewReader(linked)); !errors.Is(err, ErrOutside) {
		t.Fatalf("symlink err = %v", err)
	}
	assertLive(t, base, db, true)
	if leftovers(t, base) != 0 {
		t.Fatal("rejected restore left temporary files")
	}
}

func TestRestoreRollsBackAfterAFailedApply(t *testing.T) {
	for _, stage := range []string{"uploads", "database"} {
		t.Run(stage, func(t *testing.T) {
			base, db := restoreFixture(t)
			raw := buildPackage(t, base, db, "1.3.0+leon.8")
			mutateLive(t, base, db)
			parent := t.TempDir()
			opt := restoreOpt(base, db, parent)
			opt.FaultAfter = stage
			err := Restore(context.Background(), opt, bytes.NewReader(raw))
			if !errors.Is(err, ErrApply) || errors.Is(err, ErrRollback) {
				t.Fatalf("err = %v", err)
			}
			assertLive(t, base, db, true)
			if _, err := db.Exec(`insert into marks(name) values ('still-open')`); err != nil {
				t.Fatal(err)
			}
			if leftovers(t, base) != 0 {
				t.Fatal("rollback left temporary files in the application directory")
			}
			left, err := os.ReadDir(parent)
			if err != nil || len(left) != 0 {
				t.Fatalf("temp leftover %#v %v", left, err)
			}
		})
	}
}

func TestRestoreRejectsASymlinkConfigWithoutFollowingIt(t *testing.T) {
	base, db := restoreFixture(t)
	raw := buildPackage(t, base, db, "1.3.0+leon.8")
	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("OUTSIDE-MARKER-7d2b"), 0o600); err != nil {
		t.Fatal(err)
	}
	conf := filepath.Join(base, "conf", "conf.ini")
	if err := os.Remove(conf); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, conf); err != nil {
		t.Fatal(err)
	}
	err := Restore(context.Background(), restoreOpt(base, db, ""), bytes.NewReader(raw))
	if !errors.Is(err, ErrOutside) {
		t.Fatalf("err = %v", err)
	}
	secret, err := os.ReadFile(outside)
	if err != nil || string(secret) != "OUTSIDE-MARKER-7d2b" {
		t.Fatalf("symlink target changed: %s %v", secret, err)
	}
	info, err := os.Lstat(conf)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("config symlink was replaced: %v", err)
	}
}

func TestMySQLRestoreDoesNotWrite(t *testing.T) {
	base, db := restoreFixture(t)
	mutateLive(t, base, db)
	opt := restoreOpt(base, db, t.TempDir())
	opt.DatabaseDrive = "mysql"
	err := Restore(context.Background(), opt, bytes.NewReader([]byte("not-a-zip")))
	if !errors.Is(err, ErrNotSQLite) {
		t.Fatalf("err = %v", err)
	}
	assertLive(t, base, db, true)
}

func restoreFixture(t *testing.T) (string, *sql.DB) {
	t.Helper()
	base, db := fixture(t)
	if err := os.WriteFile(filepath.Join(base, "conf", "conf.ini"), []byte(runtimeConf()), 0o644); err != nil {
		t.Fatal(err)
	}
	return base, db
}

func runtimeConf() string {
	return "[base]\ndatabase_drive=sqlite\nsource_path=./uploads\n[sqlite]\nfile_path=./database/database.db\n"
}

func restoreOpt(base string, db *sql.DB, tempParent string) RestoreOptions {
	if tempParent == "" {
		tempParent = os.TempDir()
	}
	return RestoreOptions{
		BaseDir:       base,
		DatabaseDrive: "sqlite",
		UploadsPath:   "uploads",
		ConfFile:      filepath.Join("conf", "conf.ini"),
		CustomDir:     filepath.Join("web", "custom"),
		SQLitePath:    filepath.Join("database", "database.db"),
		Version:       "1.3.0+leon.10",
		VersionCode:   10,
		DB:            db,
		TempParent:    tempParent,
	}
}

func buildPackage(t *testing.T, base string, db *sql.DB, version string) []byte {
	t.Helper()
	opt := optionsFor(base, db, t.TempDir())
	opt.Version = version
	result, err := Build(context.Background(), opt)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(result.Dir) })
	body, err := os.ReadFile(result.ZipPath)
	if err != nil {
		t.Fatal(err)
	}
	return body
}

func mutateLive(t *testing.T, base string, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`insert into marks(name) values ('changed')`); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "uploads", "keep.txt"), []byte("CHANGED"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "uploads", "new.txt"), []byte("NEW"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "web", "custom", "index.css"), []byte("changed-css"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "conf", "conf.ini"), []byte(runtimeConf()+"# changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertLive(t *testing.T, base string, db *sql.DB, mutated bool) {
	t.Helper()
	var count int
	if err := db.QueryRow(`select count(*) from marks where name = 'seed-before'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("seed-before count=%d err=%v", count, err)
	}
	if err := db.QueryRow(`select count(*) from marks where name = 'changed'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if mutated && count != 1 {
		t.Fatalf("expected the changed row to remain, count=%d", count)
	}
	if !mutated && count != 0 {
		t.Fatalf("restore did not return the database, changed count=%d", count)
	}
	keep, err := os.ReadFile(filepath.Join(base, "uploads", "keep.txt"))
	if err != nil {
		t.Fatal(err)
	}
	newFile := filepath.Join(base, "uploads", "new.txt")
	_, newErr := os.Stat(newFile)
	css, err := os.ReadFile(filepath.Join(base, "web", "custom", "index.css"))
	if err != nil {
		t.Fatal(err)
	}
	conf, err := os.ReadFile(filepath.Join(base, "conf", "conf.ini"))
	if err != nil {
		t.Fatal(err)
	}
	if mutated {
		if string(keep) != "CHANGED" || newErr != nil || string(css) != "changed-css" || !strings.Contains(string(conf), "# changed") {
			t.Fatalf("mutated files were not preserved keep=%s css=%s conf=%s new=%v", keep, css, conf, newErr)
		}
		return
	}
	if string(keep) != "INSIDE-OK" || !os.IsNotExist(newErr) || !strings.Contains(string(css), "example") || strings.Contains(string(conf), "# changed") {
		t.Fatalf("restore mismatch keep=%s css=%s conf=%s new=%v", keep, css, conf, newErr)
	}
}

func leftovers(t *testing.T, base string) int {
	t.Helper()
	count := 0
	_ = filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err == nil && strings.Contains(d.Name(), ".leon-restore-") {
			count++
		}
		return nil
	})
	return count
}

func rewriteZip(t *testing.T, raw []byte, edit func(map[string][]byte)) []byte {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, file := range reader.File {
		body, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(body)
		body.Close()
		if err != nil {
			t.Fatal(err)
		}
		files[file.Name] = data
	}
	edit(files)
	return writeZip(t, files, nil)
}

func writeZip(t *testing.T, files map[string][]byte, extra func(*zip.Writer)) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		writer, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write(body); err != nil {
			t.Fatal(err)
		}
	}
	if extra != nil {
		extra(zw)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
