package backup

import (
	"bytes"
	"context"
	"database/sql"
	"testing"
)

func TestBackupRoundTripKeepsGroupIconStyle(t *testing.T) {
	base, db := restoreFixture(t)
	if _, err := db.Exec(`create table item_icon_group(id integer primary key, title text, icon_style integer)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`insert into item_icon_group(id, title, icon_style) values (1, 'HOME', 0), (2, '常用网站', 1), (3, '未设置', NULL)`); err != nil {
		t.Fatal(err)
	}
	raw := buildPackage(t, base, db, "1.3.0+leon.11")
	if _, err := db.Exec(`update item_icon_group set title = 'changed', icon_style = 1`); err != nil {
		t.Fatal(err)
	}
	if err := Restore(context.Background(), restoreOpt(base, db, ""), bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	assertGroupStyle(t, db, 1, "HOME", sql.NullInt64{Int64: 0, Valid: true})
	assertGroupStyle(t, db, 2, "常用网站", sql.NullInt64{Int64: 1, Valid: true})
	assertGroupStyle(t, db, 3, "未设置", sql.NullInt64{})
}

func TestLegacyBackupWithoutGroupIconStyleStillRestores(t *testing.T) {
	base, db := restoreFixture(t)
	if _, err := db.Exec(`create table item_icon_group(id integer primary key, title text)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`insert into item_icon_group(id, title) values (1, 'HOME')`); err != nil {
		t.Fatal(err)
	}
	raw := buildPackage(t, base, db, "1.3.0+leon.10")
	if _, err := db.Exec(`alter table item_icon_group add column icon_style integer`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`update item_icon_group set title = 'changed', icon_style = 1`); err != nil {
		t.Fatal(err)
	}
	if err := Restore(context.Background(), restoreOpt(base, db, ""), bytes.NewReader(raw)); err != nil {
		t.Fatal(err)
	}
	var title string
	if err := db.QueryRow(`select title from item_icon_group where id = 1`).Scan(&title); err != nil || title != "HOME" {
		t.Fatalf("title = %s %v", title, err)
	}
	if _, err := db.Exec(`select icon_style from item_icon_group`); err == nil {
		t.Fatal("leon.10 backup unexpectedly kept icon_style")
	}
}

func assertGroupStyle(t *testing.T, db *sql.DB, id int, title string, style sql.NullInt64) {
	t.Helper()
	var gotTitle string
	var got sql.NullInt64
	if err := db.QueryRow(`select title, icon_style from item_icon_group where id = ?`, id).Scan(&gotTitle, &got); err != nil {
		t.Fatal(err)
	}
	if gotTitle != title || got.Valid != style.Valid || got.Int64 != style.Int64 {
		t.Fatalf("id %d got title %s style %#v", id, gotTitle, got)
	}
}
