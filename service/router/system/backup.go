package system

import (
	"sun-panel/api/api_v1"
	"sun-panel/api/api_v1/middleware"

	"github.com/gin-gonic/gin"
)

func InitBackupRouter(router *gin.RouterGroup) {
	api := api_v1.ApiGroupApp.ApiSystem.BackupApi
	private := router.Group("", middleware.LoginInterceptor, middleware.AdminInterceptor)
	{
		private.POST("/backup/export", api.Export)
	}
}
