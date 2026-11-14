package models

import (
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/schema"
)

type legacyItemIconGroup struct {
	BaseModel
	Icon        string `gorm:"type:varchar(50)"`
	Title       string `gorm:"type:varchar(50)"`
	Description string `gorm:"type:varchar(1000)"`
	Sort        int    `gorm:"type:int(11)"`
	UserId      uint
}

func (legacyItemIconGroup) TableName() string { return "item_icon_group" }

func openGroupStyleDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := fmt.Sprintf("file:group-style-%d?mode=memory&cache=shared", time.Now().UnixNano())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		NamingStrategy: schema.NamingStrategy{SingularTable: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db
}

func iconStyleColumn(t *testing.T, db *gorm.DB, id uint) sql.NullInt64 {
	t.Helper()
	var style sql.NullInt64
	row := db.Raw("select icon_style from item_icon_group where id = ?", id).Row()
	if err := row.Scan(&style); err != nil {
		t.Fatal(err)
	}
	return style
}

func TestLegacyGroupRowsStayUnsetWhenTheStyleColumnIsAdded(t *testing.T) {
	db := openGroupStyleDB(t)
	if err := db.AutoMigrate(&legacyItemIconGroup{}); err != nil {
		t.Fatal(err)
	}
	legacy := legacyItemIconGroup{Title: "HOME", UserId: 7, Sort: 1, Icon: "material-symbols:folder-outline"}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}

	if err := db.AutoMigrate(&ItemIconGroup{}); err != nil {
		t.Fatal(err)
	}
	if style := iconStyleColumn(t, db, legacy.ID); style.Valid {
		t.Fatalf("startup migration filled an old row: %#v", style)
	}
	if err := EnsureItemIconGroupIconStyle(db); err != nil {
		t.Fatal(err)
	}
	if err := EnsureItemIconGroupIconStyle(db); err != nil {
		t.Fatal(err)
	}

	var loaded ItemIconGroup
	if err := db.First(&loaded, legacy.ID).Error; err != nil {
		t.Fatal(err)
	}
	if loaded.Title != "HOME" || loaded.UserId != 7 || loaded.IconStyle != nil {
		t.Fatalf("loaded = %#v", loaded)
	}

	info := GroupIconStyleInfo
	small := GroupIconStyleIcon
	if err := db.Create(&ItemIconGroup{Title: "详情", UserId: 7, IconStyle: &info}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&ItemIconGroup{Title: "小图标", UserId: 7, IconStyle: &small}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&ItemIconGroup{Title: "跟随", UserId: 7}).Error; err != nil {
		t.Fatal(err)
	}

	var rows []ItemIconGroup
	if err := db.Where("user_id = ?", 7).Order("id").Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 || rows[0].IconStyle != nil || rows[1].IconStyle == nil || *rows[1].IconStyle != 0 || rows[2].IconStyle == nil || *rows[2].IconStyle != 1 || rows[3].IconStyle != nil {
		t.Fatalf("rows = %#v", rows)
	}
}

func TestEnsureAddsTheColumnWhenOnlyTheLegacyTableExists(t *testing.T) {
	db := openGroupStyleDB(t)
	if err := EnsureItemIconGroupIconStyle(db); err != nil {
		t.Fatal(err)
	}
	if db.Migrator().HasTable(&ItemIconGroup{}) {
		t.Fatal("ensure created a table before startup migration")
	}
	if err := db.AutoMigrate(&legacyItemIconGroup{}); err != nil {
		t.Fatal(err)
	}
	legacy := legacyItemIconGroup{Title: "常用网站", UserId: 3}
	if err := db.Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}
	if err := EnsureItemIconGroupIconStyle(db); err != nil {
		t.Fatal(err)
	}
	if style := iconStyleColumn(t, db, legacy.ID); style.Valid {
		t.Fatalf("restored row was filled: %#v", style)
	}
	if err := EnsureItemIconGroupIconStyle(nil); err == nil {
		t.Fatal("nil database was accepted")
	}
}

func TestGroupIconStyleColumnStaysNullableOnSQLiteAndMySQL(t *testing.T) {
	parsed, err := schema.Parse(&ItemIconGroup{}, &sync.Map{}, schema.NamingStrategy{SingularTable: true})
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Table != "item_icon_group" {
		t.Fatalf("table = %s", parsed.Table)
	}
	field := parsed.LookUpField("IconStyle")
	if field == nil || field.DBName != "icon_style" || field.NotNull || field.HasDefaultValue {
		t.Fatalf("field = %#v", field)
	}
	dialectors := []struct {
		name      string
		dialector gorm.Dialector
	}{
		{name: "sqlite", dialector: sqlite.Open("file::memory:?cache=shared")},
		{name: "mysql", dialector: mysql.New(mysql.Config{DSN: "user:pass@tcp(127.0.0.1:1)/sun?charset=utf8mb4&parseTime=True&loc=Local", SkipInitializeWithVersion: true})},
	}
	for _, item := range dialectors {
		dataType := strings.ToUpper(item.dialector.DataTypeOf(field))
		t.Logf("%s icon_style type %s", item.name, dataType)
		if strings.Contains(dataType, "NOT NULL") || strings.Contains(dataType, "DEFAULT") || !strings.Contains(dataType, "INT") {
			t.Fatalf("%s column type %q would change existing rows", item.name, dataType)
		}
	}

	if !ValidGroupIconStyle(nil) {
		t.Fatal("empty style was rejected")
	}
	info := GroupIconStyleInfo
	small := GroupIconStyleIcon
	other := 2
	if !ValidGroupIconStyle(&info) || !ValidGroupIconStyle(&small) || ValidGroupIconStyle(&other) {
		t.Fatal("style validation does not match the existing icon styles")
	}
}
