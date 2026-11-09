package backup

import (
	"archive/zip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/mattn/go-sqlite3"
	"gopkg.in/ini.v1"
)

const (
	// ConfirmOverwrite 是接口要求的再次确认值。没有这个值就不会覆盖。
	ConfirmOverwrite = "overwrite"
	// RestoreSupported 写入新导出的清单，表示这份包可以按覆盖方式恢复。
	RestoreSupported  = "overwrite"
	restoreHistorical = "not-implemented"
	manifestLimit     = 8 << 20
)

var (
	ErrConfirm      = errors.New("restore confirmation missing")
	ErrIncompatible = errors.New("backup package is incompatible")
	ErrChecksum     = errors.New("backup checksum mismatch")
	ErrPackage      = errors.New("backup package is invalid")
	ErrApply        = errors.New("restore apply failed")
	ErrRollback     = errors.New("restore rollback failed")
)

var leonDisplay = regexp.MustCompile(`^1\.3\.0\+leon\.([1-9][0-9]*)$`)

// RestoreOptions 描述一次覆盖恢复。FaultAfter 只给测试注入失败点。
type RestoreOptions struct {
	BaseDir       string
	DatabaseDrive string
	UploadsPath   string
	ConfFile      string
	CustomDir     string
	SQLitePath    string
	Version       string
	VersionCode   int
	DB            *sql.DB
	TempParent    string
	MaxFiles      int
	MaxBytes      int64
	FaultAfter    string
}

func Restore(ctx context.Context, opt RestoreOptions, archive io.Reader) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	drive := strings.ToLower(strings.TrimSpace(opt.DatabaseDrive))
	if drive == "mysql" || (drive != "" && drive != "sqlite") {
		return ErrNotSQLite
	}
	if opt.DB == nil {
		return ErrSnapshot
	}
	base, err := filepath.Abs(opt.BaseDir)
	if err != nil {
		return ErrOutside
	}
	base, err = filepath.EvalSymlinks(base)
	if err != nil {
		return ErrOutside
	}
	opt.BaseDir = base
	uploads, err := canonicalInside(base, opt.UploadsPath)
	if err != nil {
		return err
	}
	confFile, err := canonicalInside(base, opt.ConfFile)
	if err != nil {
		return err
	}
	custom, err := canonicalInside(base, opt.CustomDir)
	if err != nil {
		return err
	}
	sqliteFile, err := canonicalInside(base, opt.SQLitePath)
	if err != nil {
		return err
	}
	if err := rejectSymlink(uploads); err != nil {
		return err
	}
	if err := rejectSymlink(custom); err != nil {
		return err
	}
	if err := rejectSymlink(sqliteFile); err != nil {
		return err
	}
	if err := assertDisjoint(uploads, custom, confFile, sqliteFile); err != nil {
		return err
	}

	parent := opt.TempParent
	if parent == "" {
		parent = os.TempDir()
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return err
	}
	temp, err := os.MkdirTemp(parent, "leon-panel-restore-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(temp)
	if err := rejectTempOverlap(temp, []sourceRoot{{abs: uploads}, {abs: custom}, {abs: filepath.Dir(confFile)}, {abs: filepath.Dir(sqliteFile)}}); err != nil {
		return err
	}

	maxFiles, maxBytes := limits(Options{MaxFiles: opt.MaxFiles, MaxBytes: opt.MaxBytes})
	zipPath := filepath.Join(temp, "upload.zip")
	if err := saveLimited(archive, zipPath, maxBytes); err != nil {
		return err
	}
	extracted, err := extractVerified(zipPath, filepath.Join(temp, "extract"), maxFiles, maxBytes)
	if err != nil {
		return err
	}
	if err := AcceptPackageVersion(opt.Version, opt.VersionCode, extracted.manifest); err != nil {
		return err
	}
	if err := integrityCheckFile(extracted.sqlite); err != nil {
		return err
	}
	if err := assertConfMatches(base, extracted.conf, uploads, sqliteFile); err != nil {
		return err
	}

	rollbackDB := filepath.Join(temp, "rollback.sqlite")
	if _, err := snapshotSQLite(ctx, opt.DB, rollbackDB); err != nil {
		return err
	}

	var undos []func() error
	fail := func(cause error) error {
		if undoErr := runUndos(undos); undoErr != nil {
			return fmt.Errorf("%w: %v", ErrRollback, undoErr)
		}
		if errors.Is(cause, ErrApply) {
			return cause
		}
		return fmt.Errorf("%w: %v", ErrApply, cause)
	}

	uploadsSwap, err := swapDirectory(uploads, extracted.uploads)
	if err != nil {
		return fail(err)
	}
	undos = append(undos, uploadsSwap.rollback)
	if err := opt.fault("uploads"); err != nil {
		return fail(err)
	}
	customSwap, err := swapDirectory(custom, extracted.custom)
	if err != nil {
		return fail(err)
	}
	undos = append(undos, customSwap.rollback)
	if err := opt.fault("custom"); err != nil {
		return fail(err)
	}
	confSwap, err := swapFile(confFile, extracted.conf)
	if err != nil {
		return fail(err)
	}
	undos = append(undos, confSwap.rollback)
	if err := opt.fault("config"); err != nil {
		return fail(err)
	}
	if err := copySQLite(ctx, opt.DB, extracted.sqlite); err != nil {
		if undoErr := copySQLite(ctx, opt.DB, rollbackDB); undoErr != nil {
			_ = runUndos(undos)
			return fmt.Errorf("%w: %v", ErrRollback, undoErr)
		}
		if undoErr := runUndos(undos); undoErr != nil {
			return fmt.Errorf("%w: %v", ErrRollback, undoErr)
		}
		return fmt.Errorf("%w: %v", ErrApply, err)
	}
	undos = append(undos, func() error { return copySQLite(ctx, opt.DB, rollbackDB) })
	if err := opt.fault("database"); err != nil {
		return fail(err)
	}
	uploadsSwap.commit()
	customSwap.commit()
	confSwap.commit()
	return nil
}

