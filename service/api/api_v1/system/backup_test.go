package system

import (
	"archive/zip"
	"bytes"
	"database/sql"
	"encoding/json"
	"io"
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
)

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
