package system

import (
	"sun-panel/api/api_v1/common/apiReturn"
	"sun-panel/api/api_v1/common/base"
	"sun-panel/global"
	"sun-panel/models"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
)

type ModuleConfigApi struct{}

func (a *ModuleConfigApi) GetByName(c *gin.Context) {
	req := models.ModuleConfig{}

	if err := c.ShouldBindWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}

	userInfo, ok := base.GetCurrentUserInfo(c)
	if !ok || userInfo.ID == 0 {
		apiReturn.ErrorByCode(c, 1000)
		return
	}

	mCfg := models.ModuleConfig{}
	cfg, err := mCfg.GetConfigByUserIdAndName(global.Db, userInfo.ID, req.Name)
	if err != nil {
		apiReturn.ErrorDatabase(c, err.Error())
		return
	}
	if base.GetCurrentVisitMode(c) == base.VISIT_MODE_PUBLIC && req.Name == searchModuleConfigName {
		cfg = PublicSearchConfig(cfg)
	}
	apiReturn.SuccessData(c, cfg)

}

func (a *ModuleConfigApi) Save(c *gin.Context) {
	req := models.ModuleConfig{}
	if err := c.ShouldBindWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}
	userInfo, ok := base.GetCurrentUserInfo(c)
	if !ok || userInfo.ID == 0 {
		apiReturn.ErrorByCode(c, 1000)
		return
	}
	mCfg := models.ModuleConfig{}
	mCfg.UserId = userInfo.ID
	mCfg.Value = req.Value
	mCfg.Name = req.Name

	if err := mCfg.Save(global.Db); err != nil {
		apiReturn.ErrorDatabase(c, err.Error())
		return
	}
	apiReturn.Success(c)
}