func (opt RestoreOptions) fault(stage string) error {
	if opt.FaultAfter != "" && opt.FaultAfter == stage {
		return fmt.Errorf("%w: injected after %s", ErrApply, stage)
	}
	return nil
}

func runUndos(undos []func() error) error {
	var first error
	for i := len(undos) - 1; i >= 0; i-- {
		if err := undos[i](); err != nil && first == nil {
			first = err
		}
	}
	return first
}

type extractedPackage struct {
	manifest Manifest
	sqlite   string
	conf     string
	uploads  string
	custom   string
}

func extractVerified(zipPath, dest string, maxFiles int, maxBytes int64) (*extractedPackage, error) {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, ErrPackage
	}
	defer reader.Close()
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return nil, err
	}
	seen := map[string]struct{}{}
	files := map[string]hashedFile{}
	var manifestRaw []byte
	var used int64
	for _, file := range reader.File {
		if file.Flags&0x1 != 0 {
			return nil, ErrPackage
		}
		rel, isDir, err := zipRel(file.Name)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[rel]; ok {
			return nil, ErrPackage
		}
		seen[rel] = struct{}{}
		mode := file.Mode()
		if mode&os.ModeSymlink != 0 {
			return nil, ErrOutside
		}
		if mode.IsDir() {
			isDir = true
		}
		if !isDir && !mode.IsRegular() {
			return nil, ErrOutside
		}
		if rel == "manifest.json" {
			if isDir {
				return nil, ErrPackage
			}
			manifestRaw, err = readZipFile(file, manifestLimit)
			if err != nil {
				return nil, err
			}
			continue
		}
		if isDir {
			if !allowedDir(rel) {
				return nil, ErrPackage
			}
			continue
		}
		if _, _, err := allowedFile(rel); err != nil {
			return nil, err
		}
		if len(files)+1 > maxFiles || used >= maxBytes {
			return nil, ErrLimit
		}
		saved, err := writeZipFile(dest, rel, file, maxBytes-used)
		if err != nil {
			return nil, err
		}
		used += saved.size
		files[rel] = saved
	}
	if manifestRaw == nil {
		return nil, ErrPackage
	}
	var manifest Manifest
	if err := json.Unmarshal(manifestRaw, &manifest); err != nil {
		return nil, ErrPackage
	}
	if err := validateManifest(manifest, files); err != nil {
		return nil, err
	}
	result := &extractedPackage{manifest: manifest}
	for rel, saved := range files {
		switch {
		case rel == snapshotName:
			result.sqlite = saved.path
		case rel == "conf/conf.ini":
			result.conf = saved.path
		}
	}
	uploads, err := stageSubset(dest, files, "uploads/")
	if err != nil {
		return nil, err
	}
	custom, err := stageSubset(dest, files, "custom/")
	if err != nil {
		return nil, err
	}
	result.uploads = uploads
	result.custom = custom
	return result, nil
}

