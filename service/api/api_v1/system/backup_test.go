package system

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sun-panel/global"
	"sun-panel/lib/backup"
	"sun-panel/lib/iniConfig"
	"sun-panel/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type legacyBackupGroup struct {
	models.BaseModel
	Icon        string
	Title       string
	Description string
	Sort        int
	UserId      uint
}

func (legacyBackupGroup) TableName() string { return "item_icon_group" }

func TestExportMessageUsesTheActualFailure(t *testing.T) {
	cases := []struct {
		err      error
		contains string
		absent   string
	}{
		{backup.ErrNotSQLite, "MySQL", "数据库快照失败"},
		{backup.ErrOutside, "超出应用目录", "数据库快照失败"},
		{backup.ErrIncomplete, "缺少导出所需的配置或文件", "数据库快照失败"},
		{backup.ErrLimit, "超过导出上限", "数据库快照失败"},
		{os.ErrPermission, "没有读取导出内容的权限", "数据库快照失败"},
		{backup.ErrSnapshot, "数据库快照失败", "缺少导出所需的配置"},
		{os.ErrClosed, "备份导出失败", "数据库快照失败"},
	}
	for _, item := range cases {
		message := exportMessage(item.err)
		if !strings.Contains(message, item.contains) || strings.Contains(message, item.absent) {
			t.Fatalf("%v => %s", item.err, message)
		}
	}

	gin.SetMode(gin.TestMode)
	base := t.TempDir()
	dbPath := filepath.Join(base, "database", "database.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	gormDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	previousDB := global.Db
	global.Db = gormDB
	t.Cleanup(func() { global.Db = previousDB })
	restore := useConfig(t, "[base]\ndatabase_drive=sqlite\nsource_path=./uploads\n[sqlite]\nfile_path=./database/database.db\n")
	defer restore()
	admin := models.User{BaseModel: models.BaseModel{ID: 1}, Role: 1}

	missing := callExportAt(t, &BackupApi{}, &admin, `{"path":"/etc/passwd"}`, base)
	if missing.code == 0 || !strings.Contains(missing.body, "缺少导出所需的配置或文件") || strings.Contains(missing.body, "数据库快照失败") || strings.HasPrefix(missing.body, "PK") {
		t.Fatalf("missing config %#v", missing.body)
	}

	if err := os.MkdirAll(filepath.Join(base, "conf"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "conf", "conf.ini"), []byte("database_drive=sqlite\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatal(err)
	}
	snapshot := callExportAt(t, &BackupApi{}, &admin, `{"path":"/etc/passwd"}`, base)
	if snapshot.code == 0 || !strings.Contains(snapshot.body, "数据库快照失败") || strings.Contains(snapshot.body, "缺少导出所需的配置") || strings.HasPrefix(snapshot.body, "PK") {
		t.Fatalf("snapshot %#v", snapshot.body)
	}
}

func TestBackupExportRejectsVisitorRegularUserAndMySQL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := BackupApi{}

	visitor := callExport(t, &api, nil, `{"path":"/etc/passwd"}`)
	if visitor.code != 1000 || strings.Contains(visitor.body, "PK") {
		t.Fatalf("visitor %#v", visitor.body)
	}

	regular := models.User{BaseModel: models.BaseModel{ID: 2}, Role: 2}
	denied := callExport(t, &api, &regular, `{"path":"/etc/passwd","source_path":"/etc"}`)
	if denied.code != 1005 {
		t.Fatalf("regular user %#v", denied.body)
	}

	if !backup.TryBegin() {
		t.Fatal("lock was already held")
	}
	admin := models.User{BaseModel: models.BaseModel{ID: 1}, Role: 1}
	busy := callExport(t, &api, &admin, `{"path":"/etc/passwd"}`)
	backup.End()
	if busy.code == 0 || !strings.Contains(busy.body, "已有备份正在导出") {
		t.Fatalf("busy %#v", busy.body)
	}

	restore := useConfig(t, "[base]\ndatabase_drive=mysql\nsource_path=./uploads\n")
	defer restore()
	mysql := callExport(t, &api, &admin, `{"path":"/etc/passwd"}`)
	if mysql.code == 0 || !strings.Contains(mysql.body, "MySQL") || strings.HasPrefix(mysql.body, "PK") {
		t.Fatalf("mysql %#v", mysql.body)
	}
}

func TestBackupExportIgnoresClientPathAndServesASnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "uploads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "uploads", "example-icon.png"), []byte("example-icon"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "conf"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "conf", "conf.ini"), []byte("database_drive=sqlite\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(base, "database", "database.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	gormDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := sqlDB.Ping(); err != nil {
		t.Fatal(err)
	}
	previousDB := global.Db
	global.Db = gormDB
	t.Cleanup(func() { global.Db = previousDB })
	restore := useConfig(t, "[base]\ndatabase_drive=sqlite\nsource_path=./uploads\n[sqlite]\nfile_path=./database/database.db\n")
	defer restore()

	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("OUTSIDE-MARKER-7d2b"), 0o600); err != nil {
		t.Fatal(err)
	}
	admin := models.User{BaseModel: models.BaseModel{ID: 1}, Role: 1}
	before, _ := filepath.Glob(filepath.Join(os.TempDir(), "leon-panel-backup-*"))
	body := `{"path":"` + outside + `","source_path":"/etc","sqlite":"/etc/passwd"}`
	response := callExportAt(t, &BackupApi{}, &admin, body, base)
	if response.status != 200 || !strings.HasPrefix(response.body, "PK") {
		t.Fatalf("status %d body %s", response.status, response.body)
	}
	if response.contentType != "application/zip" || !strings.Contains(response.disposition, `filename="leon-panel-backup.zip"`) {
		t.Fatalf("headers type=%s disposition=%s", response.contentType, response.disposition)
	}
	reader, err := zip.NewReader(bytes.NewReader([]byte(response.body)), int64(len(response.body)))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, file := range reader.File {
		if strings.Contains(file.Name, "..") || strings.Contains(file.Name, "passwd") {
			t.Fatalf("client path entered the archive: %s", file.Name)
		}
		opened, err := file.Open()
		if err != nil {
			t.Fatal(err)
		}
		payload, err := io.ReadAll(opened)
		opened.Close()
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(payload), "OUTSIDE-MARKER-7d2b") {
			t.Fatalf("outside file %s was packed", file.Name)
		}
		if file.Name == "uploads/example-icon.png" && string(payload) == "example-icon" {
			found = true
		}
		if file.Name == snapshotName() {
			checkSnapshotOpens(t, payload)
		}
	}
	if !found {
		t.Fatal("example upload was not in the archive")
	}
	after, _ := filepath.Glob(filepath.Join(os.TempDir(), "leon-panel-backup-*"))
	if len(after) > len(before) {
		t.Fatalf("temp archive was not removed: before %d after %d", len(before), len(after))
	}
}

