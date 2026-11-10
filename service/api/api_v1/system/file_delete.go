package system

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"sun-panel/models"

	"gorm.io/gorm"
)

const (
	// DeleteOwnFilesConfirm is the value a delete request must send after the user confirms.
	// A missing or different value deletes nothing.
	DeleteOwnFilesConfirm = "delete-own-files"
	// DeleteOwnFilesMaxIDs is the most ids one request may carry, counting duplicates.
	// A longer request is rejected before any file or row is removed.
	DeleteOwnFilesMaxIDs = 100
)

const (
	deleteReasonNotFound   = "not_found"
	deleteReasonNotOwner   = "not_owner"
	deleteReasonUnsafePath = "unsafe_path"
	deleteReasonFile       = "file"
	deleteReasonDatabase   = "database"
)

var (
	errUnsafeUploadPath = errors.New("unsafe upload path")
	errUploadFile       = errors.New("upload file remove failed")
	errUploadMissing    = errors.New("upload file missing")
	errDeleteNotOwned   = errors.New("row not owned at delete")
)

// removeUploadFile is the unlink used after a path has been checked.
// Tests replace it to simulate a disk error.
var removeUploadFile = os.Remove

type fileDeleteFailure struct {
	ID     uint   `json:"id"`
	Reason string `json:"reason"`
}

type fileDeleteResult struct {
	Deleted []uint              `json:"deleted"`
	Failed  []fileDeleteFailure `json:"failed"`
}

func newFileDeleteResult() fileDeleteResult {
	return fileDeleteResult{
		Deleted: []uint{},
		Failed:  []fileDeleteFailure{},
	}
}

// deleteOwnFiles removes files owned by userID.
// ids must already be non-empty and no longer than DeleteOwnFilesMaxIDs.
// Each id is decided on its own: another account's file is left untouched,
// and a failure on one id does not hide the result of the others.
func deleteOwnFiles(db *gorm.DB, userID uint, root string, ids []uint) (fileDeleteResult, error) {
	result := newFileDeleteResult()
	unique := uniqueDeleteIDs(ids)
	rows := []models.File{}
	if err := db.Where("id IN ?", unique).Find(&rows).Error; err != nil {
		return result, err
	}
	byID := make(map[uint]models.File, len(rows))
	for _, row := range rows {
		byID[row.ID] = row
	}
	rootAbs, rootReal, rootErr := resolveUploadRoot(root)

	for _, id := range unique {
		if id == 0 {
			result.Failed = append(result.Failed, fileDeleteFailure{ID: id, Reason: deleteReasonNotFound})
			continue
		}
		row, ok := byID[id]
		if !ok {
			result.Failed = append(result.Failed, fileDeleteFailure{ID: id, Reason: deleteReasonNotFound})
			continue
		}
		if row.UserId != userID {
			result.Failed = append(result.Failed, fileDeleteFailure{ID: id, Reason: deleteReasonNotOwner})
			continue
		}
		if rootErr != nil {
			result.Failed = append(result.Failed, fileDeleteFailure{ID: id, Reason: deleteReasonUnsafePath})
			continue
		}
		switch err := removeStoredUpload(rootAbs, rootReal, row.Src); {
		case err == nil, errors.Is(err, errUploadMissing):
			if delErr := deleteOwnedRow(db, userID, id); delErr != nil {
				reason := deleteReasonDatabase
				if errors.Is(delErr, errDeleteNotOwned) {
					reason = deleteReasonNotOwner
				}
				result.Failed = append(result.Failed, fileDeleteFailure{ID: id, Reason: reason})
				continue
			}
			result.Deleted = append(result.Deleted, id)
		case errors.Is(err, errUnsafeUploadPath):
			result.Failed = append(result.Failed, fileDeleteFailure{ID: id, Reason: deleteReasonUnsafePath})
		default:
			result.Failed = append(result.Failed, fileDeleteFailure{ID: id, Reason: deleteReasonFile})
		}
	}
	return result, nil
}

func uniqueDeleteIDs(ids []uint) []uint {
	seen := make(map[uint]struct{}, len(ids))
	unique := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		unique = append(unique, id)
	}
	return unique
}

func deleteOwnedRow(db *gorm.DB, userID, id uint) error {
	deleted := db.Where("user_id = ? AND id = ?", userID, id).Delete(&models.File{})
	if deleted.Error != nil {
		return deleted.Error
	}
	if deleted.RowsAffected == 0 {
		return errDeleteNotOwned
	}
	return nil
}

func resolveUploadRoot(root string) (string, string, error) {
	if strings.TrimSpace(root) == "" || strings.ContainsRune(root, 0) {
		return "", "", errUnsafeUploadPath
	}
	abs, err := filepath.Abs(filepath.Clean(root))
	if err != nil {
		return "", "", errUnsafeUploadPath
	}
	real, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", "", errUnsafeUploadPath
	}
	info, err := os.Stat(real)
	if err != nil || !info.IsDir() {
		return "", "", errUnsafeUploadPath
	}
	return abs, real, nil
}

// removeStoredUpload unlinks one stored path.
// The path is the database value, never a path taken from the request.
// It must stay inside the upload root. A symlink component is refused,
// so a link cannot redirect the unlink onto a file outside that root.
// A missing file inside the root returns errUploadMissing and is not an unlink of something else.
func removeStoredUpload(rootAbs, rootReal, stored string) error {
	if strings.TrimSpace(stored) == "" || strings.ContainsRune(stored, 0) {
		return errUnsafeUploadPath
	}
	cleaned := filepath.Clean(stored)
	abs := cleaned
	if !filepath.IsAbs(abs) {
		var err error
		abs, err = filepath.Abs(cleaned)
		if err != nil {
			return errUnsafeUploadPath
		}
	}
	rel, err := filepath.Rel(rootAbs, abs)
	if err != nil || rel == "." || rel == "" || uploadPathEscapes(rel) {
		return errUnsafeUploadPath
	}
	current := rootAbs
	parts := strings.Split(rel, string(filepath.Separator))
	var final os.FileInfo
	for i, part := range parts {
		if part == "" || part == "." || part == ".." {
			return errUnsafeUploadPath
		}
		current = filepath.Join(current, part)
		info, statErr := os.Lstat(current)
		if statErr != nil {
			if os.IsNotExist(statErr) {
				return errUploadMissing
			}
			return errUploadFile
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return errUnsafeUploadPath
		}
		if i < len(parts)-1 {
			if !info.IsDir() {
				return errUnsafeUploadPath
			}
			continue
		}
		if !info.Mode().IsRegular() {
			return errUnsafeUploadPath
		}
		final = info
	}
	if final == nil {
		return errUnsafeUploadPath
	}
	resolved, err := filepath.EvalSymlinks(current)
	if err != nil {
		return errUnsafeUploadPath
	}
	resolvedRel, err := filepath.Rel(rootReal, resolved)
	if err != nil || resolvedRel == "." || resolvedRel == "" || uploadPathEscapes(resolvedRel) {
		return errUnsafeUploadPath
	}
	if err := removeUploadFile(current); err != nil {
		if os.IsNotExist(err) {
			return errUploadMissing
		}
		return errUploadFile
	}
	return nil
}

func uploadPathEscapes(rel string) bool {
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return true
	}
	return strings.HasPrefix(filepath.ToSlash(rel), "../")
}
