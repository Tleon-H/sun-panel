package system

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"sun-panel/api/api_v1/common/base"
	"sun-panel/global"
	"sun-panel/models"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func openModuleConfigDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:module-config-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.ModuleConfig{}); err != nil {
		t.Fatal(err)
	}
	previous := global.Db
	global.Db = db
	t.Cleanup(func() {
		global.Db = previous
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func performModuleConfig(t *testing.T, method func(*gin.Context), body string, userID uint, visitMode int, setVisit bool) map[string]interface{} {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", bytes.NewBufferString(body))
	ctx.Request.Header.Set("Content-Type", "application/json")
	if userID != 0 {
		ctx.Set("userInfo", models.User{BaseModel: models.BaseModel{ID: userID}})
	}
	if setVisit {
		ctx.Set(base.GIN_GET_VISIT_MODE, visitMode)
	}
	method(ctx)
	var payload map[string]interface{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("response %s: %v", recorder.Body.String(), err)
	}
	return payload
}

func TestModuleConfigSaveAndReadStayOnTheSignedInAccount(t *testing.T) {
	openModuleConfigDB(t)
	api := ModuleConfigApi{}
	private := `{"name":"module-deskModuleSearchBox","userId":99,"value":{"currentId":"custom-example","engines":[{"id":"custom-example","title":"示例搜索","url":"https://example.com/search?q=%s","builtin":false},{"id":"google","title":"Google","url":"https://www.google.com/search?q=%s","builtin":true}]}}`
	saved := performModuleConfig(t, api.Save, private, 7, 0, false)
	if saved["code"] != float64(0) {
		t.Fatalf("save = %#v", saved)
	}

	var rows []models.ModuleConfig
	if err := global.Db.Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].UserId != 7 {
		t.Fatalf("rows = %#v", rows)
	}

	own := performModuleConfig(t, api.GetByName, `{"name":"module-deskModuleSearchBox","userId":99}`, 7, base.VISIT_MODE_LOGIN, true)
	if own["code"] != float64(0) {
		t.Fatalf("owner read = %#v", own)
	}
	data, _ := own["data"].(map[string]interface{})
	engines, _ := data["engines"].([]interface{})
	if len(engines) != 2 {
		t.Fatalf("owner engines = %#v", data)
	}

	other := performModuleConfig(t, api.GetByName, `{"name":"module-deskModuleSearchBox","userId":7}`, 8, base.VISIT_MODE_LOGIN, true)
	if other["code"] != float64(0) || other["data"] != nil {
		t.Fatalf("other account read = %#v", other)
	}

	public := performModuleConfig(t, api.GetByName, `{"name":"module-deskModuleSearchBox"}`, 7, base.VISIT_MODE_PUBLIC, true)
	publicData, _ := public["data"].(map[string]interface{})
	publicEngines, _ := publicData["engines"].([]interface{})
	if len(publicEngines) != 1 {
		t.Fatalf("public engines = %#v", publicData)
	}

	rejected := performModuleConfig(t, api.Save, private, 0, 0, false)
	if rejected["code"] == float64(0) {
		t.Fatal("visitor save succeeded")
	}
	var count int64
	if err := global.Db.Model(&models.ModuleConfig{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("rows after rejected save = %d", count)
	}
}
