package system

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"sun-panel/global"
	"sun-panel/lib/iniConfig"
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
	return asFileRole(t, method, body, userID, 2)
}

func asFileRole(t *testing.T, method func(*gin.Context), body string, userID uint, role int) map[string]interface{} {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	if userID != 0 {
		ctx.Set("userInfo", models.User{BaseModel: models.BaseModel{ID: userID}, Role: role})
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

func TestUploadImgUsesTheIconAndWallpaperEntries(t *testing.T) {
	db := openFileDB(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "conf.ini")
	if err := os.WriteFile(configPath, []byte("[base]\nsource_path="+filepath.Join(dir, "uploads")+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	previous := global.Config
	global.Config = iniConfig.NewIniConfig(configPath)
	t.Cleanup(func() { global.Config = previous })

	api := FileApi{}
	icon := uploadImage(t, &api, "example-icon.png", "icon", 7)
	if icon["code"] != float64(0) {
		t.Fatalf("icon upload %#v", icon)
	}
	wallpaper := uploadImage(t, &api, "example-wallpaper.png", "wallpaper", 7)
	if wallpaper["code"] != float64(0) {
		t.Fatalf("wallpaper upload %#v", wallpaper)
	}
	var categories []string
	if err := db.Table("files").Where("user_id = ?", 7).Order("file_name").Pluck("category", &categories).Error; err != nil {
		t.Fatal(err)
	}
	if len(categories) != 2 || categories[0] != models.FileCategoryIcon || categories[1] != models.FileCategoryWallpaper {
		t.Fatalf("categories %#v", categories)
	}
	found := asFileUser(t, api.GetList, `{"keyword":"example-wallpaper","limit":12}`, 7)
	data, _ := found["data"].(map[string]interface{})
	if data["count"] != float64(1) {
		t.Fatalf("search %#v", data)
	}
	for i := 0; i < 11; i++ {
		if _, err := (&models.File{}).AddFile(7, fmt.Sprintf("example-page-%02d.png", i), ".png", "./uploads/page.png", models.FileCategoryIcon); err != nil {
			t.Fatal(err)
		}
	}
	page := asFileUser(t, api.GetList, `{"page":1,"limit":12}`, 7)
	pageData, _ := page["data"].(map[string]interface{})
	list, _ := pageData["list"].([]interface{})
	if pageData["count"] != float64(13) || len(list) != 12 {
		t.Fatalf("first page %#v", pageData["count"])
	}
	next := asFileUser(t, api.GetList, `{"page":2,"limit":12}`, 7)
	nextData, _ := next["data"].(map[string]interface{})
	nextList, _ := nextData["list"].([]interface{})
	if len(nextList) != 1 {
		t.Fatalf("second page %#v", nextData)
	}
}

func uploadImage(t *testing.T, api *FileApi, name, category string, userID uint) map[string]interface{} {
	t.Helper()
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("imgfile", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(png); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("category", category); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/file/uploadImg", body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	ctx.Set("userInfo", models.User{BaseModel: models.BaseModel{ID: userID}})
	api.UploadImg(ctx)
	var payload map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response %s: %v", recorder.Body.String(), err)
	}
	return payload
}

func TestPublicGalleryShowsOnlyMarkedFiles(t *testing.T) {
	db := openFileDB(t)
	api := FileApi{}
	shared, err := (&models.File{}).AddFile(7, "公共图标.png", ".png", "./uploads/public-icon.png", models.FileCategoryIcon)
	if err != nil {
		t.Fatal(err)
	}
	if shared.IsPublic {
		t.Fatal("a new upload was public")
	}
	privateFile, err := (&models.File{}).AddFile(7, "私人图标.png", ".png", "./uploads/private-icon.png", models.FileCategoryIcon)
	if err != nil {
		t.Fatal(err)
	}
	wallpaper, err := (&models.File{}).AddFile(7, "公共壁纸.png", ".png", "./uploads/public-wall.png", models.FileCategoryWallpaper)
	if err != nil {
		t.Fatal(err)
	}
	other, err := (&models.File{}).AddFile(8, "别人的私人图.png", ".png", "./uploads/other-private.png", models.FileCategoryIcon)
	if err != nil {
		t.Fatal(err)
	}

	empty := asFileRole(t, api.PublicList, `{"userId":7}`, 8, 2)
	emptyData, _ := empty["data"].(map[string]interface{})
	if empty["code"] != float64(0) || emptyData["count"] != float64(0) {
		t.Fatalf("unmarked gallery %#v", empty)
	}
	if denied := asFileRole(t, api.SetPublic, fmt.Sprintf(`{"id":%d,"public":true}`, other.ID), 8, 2); denied["code"] != float64(1005) {
		t.Fatalf("regular user published %#v", denied)
	}
	stolen := asFileRole(t, api.SetPublic, fmt.Sprintf(`{"id":%d,"public":true,"userId":8}`, other.ID), 7, 1)
	if stolen["code"] == float64(0) {
		t.Fatal("an administrator published another account's file")
	}
	if published := asFileRole(t, api.SetPublic, fmt.Sprintf(`{"id":%d,"public":true,"userId":8}`, shared.ID), 7, 1); published["code"] != float64(0) {
		t.Fatalf("publish own %#v", published)
	}
	if published := asFileRole(t, api.SetPublic, fmt.Sprintf(`{"id":%d,"public":true}`, wallpaper.ID), 7, 1); published["code"] != float64(0) {
		t.Fatalf("publish wallpaper %#v", published)
	}
	var row struct {
		UserId   uint
		IsPublic bool
		Src      string
	}
	if err := db.Table("files").Select("user_id, is_public, src").Where("id = ?", shared.ID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.UserId != 7 || !row.IsPublic || row.Src != "./uploads/public-icon.png" {
		t.Fatalf("published row %#v", row)
	}
	if err := db.Table("files").Select("user_id, is_public, src").Where("id = ?", other.ID).Take(&row).Error; err != nil {
		t.Fatal(err)
	}
	if row.UserId != 8 || row.IsPublic {
		t.Fatalf("other account changed %#v", row)
	}

	wildcard := asFileRole(t, api.PublicList, `{"userId":8,"keyword":"%"}`, 8, 2)
	wildcardData, _ := wildcard["data"].(map[string]interface{})
	if wildcardData["count"] != float64(0) {
		t.Fatalf("wildcard listed public files %#v", wildcardData)
	}
	gallery := asFileRole(t, api.PublicList, `{"userId":7,"user_id":8}`, 8, 2)
	names := publicNames(t, gallery)
	if !containsName(names, "公共图标.png") || !containsName(names, "公共壁纸.png") || containsName(names, "私人图标.png") || containsName(names, "别人的私人图.png") {
		t.Fatalf("gallery names %#v", names)
	}
	icons := asFileRole(t, api.PublicList, `{"category":"icon","userId":7}`, 8, 2)
	iconNames := publicNames(t, icons)
	if len(iconNames) != 1 || iconNames[0] != "公共图标.png" {
		t.Fatalf("icon gallery %#v", iconNames)
	}

	own := asFileRole(t, api.GetList, `{"userId":7}`, 8, 2)
	ownNames := listedNames(t, own)
	if containsName(ownNames, "公共图标.png") || containsName(ownNames, "私人图标.png") {
		t.Fatalf("own list included another account %#v", ownNames)
	}
	adminList := asFileRole(t, api.GetList, `{}`, 7, 1)
	adminNames := listedNames(t, adminList)
	if !containsName(adminNames, "私人图标.png") || !containsName(adminNames, "公共图标.png") || containsName(adminNames, "别人的私人图.png") {
		t.Fatalf("admin own list %#v", adminNames)
	}
	adminData, _ := adminList["data"].(map[string]interface{})
	for _, item := range adminData["list"].([]interface{}) {
		record, _ := item.(map[string]interface{})
		if record["fileName"] == "公共图标.png" && record["public"] != true {
			t.Fatalf("own public flag %#v", record)
		}
		if record["fileName"] == "私人图标.png" && record["public"] != false {
			t.Fatalf("own private flag %#v", record)
		}
	}

	if hidden := asFileRole(t, api.SetPublic, fmt.Sprintf(`{"id":%d,"public":false,"userId":8}`, shared.ID), 7, 1); hidden["code"] != float64(0) {
		t.Fatalf("unpublish %#v", hidden)
	}
	after := publicNames(t, asFileRole(t, api.PublicList, `{"userId":7}`, 8, 2))
	if containsName(after, "公共图标.png") || !containsName(after, "公共壁纸.png") {
		t.Fatalf("after unpublish %#v", after)
	}
	if visitor := asFileRole(t, api.PublicList, `{}`, 0, 0); visitor["code"] != float64(1000) {
		t.Fatalf("visitor list %#v", visitor)
	}
	if visitor := asFileRole(t, api.SetPublic, fmt.Sprintf(`{"id":%d,"public":true}`, privateFile.ID), 0, 0); visitor["code"] != float64(1000) {
		t.Fatalf("visitor publish %#v", visitor)
	}
	removed := asFileUser(t, api.Deletes, fmt.Sprintf(`{"ids":[%d]}`, wallpaper.ID), 7)
	if removed["code"] != float64(0) {
		t.Fatalf("delete %#v", removed)
	}
	if names := publicNames(t, asFileRole(t, api.PublicList, `{}`, 8, 2)); len(names) != 0 {
		t.Fatalf("deleted public file remained %#v", names)
	}
}

func TestUploadFilesKeepsEntryCategoryAndAccount(t *testing.T) {
	db := openFileDB(t)
	dir := t.TempDir()
	uploadDir := filepath.Join(dir, "uploads")
	configPath := filepath.Join(dir, "conf.ini")
	if err := os.WriteFile(configPath, []byte("[base]\nsource_path="+uploadDir+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	previous := global.Config
	global.Config = iniConfig.NewIniConfig(configPath)
	t.Cleanup(func() { global.Config = previous })

	api := FileApi{}
	png := tinyPNG(t)
	plain := uploadBatch(t, &api, 7, "", map[string]string{"userId": "99"}, map[string][]byte{
		"batch-a.png": png,
		"batch-b.PNG": png,
	})
	if plain["code"] != float64(0) {
		t.Fatalf("plain upload %#v", plain)
	}
	plainData, _ := plain["data"].(map[string]interface{})
	saved, _ := plainData["saved"].([]interface{})
	failed, _ := plainData["errFiles"].([]interface{})
	if len(saved) != 2 || len(failed) != 0 {
		t.Fatalf("plain result %#v", plainData)
	}
	for _, item := range saved {
		row, _ := item.(map[string]interface{})
		if row["category"] != models.FileCategoryUncategorized {
			t.Fatalf("batch category %#v", row)
		}
	}

	var rows []struct {
		UserId   uint
		Category string
		IsPublic bool
		Src      string
		FileName string
	}
	if err := db.Table("files").Select("user_id, category, is_public, src, file_name").Order("file_name").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows %#v", rows)
	}
	for _, row := range rows {
		if row.UserId != 7 || row.Category != models.FileCategoryUncategorized || row.IsPublic || !strings.HasPrefix(row.Src, uploadDir) {
			t.Fatalf("stored %#v", row)
		}
	}

	icon := uploadBatch(t, &api, 7, "icon", nil, map[string][]byte{"batch-icon.png": png})
	wall := uploadBatch(t, &api, 7, "wallpaper", nil, map[string][]byte{"batch-wall.png": png})
	if icon["code"] != float64(0) || wall["code"] != float64(0) {
		t.Fatalf("entry uploads %#v %#v", icon, wall)
	}
	if categoryOf(t, db, "batch-icon.png") != models.FileCategoryIcon || categoryOf(t, db, "batch-wall.png") != models.FileCategoryWallpaper {
		t.Fatal("icon and wallpaper entries were not kept")
	}

	mixed := uploadBatch(t, &api, 7, "", nil, map[string][]byte{
		"batch-ok.png": png,
		"notes.txt":    []byte("not an image"),
	})
	mixedData, _ := mixed["data"].(map[string]interface{})
	mixedSaved, _ := mixedData["saved"].([]interface{})
	mixedFailed, _ := mixedData["errFiles"].([]interface{})
	if mixed["code"] != float64(0) || len(mixedSaved) != 1 || len(mixedFailed) != 1 || mixedFailed[0] != "notes.txt" {
		t.Fatalf("mixed %#v", mixed)
	}
	if countNamed(t, db, "notes.txt") != 0 || categoryOf(t, db, "batch-ok.png") != models.FileCategoryUncategorized {
		t.Fatal("a non-image was stored or the image category changed")
	}

	before := countNamed(t, db, "batch-bad.png")
	rejected := uploadBatch(t, &api, 7, "public", map[string]string{"userId": "8"}, map[string][]byte{"batch-bad.png": png})
	if rejected["code"] == float64(0) || countNamed(t, db, "batch-bad.png") != before {
		t.Fatalf("invalid category %#v", rejected)
	}
	if visitor := uploadBatch(t, &api, 0, "", nil, map[string][]byte{"visitor.png": png}); visitor["code"] != float64(1000) {
		t.Fatalf("visitor %#v", visitor)
	}
	if countNamed(t, db, "visitor.png") != 0 {
		t.Fatal("a visitor upload was stored")
	}

	other := asFileRole(t, api.GetList, `{"userId":7,"category":"uncategorized"}`, 8, 2)
	names := listedNames(t, other)
	if containsName(names, "batch-a.png") || containsName(names, "batch-icon.png") {
		t.Fatalf("another account listed the batch %#v", names)
	}
	published := asFileRole(t, api.SetPublic, fmt.Sprintf(`{"id":%d,"public":true,"userId":8}`, fileID(t, db, "batch-icon.png")), 7, 1)
	if published["code"] != float64(0) {
		t.Fatalf("publish %#v", published)
	}
	ownAfter := listedNames(t, asFileRole(t, api.GetList, `{"userId":7}`, 8, 2))
	if containsName(ownAfter, "batch-icon.png") || containsName(ownAfter, "batch-a.png") {
		t.Fatalf("own list included another account's public image %#v", ownAfter)
	}
	gallery := publicNames(t, asFileRole(t, api.PublicList, `{"userId":7}`, 8, 2))
	if !containsName(gallery, "batch-icon.png") || containsName(gallery, "batch-a.png") {
		t.Fatalf("public list %#v", gallery)
	}
}

func tinyPNG(t *testing.T) []byte {
	t.Helper()
	png, err := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==")
	if err != nil {
		t.Fatal(err)
	}
	return png
}

func uploadBatch(t *testing.T, api *FileApi, userID uint, category string, fields map[string]string, files map[string][]byte) map[string]interface{} {
	t.Helper()
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	// Keep the multipart order stable. Map iteration would make failure names flaky.
	sort.Strings(names)
	for _, name := range names {
		part, err := writer.CreateFormFile("files[]", name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(files[name]); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.WriteField("category", category); err != nil {
		t.Fatal(err)
	}
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/api/file/uploadFiles", body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	if userID != 0 {
		ctx.Set("userInfo", models.User{BaseModel: models.BaseModel{ID: userID}, Role: 2})
	}
	api.UploadFiles(ctx)
	var payload map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response %s: %v", recorder.Body.String(), err)
	}
	return payload
}

func categoryOf(t *testing.T, db *gorm.DB, name string) string {
	t.Helper()
	var category string
	if err := db.Table("files").Select("category").Where("file_name = ?", name).Scan(&category).Error; err != nil {
		t.Fatal(err)
	}
	return category
}

func countNamed(t *testing.T, db *gorm.DB, name string) int64 {
	t.Helper()
	var count int64
	if err := db.Table("files").Where("file_name = ?", name).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func fileID(t *testing.T, db *gorm.DB, name string) uint {
	t.Helper()
	var id uint
	if err := db.Table("files").Select("id").Where("file_name = ?", name).Scan(&id).Error; err != nil || id == 0 {
		t.Fatalf("id for %s: %v", name, err)
	}
	return id
}

func publicNames(t *testing.T, payload map[string]interface{}) []string {
	t.Helper()
	if payload["code"] != float64(0) {
		t.Fatalf("public list %#v", payload)
	}
	data, _ := payload["data"].(map[string]interface{})
	list, _ := data["list"].([]interface{})
	names := make([]string, 0, len(list))
	for _, item := range list {
		row, _ := item.(map[string]interface{})
		if _, ok := row["userId"]; ok {
			t.Fatalf("public row exposed the owner %#v", row)
		}
		if _, ok := row["path"]; ok {
			t.Fatalf("public row exposed the stored path %#v", row)
		}
		names = append(names, fmt.Sprint(row["fileName"]))
	}
	return names
}

func listedNames(t *testing.T, payload map[string]interface{}) []string {
	t.Helper()
	data, _ := payload["data"].(map[string]interface{})
	list, _ := data["list"].([]interface{})
	names := make([]string, 0, len(list))
	for _, item := range list {
		row, _ := item.(map[string]interface{})
		names = append(names, fmt.Sprint(row["fileName"]))
	}
	return names
}

func containsName(names []string, name string) bool {
	for _, item := range names {
		if item == name {
			return true
		}
	}
	return false
}
