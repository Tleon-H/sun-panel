package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

const (
	defaultMaxFiles = 20000
	defaultMaxBytes = 512 << 20
	snapshotName    = "database/database.sqlite"
)

var (
	ErrNotSQLite  = errors.New("mysql is not exported")
	ErrOutside    = errors.New("path is outside the application directory")
	ErrSnapshot   = errors.New("sqlite snapshot failed")
	ErrLimit      = errors.New("backup limit exceeded")
	ErrIncomplete = errors.New("required backup file is missing")
)

var gate sync.Mutex

func TryBegin() bool {
	return gate.TryLock()
}

func End() {
	gate.Unlock()
}

type Options struct {
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
	Now           time.Time
	MaxFiles      int
	MaxBytes      int64
}

type FileEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type Skipped struct {
	Path   string `json:"path"`
	Reason string `json:"reason"`
}

type DatabaseInfo struct {
	Driver        string `json:"driver"`
	Included      bool   `json:"included"`
	Method        string `json:"method"`
	SQLiteVersion string `json:"sqliteVersion,omitempty"`
	Integrity     string `json:"integrity,omitempty"`
}

type Manifest struct {
	Format                    int          `json:"format"`
	Application               string       `json:"application"`
	Version                   string       `json:"version"`
	VersionCode               int          `json:"versionCode"`
	CreatedAt                 string       `json:"createdAt"`
	Database                  DatabaseInfo `json:"database"`
	Restore                   string       `json:"restore"`
	CompatibleWithOfficial181 bool         `json:"compatibleWithOfficial181"`
	Warnings                  []string     `json:"warnings"`
	Files                     []FileEntry  `json:"files"`
	Skipped                   []Skipped    `json:"skipped"`
}

type Result struct {
	Dir      string
	ZipPath  string
	Manifest Manifest
}

type sourceRoot struct {
	abs      string
	prefix   string
	optional bool
	file     bool
}

