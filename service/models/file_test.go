package models

import (
	"strings"
	"testing"
	"unicode/utf8"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestNormalizeFileCategoryKeepsOldRowsUncategorized(t *testing.T) {
	for _, raw := range []string{"", "  ", "Uncategorized", "uncategorized"} {
		got, ok := NormalizeFileCategory(raw)
		if !ok || got != FileCategoryUncategorized {
			t.Fatalf("%q -> %q ok=%v", raw, got, ok)
		}
	}
	if got, ok := NormalizeFileCategory("ICON"); !ok || got != FileCategoryIcon {
		t.Fatalf("icon = %q ok=%v", got, ok)
	}
	if got, ok := NormalizeFileCategory("wallpaper"); !ok || got != FileCategoryWallpaper {
		t.Fatalf("wallpaper = %q ok=%v", got, ok)
	}
	if _, ok := NormalizeFileCategory("public"); ok {
		t.Fatal("unknown category was accepted")
	}
	if DisplayFileCategory("nope") != FileCategoryUncategorized {
		t.Fatal("unknown stored category should display as uncategorized")
	}
}

func TestNormalizeFileListQuery(t *testing.T) {
	query, err := NormalizeFileListQuery(0, 0, "", "  示例  ")
	if err != nil {
		t.Fatal(err)
	}
	if query.Page != 1 || query.Limit != 24 || query.Category != "" || query.Keyword != "示例" {
		t.Fatalf("%#v", query)
	}
	query, err = NormalizeFileListQuery(2, 1000, "icon", "name")
	if err != nil {
		t.Fatal(err)
	}
	if query.Page != 2 || query.Limit != 60 || query.Category != FileCategoryIcon {
		t.Fatalf("%#v", query)
	}
	if _, err = NormalizeFileListQuery(1, 10, "shared", ""); err != ErrFileCategory {
		t.Fatalf("err = %v", err)
	}
}

func TestChineseKeywordIsTruncatedOnCharacters(t *testing.T) {
	keyword := strings.Repeat("图", fileKeywordMaxLength+1)
	query, err := NormalizeFileListQuery(1, 10, "", keyword)
	if err != nil {
		t.Fatal(err)
	}
	if !utf8.ValidString(query.Keyword) || utf8.RuneCountInString(query.Keyword) != fileKeywordMaxLength {
		t.Fatalf("keyword was cut mid-character: %q", query.Keyword)
	}
	if strings.Contains(query.Keyword, "\uFFFD") {
		t.Fatal("keyword contains a replacement character")
	}
}

func TestFileNameLikeUsesOneCharacterEscape(t *testing.T) {
	if fileNameLikeSQL != "file_name LIKE ? ESCAPE '!'" || strings.Contains(fileNameLikeSQL, `\`) {
		t.Fatalf("sql = %s", fileNameLikeSQL)
	}
	if got := fileKeywordLike(`100%_!`); got != `%100!%!_!!%` {
		t.Fatalf("pattern = %s", got)
	}
	// This opens SQLite only. It checks the same clause text; it does not execute MySQL or MariaDB.
	db := openKeywordDB(t)
	longName := strings.Repeat("图", 90) + ".png"
	for _, name := range []string{longName, `100%_!.png`, "plain.png"} {
		if _, err := (&File{}).AddFile(3, name, ".png", "./uploads/"+name, FileCategoryIcon); err != nil {
			t.Fatal(err)
		}
	}
	query, err := NormalizeFileListQuery(1, 10, "", strings.Repeat("图", 81))
	if err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := FileListScope(db, 3, query).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("chinese prefix count=%d err=%v", count, err)
	}
	literal, err := NormalizeFileListQuery(1, 10, "", `100%_!`)
	if err != nil {
		t.Fatal(err)
	}
	count = 0
	if err := FileListScope(db, 3, literal).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("literal wildcard count=%d err=%v", count, err)
	}
	anyPercent, err := NormalizeFileListQuery(1, 10, "", "%")
	if err != nil {
		t.Fatal(err)
	}
	count = 0
	if err := FileListScope(db, 3, anyPercent).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("escaped percent count=%d err=%v", count, err)
	}
	sql := db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return FileListScope(tx, 3, literal).Find(&[]File{})
	})
	if !strings.Contains(sql, "ESCAPE '!'") || strings.Contains(sql, `ESCAPE '\'`) || strings.Contains(sql, `ESCAPE '\\'`) {
		t.Fatalf("generated sql = %s", sql)
	}
}

func openKeywordDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:keyword-"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&File{}); err != nil {
		t.Fatal(err)
	}
	previous := Db
	Db = db
	t.Cleanup(func() {
		Db = previous
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}
