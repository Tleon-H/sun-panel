package system

import (
	"errors"
	"io"
	"net/http"
	"os"
	"sun-panel/api/api_v1/common/apiReturn"
	"sun-panel/api/api_v1/common/base"
	"sun-panel/global"
	"sun-panel/lib/backup"
	"sun-panel/lib/cmn"

	"github.com/gin-gonic/gin"
)

type BackupApi struct{}

func (a *BackupApi) Export(c *gin.Context) {
	user, ok := base.GetCurrentUserInfo(c)
	if !ok || user.ID == 0 {
		apiReturn.ErrorByCode(c, 1000)
		return
	}
	if user.Role != 1 {
		apiReturn.ErrorByCode(c, 1005)
		return
	}
	if !backup.TryBegin() {
		apiReturn.Error(c, "已有备份正在导出，请稍后再试。")
		return
	}
	defer backup.End()

	wd, err := os.Getwd()
	if err != nil {
		apiReturn.Error(c, exportMessage(err))
		return
	}
	info := cmn.GetSysVersionInfo()
	a.writeExport(c, wd, info.Version, info.Version_code)
}

func (a *BackupApi) writeExport(c *gin.Context, baseDir, version string, versionCode int) {
	_, _ = io.Copy(io.Discard, io.LimitReader(c.Request.Body, 1<<20))
	if global.Config == nil {
		apiReturn.Error(c, exportMessage(backup.ErrSnapshot))
		return
	}
	drive := global.Config.GetValueStringOrDefault("base", "database_drive")
	if drive == "mysql" {
		apiReturn.Error(c, exportMessage(backup.ErrNotSQLite))
		return
	}
	if global.Db == nil {
		apiReturn.Error(c, exportMessage(backup.ErrSnapshot))
		return
	}
	sqlDB, err := global.Db.DB()
	if err != nil {
		apiReturn.Error(c, exportMessage(backup.ErrSnapshot))
		return
	}
	result, err := backup.Build(c.Request.Context(), backup.Options{
		BaseDir:       baseDir,
		DatabaseDrive: drive,
		UploadsPath:   global.Config.GetValueString("base", "source_path"),
		ConfFile:      "conf/conf.ini",
		CustomDir:     "web/custom",
		SQLitePath:    global.Config.GetValueStringOrDefault("sqlite", "file_path"),
		Version:       version,
		VersionCode:   versionCode,
		DB:            sqlDB,
	})
	if err != nil {
		apiReturn.Error(c, exportMessage(err))
		return
	}
	defer os.RemoveAll(result.Dir)

	file, err := os.Open(result.ZipPath)
	if err != nil {
		apiReturn.Error(c, exportMessage(err))
		return
	}
	defer file.Close()
	stat, err := file.Stat()
	if err != nil {
		apiReturn.Error(c, exportMessage(err))
		return
	}
	c.DataFromReader(http.StatusOK, stat.Size(), "application/zip", file, map[string]string{
		"Content-Disposition":    `attachment; filename="leon-panel-backup.zip"`,
		"Cache-Control":          "no-store",
		"X-Content-Type-Options": "nosniff",
	})
}

func exportMessage(err error) string {
	switch {
	case errors.Is(err, backup.ErrNotSQLite):
		return "当前使用的是 MySQL，这项功能不会导出数据库，也不会生成备份包。请自行备份 MySQL。"
	case errors.Is(err, backup.ErrOutside):
		return "配置的目录超出应用目录或包含不允许的路径，已拒绝导出。"
	case errors.Is(err, backup.ErrIncomplete):
		return "缺少导出所需的配置或文件，没有生成备份包。当前服务仍在运行。"
	case errors.Is(err, backup.ErrLimit):
		return "文件数量或总体积超过导出上限，没有生成备份包。当前服务仍在运行。"
	case errors.Is(err, os.ErrPermission):
		return "没有读取导出内容的权限，没有生成备份包。当前服务仍在运行。"
	case errors.Is(err, backup.ErrSnapshot):
		return "数据库快照失败，没有生成备份包。当前服务仍在运行。"
	default:
		return "备份导出失败，没有生成备份包。当前服务仍在运行。"
	}
}
