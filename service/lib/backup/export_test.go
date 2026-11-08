package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSQLiteSnapshotIsConsistentAndLeavesTheServerOpen(t *testing.T) {
	base, db := fixture(t)
	defer db.Close()
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		t.Fatal(err)
	}
	tempParent := t.TempDir()
	ctx := context.Background()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 20; i++ {
			_, _ = db.Exec(`insert into marks(name) values (?)`, "during")
		}
	}()
	result, err := Build(ctx, optionsFor(base, db, tempParent))
	wg.Wait()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(result.Dir) })
	if !strings.HasPrefix(result.ZipPath, tempParent) {
		t.Fatalf("archive %s is not in the temp directory", result.ZipPath)
	}
	if strings.Contains(result.ZipPath, filepath.Join(base, "uploads")) {
		t.Fatal("archive was written into uploads")
	}
	if result.Manifest.Database.Method != "sqlite-vacuum-into" || result.Manifest.Database.Integrity != "ok" || !result.Manifest.Database.Included {
		t.Fatalf("database metadata %#v", result.Manifest.Database)
	}
	if result.Manifest.Restore != "not-implemented" || result.Manifest.CompatibleWithOfficial181 {
		t.Fatalf("manifest claims restore or official compatibility: %#v", result.Manifest)
	}
	if result.Manifest.Version != "1.3.0+leon.4" || result.Manifest.VersionCode != 10 {
		t.Fatalf("version %#v", result.Manifest)
	}
	entries := unzip(t, result.ZipPath)
	for name, body := range entries {
		if strings.Contains(name, "..") || strings.HasSuffix(name, "-wal") || strings.HasSuffix(name, "-shm") {
			t.Fatalf("unexpected archive entry %s", name)
		}
		if strings.Contains(string(body), "OUTSIDE-MARKER-7d2b") {
			t.Fatalf("symlink target was packed in %s", name)
		}
	}
	if !bytesContain(entries["uploads/keep.txt"], "INSIDE-OK") {
		t.Fatal("upload file missing")
	}
	if !bytesContain(entries["conf/conf.ini"], "database_drive=sqlite") {
		t.Fatal("config missing")
	}
	if !bytesContain(entries["custom/index.css"], "example") {
		t.Fatal("custom file missing")
	}
	for _, file := range result.Manifest.Files {
		sum := sha256.Sum256(entries[file.Path])
		if hex.EncodeToString(sum[:]) != file.SHA256 || int64(len(entries[file.Path])) != file.Size {
			t.Fatalf("checksum mismatch for %s", file.Path)
		}
	}
	var skippedSymlink bool
	for _, item := range result.Manifest.Skipped {
		if item.Reason == "symlink" {
			skippedSymlink = true
		}
	}
	if !skippedSymlink {
		t.Fatalf("skipped %#v", result.Manifest.Skipped)
	}
	snap := extract(t, entries[snapshotName])
	assertSnapshot(t, snap, true, false)
	if _, err := db.Exec(`insert into marks(name) values ('seed-after')`); err != nil {
		t.Fatal(err)
	}
	assertSnapshot(t, snap, true, false)
	var count int
	if err := db.QueryRow(`select count(*) from marks where name = 'seed-after'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("source database was not left usable: %v count=%d", err, count)
	}
}

func TestSnapshotReadsCommittedRowsThatAreStillOnlyInTheWAL(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "live.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA wal_autocheckpoint=0`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`create table marks(name text)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`insert into marks(name) values ('before-wal')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`insert into marks(name) values ('wal-committed')`); err != nil {
		t.Fatal(err)
	}
	walInfo, err := os.Stat(dbPath + "-wal")
	if err != nil || walInfo.Size() == 0 {
		t.Fatalf("committed row was not left in the wal: %v", err)
	}
	mainOnly := filepath.Join(t.TempDir(), "main-only.db")
	raw, err := os.ReadFile(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainOnly, raw, 0o600); err != nil {
		t.Fatal(err)
	}
	alone, err := sql.Open("sqlite3", mainOnly+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	var missing int
	if err := alone.QueryRow(`select count(*) from marks where name = 'wal-committed'`).Scan(&missing); err != nil {
		alone.Close()
		t.Fatal(err)
	}
	alone.Close()
	if missing != 0 {
		t.Fatal("copying the main database file already contained the uncheckpointed row")
	}

	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "uploads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "conf"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "conf", "conf.ini"), []byte("database_drive=sqlite\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := Build(context.Background(), optionsFor(base, db, t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(result.Dir) })
	entries := unzip(t, result.ZipPath)
	for name := range entries {
		if strings.HasSuffix(name, "-wal") || strings.HasSuffix(name, "-shm") {
			t.Fatalf("live wal was packed: %s", name)
		}
	}
	snap := extract(t, entries[snapshotName])
	isolated, err := sql.Open("sqlite3", snap+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer isolated.Close()
	var integrity string
	if err := isolated.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity %q %v", integrity, err)
	}
	var count int
	if err := isolated.QueryRow(`select count(*) from marks where name = 'wal-committed'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("snapshot missed the committed wal row: count=%d err=%v", count, err)
	}
	if err := isolated.QueryRow(`select count(*) from marks where name = 'before-wal'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("snapshot missed the checkpointed row: count=%d err=%v", count, err)
	}
}

func TestMySQLDoesNotCreateAnArchive(t *testing.T) {
	base, db := fixture(t)
	defer db.Close()
	parent := t.TempDir()
	opt := optionsFor(base, db, parent)
	opt.DatabaseDrive = "mysql"
	if _, err := Build(context.Background(), opt); !errors.Is(err, ErrNotSQLite) {
		t.Fatalf("err = %v", err)
	}
	left, err := os.ReadDir(parent)
	if err != nil || len(left) != 0 {
		t.Fatalf("temp leftover %#v %v", left, err)
	}
}