func snapshotName() string {
	return "database/database.sqlite"
}

func checkSnapshotOpens(t *testing.T, payload []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snapshot.sqlite")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite3", path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("snapshot integrity %q %v", integrity, err)
	}
}

type exportResponse struct {
	status      int
	code        float64
	body        string
	contentType string
	disposition string
}

func callExport(t *testing.T, api *BackupApi, user *models.User, body string) exportResponse {
	t.Helper()
	return callExportAt(t, api, user, body, "")
}

func callExportAt(t *testing.T, api *BackupApi, user *models.User, body, baseDir string) exportResponse {
	t.Helper()
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/backup/export", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	if user != nil {
		ctx.Set("userInfo", *user)
	}
	if baseDir == "" {
		api.Export(ctx)
	} else {
		api.writeExport(ctx, baseDir, "1.3.0+leon.4", 10)
	}
	response := exportResponse{
		status:      recorder.Code,
		body:        recorder.Body.String(),
		contentType: recorder.Header().Get("Content-Type"),
		disposition: recorder.Header().Get("Content-Disposition"),
	}
	if strings.HasPrefix(response.body, "{") {
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(response.body), &payload); err == nil {
			if code, ok := payload["code"].(float64); ok {
				response.code = code
			}
		}
	}
	return response
}