type hashedFile struct {
	path string
	size int64
	sum  string
}

func validateManifest(manifest Manifest, files map[string]hashedFile) error {
	if manifest.Format != 1 || manifest.Application != "Leon Panel" || manifest.CompatibleWithOfficial181 {
		return ErrIncompatible
	}
	if manifest.Restore != RestoreSupported && manifest.Restore != restoreHistorical {
		return ErrIncompatible
	}
	if manifest.VersionCode != 10 {
		return ErrIncompatible
	}
	if _, ok := leonRevision(manifest.Version); !ok {
		return ErrIncompatible
	}
	db := manifest.Database
	if db.Driver != "sqlite" || !db.Included || db.Method != "sqlite-vacuum-into" || db.Integrity != "ok" {
		return ErrIncompatible
	}
	if len(manifest.Files) == 0 {
		return ErrIncomplete
	}
	seen := map[string]struct{}{}
	for _, entry := range manifest.Files {
		if _, ok := seen[entry.Path]; ok {
			return ErrPackage
		}
		seen[entry.Path] = struct{}{}
		if _, _, err := allowedFile(entry.Path); err != nil {
			return err
		}
		saved, ok := files[entry.Path]
		if !ok {
			return ErrIncomplete
		}
		if entry.Size != saved.size || !validSHA(entry.SHA256) || !strings.EqualFold(entry.SHA256, saved.sum) {
			return ErrChecksum
		}
	}
	if len(seen) != len(files) {
		return ErrPackage
	}
	if _, ok := files[snapshotName]; !ok {
		return ErrIncomplete
	}
	if _, ok := files["conf/conf.ini"]; !ok {
		return ErrIncomplete
	}
	return nil
}

// AcceptPackageVersion 判断清单里的显示版本能不能由当前程序恢复。
func AcceptPackageVersion(current string, currentCode int, manifest Manifest) error {
	if currentCode != 10 || manifest.VersionCode != 10 {
		return ErrIncompatible
	}
	currentN, ok := leonRevision(current)
	packageN, ok2 := leonRevision(manifest.Version)
	if !ok || !ok2 || packageN > currentN {
		return ErrIncompatible
	}
	if manifest.Format != 1 || manifest.Application != "Leon Panel" || manifest.CompatibleWithOfficial181 {
		return ErrIncompatible
	}
	if manifest.Restore != RestoreSupported && manifest.Restore != restoreHistorical {
		return ErrIncompatible
	}
	return nil
}

func leonRevision(version string) (int, bool) {
	match := leonDisplay.FindStringSubmatch(version)
	if match == nil {
		return 0, false
	}
	n, err := strconv.Atoi(match[1])
	if err != nil || n < 1 {
		return 0, false
	}
	return n, true
}

