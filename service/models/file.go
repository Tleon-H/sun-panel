package models

import (
	"errors"
	"strings"

	"gorm.io/gorm"
)

const (
	FileCategoryUncategorized = "uncategorized"
	FileCategoryIcon          = "icon"
	FileCategoryWallpaper     = "wallpaper"
)

const (
	fileListDefaultLimit = 24
	fileListMaxLimit     = 60
	fileKeywordMaxLength = 80
)

var ErrFileCategory = errors.New("invalid file category")

type File struct {
	BaseModel
	Src      string `json:"src"`
	UserId   uint   `json:"userId" gorm:"index"`
	FileName string `json:"fileName" gorm:"varchar(255)"` // 文件名
	Method   int    `gorm:"int(5)" json:"method"`         // 上传方式
	Ext      string `gorm:"varchar(255)" json:"ext"`      // 扩展名
	// Category is metadata only. It never changes the stored path.
	Category string `json:"category" gorm:"type:varchar(32);not null;default:uncategorized"`
}

type FileListQuery struct {
	Page     int
	Limit    int
	Category string
	Keyword  string
}

// NormalizeFileCategory accepts the three stored values.
// An empty value is the historical uncategorized state.
func NormalizeFileCategory(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", FileCategoryUncategorized:
		return FileCategoryUncategorized, true
	case FileCategoryIcon:
		return FileCategoryIcon, true
	case FileCategoryWallpaper:
		return FileCategoryWallpaper, true
	default:
		return "", false
	}
}

// DisplayFileCategory maps a stored value for API clients.
// Rows created before the column existed, or left blank, stay uncategorized.
func DisplayFileCategory(raw string) string {
	category, ok := NormalizeFileCategory(raw)
	if !ok {
		return FileCategoryUncategorized
	}
	return category
}

func NormalizeFileListQuery(page, limit int, category, keyword string) (FileListQuery, error) {
	query := FileListQuery{Page: page, Limit: limit}
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.Limit <= 0 {
		query.Limit = fileListDefaultLimit
	}
	if query.Limit > fileListMaxLimit {
		query.Limit = fileListMaxLimit
	}
	query.Keyword = trimFileKeyword(keyword)
	if strings.TrimSpace(category) == "" {
		return query, nil
	}
	normalized, ok := NormalizeFileCategory(category)
	if !ok {
		return FileListQuery{}, ErrFileCategory
	}
	query.Category = normalized
	return query, nil
}

const fileNameLikeSQL = "file_name LIKE ? ESCAPE '!'"

func trimFileKeyword(keyword string) string {
	keyword = strings.TrimSpace(keyword)
	characters := []rune(keyword)
	if len(characters) > fileKeywordMaxLength {
		return string(characters[:fileKeywordMaxLength])
	}
	return keyword
}

func fileKeywordLike(keyword string) string {
	// '!' is one character in both SQLite and MySQL/MariaDB. A backslash escape is one
	// character in SQLite and an invalid or multi-character escape in MySQL.
	escaped := strings.NewReplacer(`!`, `!!`, `%`, `!%`, `_`, `!_`).Replace(keyword)
	return "%" + escaped + "%"
}

func FileListScope(db *gorm.DB, userID uint, query FileListQuery) *gorm.DB {
	scope := db.Model(&File{}).Where("user_id = ?", userID)
	switch query.Category {
	case "":
	case FileCategoryUncategorized:
		scope = scope.Where("(category = ? OR category = ? OR category IS NULL)", FileCategoryUncategorized, "")
	default:
		scope = scope.Where("category = ?", query.Category)
	}
	if query.Keyword != "" {
		scope = scope.Where(fileNameLikeSQL, fileKeywordLike(query.Keyword))
	}
	return scope
}

// AddFile records an upload. The path is stored unchanged.
// An unrecognized category is saved as uncategorized instead of failing after the bytes are written.
func (m *File) AddFile(userId uint, fileName, ext, src, category string) (File, error) {
	stored := DisplayFileCategory(category)
	file := File{
		UserId:   userId,
		FileName: fileName,
		Src:      src,
		Ext:      ext,
		Category: stored,
	}
	err := Db.Create(&file).Error
	return file, err
}

// UpdateOwnCategory changes only the category of a file owned by userID.
// A missing id and another account's id both affect zero rows.
func (m *File) UpdateOwnCategory(db *gorm.DB, userID, id uint, category string) (bool, error) {
	stored, ok := NormalizeFileCategory(category)
	if !ok {
		return false, ErrFileCategory
	}
	result := db.Model(&File{}).Where("user_id = ? AND id = ?", userID, id).Update("category", stored)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}