func TestBackupRestoreRequiresAdminConfirmationAndRejectsBadPackages(t *testing.T) {
	gin.SetMode(gin.TestMode)
	api := BackupApi{}
	visitor := callRestore(t, &api, nil, "", nil, "")
	if visitor.code != 1000 {
		t.Fatalf("visitor %#v", visitor.body)
	}
	regular := models.User{BaseModel: models.BaseModel{ID: 2}, Role: 2}
	denied := callRestore(t, &api, &regular, backup.ConfirmOverwrite, []byte("nope"), "")
	if denied.code != 1005 {
		t.Fatalf("regular %#v", denied.body)
	}

	base, db, sqlDB := restoreService(t)
	defer sqlDB.Close()
	archive := servicePackage(t, base, sqlDB)
	if _, err := db.Exec(`insert into marks(name) values ('changed')`); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "uploads", "keep.txt"), []byte("CHANGED"), 0o644); err != nil {
		t.Fatal(err)
	}
	admin := models.User{BaseModel: models.BaseModel{ID: 1}, Role: 1}
	unconfirmed := callRestore(t, &api, &admin, "", archive, base)
	if unconfirmed.code == 0 || !strings.Contains(unconfirmed.body, "再次确认") {
		t.Fatalf("unconfirmed %#v", unconfirmed.body)
	}
	var count int
	if err := db.QueryRow(`select count(*) from marks where name = 'changed'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("unconfirmed restore changed the database: %v count=%d", err, count)
	}
	body, err := os.ReadFile(filepath.Join(base, "uploads", "keep.txt"))
	if err != nil || string(body) != "CHANGED" {
		t.Fatalf("unconfirmed restore changed uploads: %s %v", body, err)
	}

	outside := filepath.Join(t.TempDir(), "secret.txt")
	if err := os.WriteFile(outside, []byte("OUTSIDE-MARKER-7d2b"), 0o600); err != nil {
		t.Fatal(err)
	}
	bad := callRestore(t, &api, &admin, backup.ConfirmOverwrite, []byte("not-a-backup"), base)
	if bad.code == 0 || !strings.Contains(bad.body, "没有改动现有数据") || strings.Contains(bad.body, "OUTSIDE") {
		t.Fatalf("bad package %#v", bad.body)
	}
	secret, err := os.ReadFile(outside)
	if err != nil || string(secret) != "OUTSIDE-MARKER-7d2b" {
		t.Fatal("outside file changed")
	}

	if !backup.TryBegin() {
		t.Fatal("lock was already held")
	}
	busy := callRestore(t, &api, &admin, backup.ConfirmOverwrite, archive, base)
	backup.End()
	if busy.code == 0 || !strings.Contains(busy.body, "已有备份正在导出或恢复") {
		t.Fatalf("busy %#v", busy.body)
	}
	if err := db.QueryRow(`select count(*) from marks where name = 'changed'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("busy restore changed the database: %v count=%d", err, count)
	}

	mysqlRestore := useConfig(t, "[base]\ndatabase_drive=mysql\nsource_path=./uploads\n[sqlite]\nfile_path=./database/database.db\n")
	mysql := callRestore(t, &api, &admin, backup.ConfirmOverwrite, archive, base)
	mysqlRestore()
	if mysql.code == 0 || !strings.Contains(mysql.body, "MySQL") {
		t.Fatalf("mysql %#v", mysql.body)
	}
	if err := db.QueryRow(`select count(*) from marks where name = 'changed'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("mysql restore changed the database: %v count=%d", err, count)
	}
}

func TestBackupRestoreOverwritesWithTheSnapshot(t *testing.T) {
	gin.SetMode(gin.TestMode)
	base, db, sqlDB := restoreService(t)
	defer sqlDB.Close()
	archive := servicePackage(t, base, sqlDB)
	if _, err := db.Exec(`insert into marks(name) values ('changed')`); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "uploads", "keep.txt"), []byte("CHANGED"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "uploads", "new.txt"), []byte("NEW"), 0o644); err != nil {
		t.Fatal(err)
	}
	admin := models.User{BaseModel: models.BaseModel{ID: 1}, Role: 1}
	response := callRestore(t, &BackupApi{}, &admin, backup.ConfirmOverwrite, archive, base)
	if response.code != 0 || !strings.Contains(response.body, `"code":0`) {
		t.Fatalf("restore %#v", response.body)
	}
	var count int
	if err := db.QueryRow(`select count(*) from marks where name = 'seed-before'`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("snapshot row missing: %v count=%d", err, count)
	}
	if err := db.QueryRow(`select count(*) from marks where name = 'changed'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("changed row remained: %v count=%d", err, count)
	}
	kept, err := os.ReadFile(filepath.Join(base, "uploads", "keep.txt"))
	if err != nil || string(kept) != "INSIDE-OK" {
		t.Fatalf("upload = %s %v", kept, err)
	}
	if _, err := os.Stat(filepath.Join(base, "uploads", "new.txt")); !os.IsNotExist(err) {
		t.Fatalf("new upload remained: %v", err)
	}
	if _, err := db.Exec(`insert into marks(name) values ('still-serving')`); err != nil {
		t.Fatal(err)
	}
}

func restoreService(t *testing.T) (string, *sql.DB, *sql.DB) {
	t.Helper()
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "uploads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "uploads", "keep.txt"), []byte("INSIDE-OK"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "conf"), 0o755); err != nil {
		t.Fatal(err)
	}
	conf := "[base]\ndatabase_drive=sqlite\nsource_path=./uploads\n[sqlite]\nfile_path=./database/database.db\n"
	if err := os.WriteFile(filepath.Join(base, "conf", "conf.ini"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "web", "custom"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "web", "custom", "index.css"), []byte("/* example */"), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(base, "database", "database.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	gormDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	if _, err := sqlDB.Exec(`create table marks(name text)`); err != nil {
		t.Fatal(err)
	}
	if _, err := sqlDB.Exec(`insert into marks(name) values ('seed-before')`); err != nil {
		t.Fatal(err)
	}
	previousDB := global.Db
	global.Db = gormDB
	t.Cleanup(func() { global.Db = previousDB })
	restore := useConfig(t, conf)
	t.Cleanup(restore)
	return base, sqlDB, sqlDB
}