func TestRejectsTraversalSymlinkRootsAndCleansFailedSnapshots(t *testing.T) {
	base, db := fixture(t)
	db.Close()
	parent := t.TempDir()
	opt := optionsFor(base, db, parent)
	if _, err := Build(context.Background(), opt); !errors.Is(err, ErrSnapshot) {
		t.Fatalf("closed database err = %v", err)
	}
	if left, _ := os.ReadDir(parent); len(left) != 0 {
		t.Fatalf("failed snapshot left %#v", left)
	}

	opt.DB = nil
	opt.UploadsPath = filepath.Join("uploads", "..", "..", "etc")
	if _, err := Build(context.Background(), opt); !errors.Is(err, ErrOutside) {
		t.Fatalf("traversal err = %v", err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("OUTSIDE-MARKER-7d2b"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(base, "linked-uploads")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	opt.UploadsPath = "linked-uploads"
	if _, err := Build(context.Background(), opt); !errors.Is(err, ErrOutside) {
		t.Fatalf("symlink root err = %v", err)
	}

	opt = optionsFor(base, nil, parent)
	opt.TempParent = filepath.Join(base, "uploads")
	opt.MaxBytes = 1
	live, err := sql.Open("sqlite3", filepath.Join(base, "database", "database.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer live.Close()
	opt.DB = live
	if _, err := Build(context.Background(), opt); !errors.Is(err, ErrOutside) && !errors.Is(err, ErrLimit) {
		t.Fatalf("nested temp err = %v", err)
	}
	matches, _ := filepath.Glob(filepath.Join(base, "uploads", "leon-panel-backup-*"))
	if len(matches) != 0 {
		t.Fatalf("nested temp remained: %#v", matches)
	}
}

func TestLimitAndMissingConfigDoNotLeaveTempFiles(t *testing.T) {
	base, db := fixture(t)
	defer db.Close()
	parent := t.TempDir()
	opt := optionsFor(base, db, parent)
	opt.MaxBytes = 1
	if _, err := Build(context.Background(), opt); !errors.Is(err, ErrLimit) {
		t.Fatalf("limit err = %v", err)
	}
	if left, _ := os.ReadDir(parent); len(left) != 0 {
		t.Fatalf("limit leftover %#v", left)
	}
	if err := os.Remove(filepath.Join(base, "conf", "conf.ini")); err != nil {
		t.Fatal(err)
	}
	opt.MaxBytes = 0
	if _, err := Build(context.Background(), opt); !errors.Is(err, ErrIncomplete) {
		t.Fatalf("missing config err = %v", err)
	}
}

func TestEscapesRelativePaths(t *testing.T) {
	if !escapes("..") || !escapes("../etc/passwd") {
		t.Fatal("traversal was accepted")
	}
	if escapes("keep.txt") || escapes("dir/keep.txt") {
		t.Fatal("normal relative path was rejected")
	}
}

func fixture(t *testing.T) (string, *sql.DB) {
	t.Helper()
	base := t.TempDir()
	uploads := filepath.Join(base, "uploads")
	if err := os.MkdirAll(uploads, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(uploads, "keep.txt"), []byte("INSIDE-OK"), 0o644); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("OUTSIDE-MARKER-7d2b"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(outside, "secret.txt"), filepath.Join(uploads, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(uploads, "escape-dir")); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "conf"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "conf", "conf.ini"), []byte("database_drive=sqlite\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	custom := filepath.Join(base, "web", "custom")
	if err := os.MkdirAll(custom, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(custom, "index.css"), []byte("/* example */"), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(base, "database", "database.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`create table marks(name text)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`insert into marks(name) values ('seed-before')`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return base, db
}

func optionsFor(base string, db *sql.DB, tempParent string) Options {
	return Options{
		BaseDir:       base,
		DatabaseDrive: "sqlite",
		UploadsPath:   "uploads",
		ConfFile:      filepath.Join("conf", "conf.ini"),
		CustomDir:     filepath.Join("web", "custom"),
		SQLitePath:    filepath.Join("database", "database.db"),
		Version:       "1.3.0+leon.4",
		VersionCode:   10,
		DB:            db,
		TempParent:    tempParent,
		Now:           time.Date(2026, 11, 7, 17, 0, 0, 0, time.UTC),
	}
}

func unzip(t *testing.T, zipPath string) map[string][]byte {
	t.Helper()
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	entries := map[string][]byte{}
	for _, file := range reader.File {
		if strings.Contains(file.Name, "..") {
			t.Fatalf("zip slip %s", file.Name)
		}
		body, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(body)
		body.Close()
		if err != nil {
			t.Fatal(err)
		}
		entries[file.Name] = data
	}
	return entries
}

func extract(t *testing.T, body []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "database.sqlite")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertSnapshot(t *testing.T, path string, hasBefore, hasAfter bool) {
	t.Helper()
	db, err := sql.Open("sqlite3", path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity %q %v", integrity, err)
	}
	var count int
	if err := db.QueryRow(`select count(*) from marks where name = 'seed-before'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("seed-before count=%d err=%v", count, err)
	}
	if err := db.QueryRow(`select count(*) from marks where name = 'seed-after'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if hasAfter && count != 1 {
		t.Fatalf("expected later row, count=%d", count)
	}
	if !hasAfter && count != 0 {
		t.Fatalf("snapshot changed after export, count=%d", count)
	}
	if !hasBefore {
		t.Fatal("unused")
	}
}

func bytesContain(body []byte, text string) bool {
	return strings.Contains(string(body), text)
}
