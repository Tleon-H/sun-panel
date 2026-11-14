package models

import (
	"errors"

	"gorm.io/gorm"
)

const (
	// GroupIconStyleInfo 是详情大卡片，与前端 PanelPanelConfigStyleEnum.info 相同。
	GroupIconStyleInfo = 0
	// GroupIconStyleIcon 是小图标，与前端 PanelPanelConfigStyleEnum.icon 相同。
	GroupIconStyleIcon = 1
)

type ItemIconGroup struct {
	BaseModel
	Icon        string `json:"icon"`
	Title       string `gorm:"type:varchar(50)" json:"title"`
	Description string `gorm:"type:varchar(1000)" json:"description"`
	Sort        int    `gorm:"type:int(11)" json:"sort"`
	UserId      uint   `json:"userId"`
	User        User   `json:"user"`
	// IconStyle 为空表示跟随该账号的全局图标样式。0 是详情大卡片，1 是小图标。
	IconStyle *int `gorm:"column:icon_style;type:int" json:"iconStyle"`
}

// ValidGroupIconStyle 接受空值、详情大卡片和小图标。空值表示跟随全局。
func ValidGroupIconStyle(style *int) bool {
	if style == nil {
		return true
	}
	switch *style {
	case GroupIconStyleInfo, GroupIconStyleIcon:
		return true
	default:
		return false
	}
}

// EnsureItemIconGroupIconStyle 给已有的分组表补上可空的 icon_style。
// 旧行保持为空，因此仍跟随全局样式。表还不存在时不做处理，交给启动时的建表。
func EnsureItemIconGroupIconStyle(db *gorm.DB) error {
	if db == nil {
		return errors.New("database is not open")
	}
	if !db.Migrator().HasTable(&ItemIconGroup{}) {
		return nil
	}
	if db.Migrator().HasColumn(&ItemIconGroup{}, "IconStyle") {
		return nil
	}
	return db.Migrator().AddColumn(&ItemIconGroup{}, "IconStyle")
}

func (m *ItemIconGroup) DeleteByUserId(db *gorm.DB, userId uint) (err error) {
	err = db.Delete(&ItemIconGroup{}, "user_id = ?", userId).Error
	return
}