func Build(ctx context.Context, opt Options) (*Result, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	drive := strings.ToLower(strings.TrimSpace(opt.DatabaseDrive))
	if drive == "mysql" || (drive != "" && drive != "sqlite") {
		return nil, ErrNotSQLite
	}
	base, err := filepath.Abs(opt.BaseDir)
	if err != nil {
		return nil, ErrOutside
	}
	base, err = filepath.EvalSymlinks(base)
	if err != nil {
		return nil, ErrOutside
	}
	roots, err := resolveRoots(base, opt)
	if err != nil {
		return nil, err
	}
	if opt.DB == nil {
		return nil, ErrSnapshot
	}

	parent := opt.TempParent
	if parent == "" {
		parent = os.TempDir()
	}
	parent, err = filepath.Abs(parent)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp(parent, "leon-panel-backup-")
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(dir)
		}
	}()
	if err := rejectTempOverlap(dir, roots); err != nil {
		return nil, err
	}

	snapPath := filepath.Join(dir, "database.sqlite")
	info, err := snapshotSQLite(ctx, opt.DB, snapPath)
	if err != nil {
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	zipPath := filepath.Join(dir, "leon-panel-backup.zip")
	zipFile, err := os.OpenFile(zipPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}
	zw := zip.NewWriter(zipFile)
	maxFiles, maxBytes := limits(opt)
	files := make([]FileEntry, 0, 16)
	skipped := make([]Skipped, 0)
	budget := &byteBudget{files: maxFiles, bytes: maxBytes}
	liveDB := liveDatabasePaths(base, opt.SQLitePath)

	entry, err := addRegularFile(zw, snapPath, snapshotName, budget)
	if err != nil {
		_ = zw.Close()
		_ = zipFile.Close()
		return nil, err
	}
	files = append(files, entry)

	packed := make([]string, 0, len(roots))
	for _, root := range roots {
		if root.abs == "" {
			if root.optional {
				skipped = append(skipped, Skipped{Path: root.prefix, Reason: "missing"})
				continue
			}
			_ = zw.Close()
			_ = zipFile.Close()
			return nil, ErrIncomplete
		}
		if !root.file && insideAny(root.abs, packed) {
			skipped = append(skipped, Skipped{Path: root.prefix, Reason: "overlap"})
			continue
		}
		if root.file {
			if skip, reason := skipLive(root.abs, liveDB); skip {
				skipped = append(skipped, Skipped{Path: root.prefix, Reason: reason})
				continue
			}
			name := path.Join(root.prefix, filepath.Base(root.abs))
			if root.prefix == "conf" {
				name = "conf/conf.ini"
			}
			entry, err := addRegularFile(zw, root.abs, name, budget)
			if err != nil {
				_ = zw.Close()
				_ = zipFile.Close()
				return nil, err
			}
			files = append(files, entry)
			continue
		}
		err = filepath.WalkDir(root.abs, func(current string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if current == root.abs {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			rel, err := filepath.Rel(root.abs, current)
			if err != nil || escapes(rel) {
				return ErrOutside
			}
			zipRel := path.Join(root.prefix, filepath.ToSlash(rel))
			if info.Mode()&os.ModeSymlink != 0 {
				skipped = append(skipped, Skipped{Path: zipRel, Reason: "symlink"})
				return nil
			}
			if insideAny(current, packed) {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if !info.Mode().IsRegular() {
				skipped = append(skipped, Skipped{Path: zipRel, Reason: "not-regular"})
				return nil
			}
			if skip, reason := skipLive(current, liveDB); skip {
				skipped = append(skipped, Skipped{Path: zipRel, Reason: reason})
				return nil
			}
			entry, err := addRegularFile(zw, current, zipRel, budget)
			if err != nil {
				return err
			}
			files = append(files, entry)
			return nil
		})
		if err != nil {
			_ = zw.Close()
			_ = zipFile.Close()
			return nil, err
		}
		packed = append(packed, root.abs)
	}

	when := opt.Now
	if when.IsZero() {
		when = time.Now()
	}
	manifest := Manifest{
		Format:                    1,
		Application:               "Leon Panel",
		Version:                   opt.Version,
		VersionCode:               opt.VersionCode,
		CreatedAt:                 when.UTC().Format(time.RFC3339),
		Database:                  info,
		Restore:                   RestoreSupported,
		CompatibleWithOfficial181: false,
		Warnings: []string{
			"此压缩包可能包含密码哈希、密钥和私人导航，只供管理员自行保存，不要发送给他人或放到公开目录。",
			"恢复会覆盖当前的数据库、上传文件、conf/conf.ini 和 web/custom，必须由管理员再次确认后才会执行。",
			"这不是官方 1.8.1 的备份包，不能恢复官方备份，也不接受比当前程序更新的数据库结构。",
			"符号链接不会被跟随。超出应用目录的路径不会打包，也不会在恢复时写回。MySQL 不会被导出或恢复。",
		},
		Files:   files,
		Skipped: skipped,
	}
	encoded, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		_ = zw.Close()
		_ = zipFile.Close()
		return nil, err
	}
	writer, err := zw.CreateHeader(&zip.FileHeader{Name: "manifest.json", Method: zip.Deflate, Flags: 0x800})
	if err != nil {
		_ = zw.Close()
		_ = zipFile.Close()
		return nil, err
	}
	if _, err := writer.Write(encoded); err != nil {
		_ = zw.Close()
		_ = zipFile.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		_ = zipFile.Close()
		return nil, err
	}
	if err := zipFile.Close(); err != nil {
		return nil, err
	}
	success = true
	return &Result{Dir: dir, ZipPath: zipPath, Manifest: manifest}, nil
}

func limits(opt Options) (int, int64) {
	files := opt.MaxFiles
	if files <= 0 {
		files = defaultMaxFiles
	}
	bytes := opt.MaxBytes
	if bytes <= 0 {
		bytes = defaultMaxBytes
	}
	return files, bytes
}

type byteBudget struct {
	files int
	bytes int64
	used  int
	size  int64
}

func addRegularFile(zw *zip.Writer, source, name string, budget *byteBudget) (FileEntry, error) {
	if strings.HasPrefix(name, "/") || strings.Contains(name, "../") || name == ".." || strings.Contains(name, "\\") {
		return FileEntry{}, ErrOutside
	}
	info, err := os.Lstat(source)
	if err != nil {
		return FileEntry{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return FileEntry{}, ErrOutside
	}
	if budget.used+1 > budget.files || budget.size+info.Size() > budget.bytes {
		return FileEntry{}, ErrLimit
	}
	file, err := os.Open(source)
	if err != nil {
		return FileEntry{}, err
	}
	defer file.Close()
	header, err := zip.FileInfoHeader(info)
	if err != nil {
		return FileEntry{}, err
	}
	header.Name = name
	header.Method = zip.Deflate
	header.Flags |= 0x800
	writer, err := zw.CreateHeader(header)
	if err != nil {
		return FileEntry{}, err
	}
	hash := sha256.New()
	written, err := io.Copy(io.MultiWriter(writer, hash), file)
	if err != nil {
		return FileEntry{}, err
	}
	if written != info.Size() {
		return FileEntry{}, fmt.Errorf("%w: size changed while reading", ErrSnapshot)
	}
	budget.used++
	budget.size += written
	return FileEntry{Path: name, Size: written, SHA256: hex.EncodeToString(hash.Sum(nil))}, nil
}

func resolveRoots(base string, opt Options) ([]sourceRoot, error) {
	uploads, err := resolveOne(base, opt.UploadsPath, true)
	if err != nil {
		return nil, err
	}
	conf, err := resolveOne(base, opt.ConfFile, false)
	if err != nil {
		return nil, err
	}
	custom, err := resolveOne(base, opt.CustomDir, true)
	if err != nil {
		return nil, err
	}
	if uploads == base || custom == base || conf == base {
		return nil, ErrOutside
	}
	return []sourceRoot{
		{abs: uploads, prefix: "uploads", optional: true},
		{abs: conf, prefix: "conf", file: true},
		{abs: custom, prefix: "custom", optional: true},
	}, nil
}

func resolveOne(base, target string, optional bool) (string, error) {
	if strings.TrimSpace(target) == "" {
		if optional {
			return "", nil
		}
		return "", ErrIncomplete
	}
	if strings.Contains(target, "\x00") {
		return "", ErrOutside
	}
	cleaned := filepath.Clean(target)
	var abs string
	if filepath.IsAbs(cleaned) {
		abs = cleaned
	} else {
		abs = filepath.Join(base, cleaned)
	}
	abs = filepath.Clean(abs)
	if !within(base, abs) {
		return "", ErrOutside
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		if os.IsNotExist(err) {
			if optional {
				return "", nil
			}
			return "", ErrIncomplete
		}
		return "", ErrOutside
	}
	if !within(base, resolved) {
		return "", ErrOutside
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", ErrOutside
	}
	if info.Mode()&os.ModeSymlink != 0 {
		// 只接受解析后仍在应用目录内的根目录。子项里的符号链接在遍历时跳过。
		return resolved, nil
	}
	return abs, nil
}

func within(base, target string) bool {
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(target))
	if err != nil {
		return false
	}
	return !escapes(rel)
}

func escapes(rel string) bool {
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return true
	}
	return strings.HasPrefix(filepath.ToSlash(rel), "../")
}