func validSHA(value string) bool {
	if len(value) != 64 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func allowedFile(rel string) (string, string, error) {
	switch {
	case rel == snapshotName:
		return "database", "", nil
	case rel == "conf/conf.ini":
		return "conf", "", nil
	case strings.HasPrefix(rel, "uploads/"):
		rest := strings.TrimPrefix(rel, "uploads/")
		if rest == "" || strings.Contains(rest, "..") {
			return "", "", ErrOutside
		}
		return "uploads", rest, nil
	case strings.HasPrefix(rel, "custom/"):
		rest := strings.TrimPrefix(rel, "custom/")
		if rest == "" || strings.Contains(rest, "..") {
			return "", "", ErrOutside
		}
		return "custom", rest, nil
	default:
		if strings.Contains(rel, "..") {
			return "", "", ErrOutside
		}
		return "", "", ErrPackage
	}
}

func allowedDir(rel string) bool {
	switch rel {
	case "uploads", "custom", "database", "conf":
		return true
	}
	return strings.HasPrefix(rel, "uploads/") || strings.HasPrefix(rel, "custom/")
}

func zipRel(name string) (string, bool, error) {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") || strings.ContainsRune(name, 0) {
		return "", false, ErrOutside
	}
	isDir := strings.HasSuffix(name, "/")
	trimmed := strings.TrimSuffix(name, "/")
	if trimmed == "" {
		return "", false, ErrOutside
	}
	parts := strings.Split(trimmed, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", false, ErrOutside
		}
	}
	return trimmed, isDir, nil
}

func saveLimited(src io.Reader, dest string, max int64) error {
	file, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	n, err := io.Copy(file, io.LimitReader(src, max+1))
	if err != nil {
		return err
	}
	if n > max {
		return ErrLimit
	}
	return file.Close()
}

func readZipFile(file *zip.File, limit int64) ([]byte, error) {
	body, err := file.Open()
	if err != nil {
		return nil, ErrPackage
	}
	defer body.Close()
	data, err := io.ReadAll(io.LimitReader(body, limit+1))
	if err != nil {
		return nil, ErrPackage
	}
	if int64(len(data)) > limit {
		return nil, ErrLimit
	}
	return data, nil
}

func writeZipFile(dest, rel string, file *zip.File, budget int64) (hashedFile, error) {
	target := filepath.Join(dest, filepath.FromSlash(rel))
	if !within(dest, target) {
		return hashedFile{}, ErrOutside
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return hashedFile{}, err
	}
	body, err := file.Open()
	if err != nil {
		return hashedFile{}, ErrPackage
	}
	defer body.Close()
	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return hashedFile{}, err
	}
	defer out.Close()
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, hash), io.LimitReader(body, budget+1))
	if err != nil {
		return hashedFile{}, err
	}
	if n > budget {
		return hashedFile{}, ErrLimit
	}
	if err := out.Close(); err != nil {
		return hashedFile{}, err
	}
	return hashedFile{path: target, size: n, sum: hex.EncodeToString(hash.Sum(nil))}, nil
}

func stageSubset(dest string, files map[string]hashedFile, prefix string) (string, error) {
	stage := filepath.Join(dest, strings.TrimSuffix(prefix, "/"))
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return "", err
	}
	return stage, nil
}

func assertConfMatches(base, confPath, uploads, sqliteFile string) error {
	cfg, err := ini.Load(confPath)
	if err != nil {
		return ErrPackage
	}
	drive := strings.ToLower(strings.TrimSpace(cfg.Section("base").Key("database_drive").String()))
	if drive == "" {
		drive = "sqlite"
	}
	if drive != "sqlite" {
		return ErrIncompatible
	}
	confUploads := strings.TrimSpace(cfg.Section("base").Key("source_path").String())
	confSQLite := strings.TrimSpace(cfg.Section("sqlite").Key("file_path").String())
	if confUploads == "" || confSQLite == "" {
		return ErrIncomplete
	}
	gotUploads, err := canonicalInside(base, confUploads)
	if err != nil {
		return err
	}
	gotSQLite, err := canonicalInside(base, confSQLite)
	if err != nil {
		return err
	}
	if gotUploads != uploads || gotSQLite != sqliteFile {
		return ErrIncompatible
	}
	return nil
}