func servicePackage(t *testing.T, base string, db *sql.DB) []byte {
	t.Helper()
	result, err := backup.Build(context.Background(), backup.Options{
		BaseDir:       base,
		DatabaseDrive: "sqlite",
		UploadsPath:   "uploads",
		ConfFile:      "conf/conf.ini",
		CustomDir:     "web/custom",
		SQLitePath:    "database/database.db",
		Version:       "1.3.0+leon.7",
		VersionCode:   10,
		DB:            db,
		TempParent:    t.TempDir(),
	})
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

func callRestore(t *testing.T, api *BackupApi, user *models.User, confirm string, archive []byte, baseDir string) exportResponse {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if confirm != "" {
		if err := writer.WriteField("confirm", confirm); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.WriteField("path", "/etc/passwd"); err != nil {
		t.Fatal(err)
	}
	if archive != nil {
		part, err := writer.CreateFormFile("file", "leon-panel-backup.zip")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(archive); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/backup/restore", body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	if user != nil {
		ctx.Set("userInfo", *user)
	}
	if baseDir == "" {
		api.Restore(ctx)
	} else {
		api.restoreAt(ctx, baseDir, "1.3.0+leon.11", 10)
	}
	response := exportResponse{status: recorder.Code, body: recorder.Body.String()}
	if strings.HasPrefix(response.body, "{") {
		var payload map[string]interface{}
		if err := json.Unmarshal([]byte(response.body), &payload); err == nil {
			if code, ok := payload["code"].(float64); ok {
				response.code = code
			}
		}
	}
	return response
}

func TestLegacyBackupGainsAnEmptyGroupIconStyle(t *testing.T) {
	gin.SetMode(gin.TestMode)
	base := t.TempDir()
	if err := os.MkdirAll(filepath.Join(base, "uploads"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "uploads", "keep.txt"), []byte("INSIDE-OK"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "conf"), 0o755); err != nil {
		t.Fatal(err)
	}
	conf := "[base]\ndatabase_drive=sqlite\nsource_path=./uploads\n[sqlite]\nfile_path=./database/database.db\n"
	if err := os.WriteFile(filepath.Join(base, "conf", "conf.ini"), []byte(conf), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(base, "web", "custom"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "web", "custom", "index.css"), []byte("/* example */"), 0o644); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(base, "database", "database.db")
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		t.Fatal(err)
	}
	gormDB, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := gormDB.AutoMigrate(&legacyBackupGroup{}); err != nil {
		t.Fatal(err)
	}
	legacy := legacyBackupGroup{Title: "HOME", UserId: 7, Sort: 1}
	if err := gormDB.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	previous := global.Db
	global.Db = gormDB
	t.Cleanup(func() { global.Db = previous })
	restore := useConfig(t, conf)
	defer restore()

	result, err := backup.Build(context.Background(), backup.Options{
		BaseDir:       base,
		DatabaseDrive: "sqlite",
		UploadsPath:   "uploads",
		ConfFile:      filepath.Join("conf", "conf.ini"),
		CustomDir:     filepath.Join("web", "custom"),
		SQLitePath:    filepath.Join("database", "database.db"),
		Version:       "1.3.0+leon.10",
		VersionCode:   10,
		DB:            sqlDB,
		TempParent:    t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(result.Dir) })
	archive, err := os.ReadFile(result.ZipPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := gormDB.Model(&legacyBackupGroup{}).Where("id = ?", legacy.ID).Update("title", "changed").Error; err != nil {
		t.Fatal(err)
	}

	admin := models.User{BaseModel: models.BaseModel{ID: 1}, Role: 1}
	response := callRestore(t, &BackupApi{}, &admin, backup.ConfirmOverwrite, archive, base)
	if response.code != 0 {
		t.Fatalf("restore %#v", response.body)
	}
	var saved models.ItemIconGroup
	if err := gormDB.First(&saved, legacy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if saved.Title != "HOME" || saved.UserId != 7 || saved.IconStyle != nil {
		t.Fatalf("restored group = %#v", saved)
	}
}

func useConfig(t *testing.T, contents string) func() {
	t.Helper()
	path := filepath.Join(t.TempDir(), "conf.ini")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	previous := global.Config
	global.Config = iniConfig.NewIniConfig(path)
	return func() { global.Config = previous }
}