func rejectTempOverlap(temp string, roots []sourceRoot) error {
	for _, root := range roots {
		if root.abs == "" {
			continue
		}
		if within(root.abs, temp) || within(temp, root.abs) {
			return ErrOutside
		}
	}
	return nil
}

func insideAny(target string, dirs []string) bool {
	for _, dir := range dirs {
		if target != dir && within(dir, target) {
			return true
		}
	}
	return false
}

func liveDatabasePaths(base, configured string) map[string]struct{} {
	paths := map[string]struct{}{}
	if strings.TrimSpace(configured) == "" || strings.Contains(configured, "\x00") {
		return paths
	}
	cleaned := filepath.Clean(configured)
	abs := cleaned
	if !filepath.IsAbs(cleaned) {
		abs = filepath.Join(base, cleaned)
	}
	abs = filepath.Clean(abs)
	if !within(base, abs) {
		return paths
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		paths[abs+suffix] = struct{}{}
	}
	return paths
}

func skipLive(path string, live map[string]struct{}) (bool, string) {
	if _, ok := live[filepath.Clean(path)]; ok {
		return true, "live-database"
	}
	return false, ""
}

func snapshotSQLite(ctx context.Context, db *sql.DB, dest string) (DatabaseInfo, error) {
	info := DatabaseInfo{Driver: "sqlite", Included: false, Method: "sqlite-vacuum-into"}
	conn, err := db.Conn(ctx)
	if err != nil {
		return info, fmt.Errorf("%w: %v", ErrSnapshot, err)
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, "PRAGMA busy_timeout = 5000"); err != nil {
		return info, fmt.Errorf("%w: %v", ErrSnapshot, err)
	}
	var version string
	if err := conn.QueryRowContext(ctx, "select sqlite_version()").Scan(&version); err != nil {
		return info, fmt.Errorf("%w: %v", ErrSnapshot, err)
	}
	info.SQLiteVersion = version
	if !sqliteSupportsVacuumInto(version) {
		return info, ErrSnapshot
	}
	if err := vacuumInto(ctx, conn, dest); err != nil {
		return info, fmt.Errorf("%w: %v", ErrSnapshot, err)
	}
	integrity, err := integrityCheck(dest)
	if err != nil {
		return info, err
	}
	info.Included = true
	info.Integrity = integrity
	return info, nil
}

