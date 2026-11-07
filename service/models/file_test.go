package models

import (
	"testing"
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