func canonicalInside(base, target string) (string, error) {
	if strings.TrimSpace(target) == "" || strings.ContainsRune(target, 0) {
		return "", ErrOutside
	}
	cleaned := filepath.Clean(target)
	abs := cleaned
	if !filepath.IsAbs(cleaned) {
		abs = filepath.Join(base, cleaned)
	}
	abs = filepath.Clean(abs)
	if !within(base, abs) || abs == filepath.Clean(base) {
		return "", ErrOutside
	}
	current := abs
	for {
		info, err := os.Lstat(current)
		if err != nil {
			if os.IsNotExist(err) {
				next := filepath.Dir(current)
				if next == current || !within(base, next) && next != filepath.Clean(base) {
					return "", ErrOutside
				}
				current = next
				if current == filepath.Clean(base) {
					return abs, nil
				}
				continue
			}
			return "", ErrOutside
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return "", ErrOutside
		}
		if current == filepath.Clean(base) {
			return abs, nil
		}
		next := filepath.Dir(current)
		if next == current {
			return "", ErrOutside
		}
		current = next
	}
}

func rejectSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return ErrOutside
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrOutside
	}
	return nil
}

func assertDisjoint(uploads, custom, confFile, sqliteFile string) error {
	if overlaps(uploads, custom) || within(uploads, confFile) || within(uploads, sqliteFile) || within(custom, confFile) || within(custom, sqliteFile) {
		return ErrOutside
	}
	return nil
}

func overlaps(a, b string) bool {
	return a == b || within(a, b) || within(b, a)
}

type dirSwap struct {
	live    string
	backup  string
	staged  string
	applied bool
}

func swapDirectory(live, stagedSrc string) (*dirSwap, error) {
	parent := filepath.Dir(live)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, err
	}
	if err := rejectSymlink(parent); err != nil {
		return nil, err
	}
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	swap := &dirSwap{
		live:   live,
		backup: filepath.Join(parent, ".leon-restore-old-"+id),
		staged: filepath.Join(parent, ".leon-restore-new-"+id),
	}
	if err := copyTree(swap.staged, stagedSrc); err != nil {
		_ = os.RemoveAll(swap.staged)
		return nil, err
	}
	if _, err := os.Lstat(live); err == nil {
		if err := rejectSymlink(live); err != nil {
			_ = os.RemoveAll(swap.staged)
			return nil, err
		}
		if err := os.Rename(live, swap.backup); err != nil {
			_ = os.RemoveAll(swap.staged)
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		_ = os.RemoveAll(swap.staged)
		return nil, err
	}
	if err := os.Rename(swap.staged, live); err != nil {
		_ = os.Rename(swap.backup, live)
		_ = os.RemoveAll(swap.staged)
		return nil, err
	}
	swap.applied = true
	return swap, nil
}

func (s *dirSwap) rollback() error {
	if s == nil || !s.applied {
		return nil
	}
	failed := s.staged + "-failed"
	if err := os.Rename(s.live, failed); err != nil {
		return err
	}
	if _, err := os.Lstat(s.backup); err == nil {
		if err := os.Rename(s.backup, s.live); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	} else if err := os.RemoveAll(s.live); err != nil && !os.IsNotExist(err) {
		_ = os.RemoveAll(failed)
		return err
	}
	s.applied = false
	return os.RemoveAll(failed)
}

func (s *dirSwap) commit() {
	if s == nil || !s.applied {
		return
	}
	_ = os.RemoveAll(s.backup)
	s.applied = false
}

type fileSwap struct {
	live    string
	backup  string
	applied bool
	hadLive bool
}

