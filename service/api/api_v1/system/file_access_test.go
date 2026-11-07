package system

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sun-panel/global"
	"sun-panel/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openFileDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:files-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.File{}); err != nil {
		t.Fatal(err)
	}
	previousGlobal := global.Db
	previousModel := models.Db
	global.Db = db
	models.Db = db
	t.Cleanup(func() {
		global.Db = previousGlobal
		models.Db = previousModel
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func asFileUser(t *testing.T, method func(*gin.Context), body string, userID uint) map[string]interface{} {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	if userID != 0 {
		ctx.Set("userInfo", models.User{BaseModel: models.BaseModel{ID: userID}})
	}
	method(ctx)
	var payload map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response %s: %v", recorder.Body.String(), err)
	}
	return payload
}

func TestFileListFiltersPagesAndStaysOnTheAccount(t *testing.T) {
	db := openFileDB(t)
	api := FileApi{}
	saved, err := (&models.File{}).AddFile(7, "示例图标.png", ".png", "./uploads/example-icon.png", models.FileCategoryIcon)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Src != "./uploads/example-icon.png" || saved.Category != models.FileCategoryIcon {
		t.Fatalf("saved %#v", saved)
	}
	if _, err := (&models.File{}).AddFile(7, "示例壁纸.png", ".png", "./uploads/example-wall.png", models.FileCategoryWallpaper); err != nil {
		t.Fatal(err)
	}
	legacy, err := (&models.File{}).AddFile(7, "旧图片.png", ".png", "./uploads/example-old.png", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&models.File{}).Where("id = ?", legacy.ID).Update("category", "").Error; err != nil {
		t.Fatal(err)
	}
	if _, err := (&models.File{}).AddFile(8, "别人的图.png", ".png", "./uploads/example-other.png", models.FileCategoryIcon); err != nil {
		t.Fatal(err)
	}

	page := asFileUser(t, api.GetList, `{"page":1,"limit":2}`, 7)
	data, _ := page["data"].(map[string]interface{})
	if data["count"] != float64(3) {
		t.Fatalf("count = %#v", data)
	}
	list, _ := data["list"].([]interface{})
	if len(list) != 2 {
		t.Fatalf("page length = %d", len(list))
	}
	second := asFileUser(t, api.GetList, `{"page":2,"limit":2}`, 7)
	secondData, _ := second["data"].(map[string]interface{})
	secondList, _ := secondData["list"].([]interface{})
	if len(secondList) != 1 {
		t.Fatalf("second page = %#v", secondData)
	}

	icons := asFileUser(t, api.GetList, `{"category":"icon"}`, 7)
	iconData, _ := icons["data"].(map[string]interface{})
	if iconData["count"] != float64(1) {
		t.Fatalf("icons = %#v", iconData)
	}
	old := asFileUser(t, api.GetList, `{"category":"uncategorized","keyword":"旧"}`, 7)
	oldData, _ := old["data"].(map[string]interface{})
	oldList, _ := oldData["list"].([]interface{})
	if oldData["count"] != float64(1) || len(oldList) != 1 {
		t.Fatalf("legacy = %#v", oldData)
	}
	row, _ := oldList[0].(map[string]interface{})
	if row["category"] != models.FileCategoryUncategorized || row["src"] != "/uploads/example-old.png" {
		t.Fatalf("legacy row = %#v", row)
	}
	if row["fileName"] == "别人的图.png" {
		t.Fatal("another account's file was listed")
	}

	other := asFileUser(t, api.GetList, `{"keyword":"%"}`, 8)
	otherData, _ := other["data"].(map[string]interface{})
	if otherData["count"] != float64(0) {
		t.Fatalf("wildcard keyword matched unexpectedly: %#v", otherData)
	}
	otherAll := asFileUser(t, api.GetList, `{}`, 8)
	otherAllData, _ := otherAll["data"].(map[string]interface{})
	if otherAllData["count"] != float64(1) {
		t.Fatalf("other account count = %#v", otherAllData)
	}

	if bad := asFileUser(t, api.GetList, `{"category":"public"}`, 7); bad["code"] == float64(0) {
		t.Fatalf("invalid category accepted: %#v", bad)
	}
	if visitor := asFileUser(t, api.GetList, `{}`, 0); visitor["code"] == float64(0) {
		t.Fatalf("visitor listed files: %#v", visitor)
	}

	moved := asFileUser(t, api.UpdateCategory, fmt.Sprintf(`{"id":%d,"category":"wallpaper"}`, legacy.ID), 7)
	if moved["code"] != float64(0) {
		t.Fatalf("update = %#v", moved)
	}
	var after struct {
		Category string
		Src      string
	}
	if err := db.Table("files").Select("category, src").Where("file_name = ?", "旧图片.png").Take(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after.Category != models.FileCategoryWallpaper || after.Src != "./uploads/example-old.png" {
		t.Fatalf("after update %#v", after)
	}
	if stolen := asFileUser(t, api.UpdateCategory, fmt.Sprintf(`{"id":%d,"category":"icon"}`, saved.ID), 8); stolen["code"] == float64(0) {
		t.Fatal("another account changed the category")
	}
	if err := db.Table("files").Select("category, src").Where("file_name = ?", "示例图标.png").Take(&after).Error; err != nil {
		t.Fatal(err)
	}
	if after.Category != models.FileCategoryIcon || after.Src != "./uploads/example-icon.png" {
		t.Fatalf("owner category changed: %#v", after)
	}
	if invalid := asFileUser(t, api.UpdateCategory, fmt.Sprintf(`{"id":%d,"category":"javascript"}`, saved.ID), 7); invalid["code"] == float64(0) {
		t.Fatal("invalid category was saved")
	}
}