func vacuumInto(ctx context.Context, conn *sql.Conn, dest string) error {
	if strings.ContainsRune(dest, '\x00') {
		return ErrSnapshot
	}
	if _, err := conn.ExecContext(ctx, "VACUUM INTO ?", dest); err == nil {
		return nil
	} else {
		quoted := strings.ReplaceAll(dest, "'", "''")
		if _, err2 := conn.ExecContext(ctx, "VACUUM INTO '"+quoted+"'"); err2 == nil {
			return nil
		}
		return err
	}
}

func integrityCheck(dest string) (string, error) {
	check, err := sql.Open("sqlite3", dest+"?mode=ro")
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrSnapshot, err)
	}
	defer check.Close()
	rows, err := check.Query("PRAGMA integrity_check")
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrSnapshot, err)
	}
	defer rows.Close()
	var messages []string
	for rows.Next() {
		var message string
		if err := rows.Scan(&message); err != nil {
			return "", fmt.Errorf("%w: %v", ErrSnapshot, err)
		}
		messages = append(messages, message)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("%w: %v", ErrSnapshot, err)
	}
	if len(messages) != 1 || messages[0] != "ok" {
		return "", ErrSnapshot
	}
	return "ok", nil
}

func sqliteSupportsVacuumInto(version string) bool {
	parts := strings.Split(version, ".")
	if len(parts) < 2 {
		return false
	}
	major, err1 := atoi(parts[0])
	minor, err2 := atoi(parts[1])
	if err1 != nil || err2 != nil {
		return false
	}
	return major > 3 || (major == 3 && minor >= 27)
}

func atoi(value string) (int, error) {
	n := 0
	if value == "" {
		return 0, fmt.Errorf("empty")
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("nan")
		}
		n = n*10 + int(r-'0')
	}
	return n, nil
}
