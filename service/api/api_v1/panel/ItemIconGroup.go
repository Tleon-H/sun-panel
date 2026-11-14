package panel

import (
	"encoding/json"
	"errors"
	"math"
	"sun-panel/api/api_v1/common/apiData/commonApiStructs"
	"sun-panel/api/api_v1/common/apiReturn"
	"sun-panel/api/api_v1/common/base"
	"sun-panel/global"
	"sun-panel/models"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
)

// itemIconGroupEditRequest 记住请求里有没有带 iconStyle。
// 没带时不改这一列，旧客户端编辑分组名称不会把已经设置的样式清掉。
type itemIconGroupEditRequest struct {
	models.ItemIconGroup
	iconStylePresent bool
}

func (r *itemIconGroupEditRequest) UnmarshalJSON(data []byte) error {
	if err := json.Unmarshal(data, &r.ItemIconGroup); err != nil {
		return err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	_, r.iconStylePresent = raw["iconStyle"]
	return nil
}

type ItemIconGroup struct {
}

func (a *ItemIconGroup) Edit(c *gin.Context) {
	userInfo, _ := base.GetCurrentUserInfo(c)
	req := itemIconGroupEditRequest{}

	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}
	if !models.ValidGroupIconStyle(req.IconStyle) {
		apiReturn.Error(c, "分组卡片样式只能选择跟随全局、详情图标或小图标")
		return
	}

	req.UserId = userInfo.ID

	if req.ID != 0 {
		updates := map[string]interface{}{
			"icon":        req.Icon,
			"title":       req.Title,
			"description": req.Description,
			"user_id":     userInfo.ID,
		}
		if req.Sort != 0 {
			updates["sort"] = req.Sort
		}
		if req.iconStylePresent {
			if req.IconStyle == nil {
				updates["icon_style"] = nil
			} else {
				updates["icon_style"] = *req.IconStyle
			}
		}
		result := global.Db.Model(&models.ItemIconGroup{}).
			Where("id = ? AND user_id = ?", req.ID, userInfo.ID).
			Updates(updates)
		if result.Error != nil {
			apiReturn.ErrorDatabase(c, result.Error.Error())
			return
		}
		var saved models.ItemIconGroup
		err := global.Db.Where("id = ? AND user_id = ?", req.ID, userInfo.ID).First(&saved).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			apiReturn.Error(c, "分组不存在")
			return
		}
		if err != nil {
			apiReturn.ErrorDatabase(c, err.Error())
			return
		}
		apiReturn.SuccessData(c, saved)
		return
	}

	if err := global.Db.Create(&req.ItemIconGroup).Error; err != nil {
		apiReturn.ErrorDatabase(c, err.Error())
		return
	}
	apiReturn.SuccessData(c, req.ItemIconGroup)
}

func (a *ItemIconGroup) GetList(c *gin.Context) {

	userInfo, _ := base.GetCurrentUserInfo(c)
	groups := []models.ItemIconGroup{}

	err := global.Db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Order("sort ,created_at").Where("user_id=?", userInfo.ID).Find(&groups).Error; err != nil {
			apiReturn.ErrorDatabase(c, err.Error())
			return err
		}

		// 判断分组是否为空，为空将自动创建默认分组
		if len(groups) == 0 {
			defaultGroup := models.ItemIconGroup{
				Title:  "APP",
				UserId: userInfo.ID,
				Icon:   "material-symbols:ad-group-outline",
			}
			if err := tx.Create(&defaultGroup).Error; err != nil {
				apiReturn.ErrorDatabase(c, err.Error())
				return err
			}

			// 并将当前账号下所有无分组的图标更新到当前组
			if err := tx.Model(&models.ItemIcon{}).Where("user_id=?", userInfo.ID).Update("item_icon_group_id", defaultGroup.ID).Error; err != nil {
				apiReturn.ErrorDatabase(c, err.Error())
				return err
			}

			groups = append(groups, defaultGroup)
		}

		// 返回 nil 提交事务
		return nil
	})

	if err != nil {
		apiReturn.ErrorDatabase(c, err.Error())
		return
	} else {
		apiReturn.SuccessListData(c, groups, 0)
	}
}

func (a *ItemIconGroup) Deletes(c *gin.Context) {
	req := commonApiStructs.RequestDeleteIds[uint]{}

	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}
	userInfo, _ := base.GetCurrentUserInfo(c)

	var count int64
	if err := global.Db.Model(&models.ItemIconGroup{}).Where(" user_id=?", userInfo.ID).Count(&count).Error; err != nil {
		apiReturn.ErrorDatabase(c, err.Error())
		return
	} else {
		if math.Abs(float64(len(req.Ids))-float64(count)) < 1 {
			apiReturn.ErrorCode(c, 1201, "At least one must be retained", nil)
			return
		}

	}

	txErr := global.Db.Transaction(func(tx *gorm.DB) error {
		mitemIcon := models.ItemIcon{}
		if err := tx.Delete(&models.ItemIconGroup{}, "id in ? AND user_id=?", req.Ids, userInfo.ID).Error; err != nil {
			return err
		}

		if err := mitemIcon.DeleteByItemIconGroupIds(tx, userInfo.ID, req.Ids); err != nil {
			return err
		}

		return nil
	})

	if txErr != nil {
		apiReturn.ErrorDatabase(c, txErr.Error())
		return
	}

	apiReturn.Success(c)
}

// 保存排序
func (a *ItemIconGroup) SaveSort(c *gin.Context) {
	req := commonApiStructs.SortRequest{}

	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}

	userInfo, _ := base.GetCurrentUserInfo(c)

	transactionErr := global.Db.Transaction(func(tx *gorm.DB) error {
		// 在事务中执行一些 db 操作（从这里开始，您应该使用 'tx' 而不是 'db'）
		for _, v := range req.SortItems {
			if err := tx.Model(&models.ItemIconGroup{}).Where("user_id=? AND id=?", userInfo.ID, v.Id).Update("sort", v.Sort).Error; err != nil {
				// 返回任何错误都会回滚事务
				return err
			}
		}

		// 返回 nil 提交事务
		return nil
	})

	if transactionErr != nil {
		apiReturn.ErrorDatabase(c, transactionErr.Error())
		return
	}

	apiReturn.Success(c)
}