func swapFile(live, src string) (*fileSwap, error) {
	parent := filepath.Dir(live)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return nil, err
	}
	if err := rejectSymlink(parent); err != nil {
		return nil, err
	}
	id, err := randomID()
	if err != nil {
		return nil, err
	}
	swap := &fileSwap{live: live, backup: filepath.Join(parent, ".leon-restore-file-"+id)}
	staged := filepath.Join(parent, ".leon-restore-file-new-"+id)
	if err := copyRegular(staged, src); err != nil {
		_ = os.Remove(staged)
		return nil, err
	}
	if _, err := os.Lstat(live); err == nil {
		swap.hadLive = true
		if err := os.Rename(live, swap.backup); err != nil {
			_ = os.Remove(staged)
			return nil, err
		}
	} else if !os.IsNotExist(err) {
		_ = os.Remove(staged)
		return nil, err
	}
	if err := os.Rename(staged, live); err != nil {
		if swap.hadLive {
			_ = os.Rename(swap.backup, live)
		}
		_ = os.Remove(staged)
		return nil, err
	}
	swap.applied = true
	return swap, nil
}

func (s *fileSwap) rollback() error {
	if s == nil || !s.applied {
		return nil
	}
	if err := os.Remove(s.live); err != nil && !os.IsNotExist(err) {
		return err
	}
	if s.hadLive {
		if err := os.Rename(s.backup, s.live); err != nil {
			return err
		}
	}
	s.applied = false
	return nil
}

func (s *fileSwap) commit() {
	if s == nil || !s.applied {
		return
	}
	_ = os.Remove(s.backup)
	s.applied = false
}

func copyTree(dst, src string) error {
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	return filepath.WalkDir(src, func(current string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(src, current)
		if err != nil || escapes(rel) {
			return ErrOutside
		}
		target := filepath.Join(dst, rel)
		if !within(dst, target) && target != dst {
			return ErrOutside
		}
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return ErrOutside
		}
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if !info.Mode().IsRegular() {
			return ErrOutside
		}
		return copyRegular(target, current)
	})
}

func copyRegular(dst, src string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	info, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return ErrOutside
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	if err := out.Sync(); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func randomID() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func integrityCheckFile(path string) error {
	_, err := integrityCheck(path)
	return err
}

func copySQLite(ctx context.Context, dest *sql.DB, srcPath string) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	src, err := sql.Open("sqlite3", srcPath+"?mode=ro")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSnapshot, err)
	}
	defer src.Close()
	src.SetMaxOpenConns(1)
	srcConn, err := src.Conn(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSnapshot, err)
	}
	defer srcConn.Close()
	if _, err := srcConn.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		return fmt.Errorf("%w: %v", ErrSnapshot, err)
	}
	destConn, err := dest.Conn(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrSnapshot, err)
	}
	defer destConn.Close()
	if _, err := destConn.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		return fmt.Errorf("%w: %v", ErrSnapshot, err)
	}
	if err := destConn.Raw(func(destDriver any) error {
		return srcConn.Raw(func(srcDriver any) error {
			dst, ok := destDriver.(*sqlite3.SQLiteConn)
			source, ok2 := srcDriver.(*sqlite3.SQLiteConn)
			if !ok || !ok2 {
				return fmt.Errorf("%w: unexpected sqlite driver", ErrSnapshot)
			}
			bk, err := dst.Backup("main", source, "main")
			if err != nil {
				return err
			}
			finished := false
			defer func() {
				if !finished {
					_ = bk.Close()
				}
			}()
			previous := -1
			stalled := 0
			for {
				if err := ctx.Err(); err != nil {
					return err
				}
				done, err := bk.Step(64)
				if err != nil {
					return err
				}
				if done {
					finished = true
					return bk.Close()
				}
				remaining := bk.Remaining()
				if remaining == previous {
					stalled++
					if stalled > 250 {
						return fmt.Errorf("sqlite backup made no progress")
					}
					time.Sleep(20 * time.Millisecond)
					continue
				}
				stalled = 0
				previous = remaining
			}
		})
	}); err != nil {
		return fmt.Errorf("%w: %v", ErrSnapshot, err)
	}
	var integrity string
	if err := destConn.QueryRowContext(ctx, "PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		return ErrSnapshot
	}
	if _, err := destConn.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)"); err != nil {
		return fmt.Errorf("%w: %v", ErrSnapshot, err)
	}
	return nil
}
