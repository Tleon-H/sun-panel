package system

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sun-panel/global"
	"sun-panel/models"

	"gorm.io/gorm"
)

func TestDeleteOwnFilesRequiresConfirmationAndRejectsUnsafeRequests(t *testing.T) {
	db := openFileDB(t)
	root := t.TempDir()
	restore := useConfig(t, "[base]\nsource_path="+root+"\n")
	t.Cleanup(restore)
	api := FileApi{}
	ownedPath := filepath.Join(root, "owned.png")
	if err := os.WriteFile(ownedPath, []byte("owned-bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	owned := addUpload(t, 7, "自己的图.png", ownedPath)

	visitor := asFileRole(t, api.Deletes, confirmedIDs(owned.ID), 0, 0)
	if visitor["code"] != float64(1000) {
		t.Fatalf("visitor %#v", visitor)
	}
	assertKept(t, db, owned.ID, ownedPath, "owned-bytes")

	missing := asFileUser(t, api.Deletes, fmt.Sprintf(`{"ids":[%d]}`, owned.ID), 7)
	if missing["code"] == float64(0) || !strings.Contains(fmt.Sprint(missing["msg"]), "再次确认") {
		t.Fatalf("missing confirm %#v", missing)
	}
	assertKept(t, db, owned.ID, ownedPath, "owned-bytes")

	wrong := asFileUser(t, api.Deletes, fmt.Sprintf(`{"ids":[%d],"confirm":"yes"}`, owned.ID), 7)
	if wrong["code"] == float64(0) || !strings.Contains(fmt.Sprint(wrong["msg"]), "再次确认") {
		t.Fatalf("wrong confirm %#v", wrong)
	}
	assertKept(t, db, owned.ID, ownedPath, "owned-bytes")

	empty := asFileUser(t, api.Deletes, fmt.Sprintf(`{"ids":[],"confirm":%q}`, DeleteOwnFilesConfirm), 7)
	if empty["code"] != float64(1400) {
		t.Fatalf("empty %#v", empty)
	}
	assertKept(t, db, owned.ID, ownedPath, "owned-bytes")

	negative := asFileUser(t, api.Deletes, fmt.Sprintf(`{"ids":[-1],"confirm":%q}`, DeleteOwnFilesConfirm), 7)
	if negative["code"] != float64(1400) {
		t.Fatalf("negative %#v", negative)
	}
	assertKept(t, db, owned.ID, ownedPath, "owned-bytes")

	malformed := asFileUser(t, api.Deletes, fmt.Sprintf(`{"ids":"no","confirm":%q}`, DeleteOwnFilesConfirm), 7)
	if malformed["code"] != float64(1400) {
		t.Fatalf("malformed %#v", malformed)
	}
	assertKept(t, db, owned.ID, ownedPath, "owned-bytes")

	huge := make([]string, DeleteOwnFilesMaxIDs+1)
	for i := range huge {
		huge[i] = fmt.Sprintf("%d", owned.ID)
	}
	oversized := asFileUser(t, api.Deletes, fmt.Sprintf(`{"ids":[%s],"confirm":%q}`, strings.Join(huge, ","), DeleteOwnFilesConfirm), 7)
	if oversized["code"] == float64(0) || !strings.Contains(fmt.Sprint(oversized["msg"]), "100") {
		t.Fatalf("oversized %#v", oversized)
	}
	assertKept(t, db, owned.ID, ownedPath, "owned-bytes")

	savedConfig := global.Config
	global.Config = nil
	t.Cleanup(func() {
		if global.Config == nil {
			global.Config = savedConfig
		}
	})
	noRoot := asFileUser(t, api.Deletes, confirmedIDs(owned.ID), 7)
	global.Config = savedConfig
	if noRoot["code"] == float64(0) {
		t.Fatalf("missing upload root still deleted %#v", noRoot)
	}
	if reasons := failureReasons(t, noRoot); reasons[float64(owned.ID)] != deleteReasonUnsafePath {
		t.Fatalf("missing root reasons %#v", noRoot)
	}
	assertKept(t, db, owned.ID, ownedPath, "owned-bytes")

	duplicate := asFileUser(t, api.Deletes, fmt.Sprintf(`{"ids":[%d,%d,%d],"confirm":%q}`, owned.ID, owned.ID, owned.ID, DeleteOwnFilesConfirm), 7)
	if duplicate["code"] != float64(0) {
		t.Fatalf("duplicate %#v", duplicate)
	}
	deleted := numberList(t, duplicate, "deleted")
	if len(deleted) != 1 || deleted[0] != float64(owned.ID) {
		t.Fatalf("duplicate deleted %#v", duplicate["data"])
	}
	if liveFileCount(t, db, owned.ID) != 0 {
		t.Fatal("duplicate ids deleted the row more than the record allows, or left it behind")
	}
	if _, err := os.Stat(ownedPath); !os.IsNotExist(err) {
		t.Fatalf("owned file remained: %v", err)
	}
}

func TestDeleteOwnFilesKeepsOtherAccountsAndReportsPartialResults(t *testing.T) {
	db := openFileDB(t)
	root := t.TempDir()
	restore := useConfig(t, "[base]\nsource_path="+root+"\n")
	t.Cleanup(restore)
	api := FileApi{}
	ownPath := filepath.Join(root, "mine.png")
	otherPath := filepath.Join(root, "theirs.png")
	if err := os.WriteFile(ownPath, []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(otherPath, []byte("theirs"), 0o644); err != nil {
		t.Fatal(err)
	}
	own := addUpload(t, 7, "我的图.png", ownPath)
	other := addUpload(t, 8, "别人的图.png", otherPath)
	if _, err := (&models.File{}).SetOwnPublic(db, 8, other.ID, true); err != nil {
		t.Fatal(err)
	}

	body := fmt.Sprintf(`{"ids":[%d,%d,%d,999999],"confirm":%q,"userId":8}`, own.ID, own.ID, other.ID, DeleteOwnFilesConfirm)
	mixed := asFileRole(t, api.Deletes, body, 7, 2)
	if mixed["code"] == float64(0) {
		t.Fatalf("mixed request looked fully successful %#v", mixed)
	}
	if reasons := failureReasons(t, mixed); reasons[float64(other.ID)] != deleteReasonNotOwner || reasons[999999] != deleteReasonNotFound {
		t.Fatalf("reasons %#v payload %#v", reasons, mixed)
	}
	deleted := numberList(t, mixed, "deleted")
	if len(deleted) != 1 || deleted[0] != float64(own.ID) {
		t.Fatalf("deleted %#v", mixed["data"])
	}
	if liveFileCount(t, db, own.ID) != 0 {
		t.Fatal("owned file record remained")
	}
	if _, err := os.Stat(ownPath); !os.IsNotExist(err) {
		t.Fatalf("owned file remained: %v", err)
	}
	assertKept(t, db, other.ID, otherPath, "theirs")

	again := asFileRole(t, api.Deletes, fmt.Sprintf(`{"ids":[%d],"confirm":%q,"userId":%d}`, other.ID, DeleteOwnFilesConfirm, other.UserId), 7, 1)
	if again["code"] == float64(0) {
		t.Fatalf("administrator deleted another account %#v", again)
	}
	if reasons := failureReasons(t, again); reasons[float64(other.ID)] != deleteReasonNotOwner {
		t.Fatalf("admin reasons %#v", again)
	}
	assertKept(t, db, other.ID, otherPath, "theirs")
	var owner uint
	if err := db.Model(&models.File{}).Select("user_id").Where("id = ?", other.ID).Take(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if owner != 8 {
		t.Fatalf("owner changed to %d", owner)
	}
}

func TestDeleteOwnFilesDoesNotFollowPathsOutsideTheUploadRoot(t *testing.T) {
	db := openFileDB(t)
	root := t.TempDir()
	outsideDir := t.TempDir()
	restore := useConfig(t, "[base]\nsource_path="+root+"\n")
	t.Cleanup(restore)
	api := FileApi{}

	marker := []byte("OUTSIDE-MARKER-aa0d")
	outsideFile := filepath.Join(outsideDir, "secret.txt")
	if err := os.WriteFile(outsideFile, marker, 0o600); err != nil {
		t.Fatal(err)
	}
	// Keep the ".." in the stored value. filepath.Join would clean it away before the database sees it.
	traversal := root + "/../" + filepath.Base(outsideDir) + "/secret.txt"
	if filepath.Clean(traversal) != outsideFile {
		t.Fatalf("traversal fixture cleaned to %s, want %s", filepath.Clean(traversal), outsideFile)
	}
	escaped := addUpload(t, 7, "越界.png", traversal)
	absolute := addUpload(t, 7, "绝对路径.png", outsideFile)

	link := filepath.Join(root, "link.png")
	if err := os.Symlink(outsideFile, link); err != nil {
		t.Fatal(err)
	}
	linked := addUpload(t, 7, "链接.png", link)

	linkedDir := filepath.Join(root, "linked-dir")
	if err := os.Symlink(outsideDir, linkedDir); err != nil {
		t.Fatal(err)
	}
	throughDir := addUpload(t, 7, "目录链接.png", filepath.Join(linkedDir, "secret.txt"))

	goodPath := filepath.Join(root, "good.png")
	if err := os.WriteFile(goodPath, []byte("good"), 0o644); err != nil {
		t.Fatal(err)
	}
	good := addUpload(t, 7, "正常.png", goodPath)

	body := fmt.Sprintf(`{"ids":[%d,%d,%d,%d,%d],"confirm":%q}`, escaped.ID, absolute.ID, linked.ID, throughDir.ID, good.ID, DeleteOwnFilesConfirm)
	result := asFileUser(t, api.Deletes, body, 7)
	if result["code"] == float64(0) {
		t.Fatalf("unsafe paths were reported as full success %#v", result)
	}
	reasons := failureReasons(t, result)
	for _, id := range []uint{escaped.ID, absolute.ID, linked.ID, throughDir.ID} {
		if reasons[float64(id)] != deleteReasonUnsafePath {
			t.Fatalf("id %d reason %q payload %#v", id, reasons[float64(id)], result)
		}
		if liveFileCount(t, db, id) != 1 {
			t.Fatalf("unsafe row %d was deleted", id)
		}
	}
	secret, err := os.ReadFile(outsideFile)
	if err != nil || string(secret) != string(marker) {
		t.Fatalf("outside file changed: %q %v", secret, err)
	}
	info, err := os.Lstat(link)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink was removed: %v", err)
	}
	if _, err := os.Stat(goodPath); !os.IsNotExist(err) {
		t.Fatalf("safe file remained: %v", err)
	}
	if liveFileCount(t, db, good.ID) != 0 {
		t.Fatal("safe row remained")
	}
}

func TestDeleteOwnFilesReportsAFileRemovalFailure(t *testing.T) {
	db := openFileDB(t)
	root := t.TempDir()
	restore := useConfig(t, "[base]\nsource_path="+root+"\n")
	t.Cleanup(restore)
	api := FileApi{}
	blockedPath := filepath.Join(root, "blocked.png")
	otherPath := filepath.Join(root, "other.png")
	if err := os.WriteFile(blockedPath, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(otherPath, []byte("gone"), 0o644); err != nil {
		t.Fatal(err)
	}
	blocked := addUpload(t, 7, "删不掉.png", blockedPath)
	other := addUpload(t, 7, "删得掉.png", otherPath)
	missing := addUpload(t, 7, "已经不在磁盘.png", filepath.Join(root, "missing.png"))

	previous := removeUploadFile
	removeUploadFile = func(path string) error {
		if path == blockedPath {
			return errors.New("disk full")
		}
		return os.Remove(path)
	}
	t.Cleanup(func() { removeUploadFile = previous })

	result := asFileUser(t, api.Deletes, fmt.Sprintf(`{"ids":[%d,%d,%d],"confirm":%q}`, blocked.ID, other.ID, missing.ID, DeleteOwnFilesConfirm), 7)
	if result["code"] == float64(0) {
		t.Fatalf("file failure looked successful %#v", result)
	}
	reasons := failureReasons(t, result)
	if reasons[float64(blocked.ID)] != deleteReasonFile {
		t.Fatalf("blocked reason %#v", result)
	}
	assertKept(t, db, blocked.ID, blockedPath, "keep")
	deleted := numberList(t, result, "deleted")
	if len(deleted) != 2 {
		t.Fatalf("deleted %#v", result["data"])
	}
	if liveFileCount(t, db, other.ID) != 0 || liveFileCount(t, db, missing.ID) != 0 {
		t.Fatal("successful targets remained in the list")
	}
	if _, err := os.Stat(otherPath); !os.IsNotExist(err) {
		t.Fatalf("other file remained: %v", err)
	}
}

func addUpload(t *testing.T, userID uint, name, src string) models.File {
	t.Helper()
	saved, err := (&models.File{}).AddFile(userID, name, ".png", src, models.FileCategoryIcon)
	if err != nil {
		t.Fatal(err)
	}
	return saved
}

func confirmedIDs(ids ...uint) string {
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = fmt.Sprintf("%d", id)
	}
	return fmt.Sprintf(`{"ids":[%s],"confirm":%q}`, strings.Join(parts, ","), DeleteOwnFilesConfirm)
}

func assertKept(t *testing.T, db *gorm.DB, id uint, path, contents string) {
	t.Helper()
	if liveFileCount(t, db, id) != 1 {
		t.Fatalf("row %d was deleted", id)
	}
	body, err := os.ReadFile(path)
	if err != nil || string(body) != contents {
		t.Fatalf("file %s = %q %v", path, body, err)
	}
}

func liveFileCount(t *testing.T, db *gorm.DB, id uint) int64 {
	t.Helper()
	var count int64
	if err := db.Model(&models.File{}).Where("id = ?", id).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	return count
}

func numberList(t *testing.T, payload map[string]interface{}, key string) []float64 {
	t.Helper()
	data, _ := payload["data"].(map[string]interface{})
	raw, _ := data[key].([]interface{})
	out := make([]float64, 0, len(raw))
	for _, item := range raw {
		number, ok := item.(float64)
		if !ok {
			t.Fatalf("%s item %#v", key, item)
		}
		out = append(out, number)
	}
	return out
}

func failureReasons(t *testing.T, payload map[string]interface{}) map[float64]string {
	t.Helper()
	data, _ := payload["data"].(map[string]interface{})
	raw, _ := data["failed"].([]interface{})
	reasons := map[float64]string{}
	for _, item := range raw {
		row, _ := item.(map[string]interface{})
		id, _ := row["id"].(float64)
		reason, _ := row["reason"].(string)
		reasons[id] = reason
	}
	return reasons
}
