package system

import (
	"errors"
	"io"
	"net/http"
	"os"
	"strings"
	"sun-panel/api/api_v1/common/apiReturn"
	"sun-panel/api/api_v1/common/base"
	"sun-panel/global"
	"sun-panel/lib/backup"
	"sun-panel/lib/cmn"
	"sun-panel/models"

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
		apiReturn.Error(c, "已有备份正在导出或恢复，请稍后再试。")
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

func (a *BackupApi) Restore(c *gin.Context) {
	user, ok := base.GetCurrentUserInfo(c)
	if !ok || user.ID == 0 {
		apiReturn.ErrorByCode(c, 1000)
		return
	}
	if user.Role != 1 {
		apiReturn.ErrorByCode(c, 1005)
		return
	}
	wd, err := os.Getwd()
	if err != nil {
		apiReturn.Error(c, restoreMessage(err))
		return
	}
	info := cmn.GetSysVersionInfo()
	a.restoreAt(c, wd, info.Version, info.Version_code)
}

func (a *BackupApi) restoreAt(c *gin.Context, baseDir, version string, versionCode int) {
	user, ok := base.GetCurrentUserInfo(c)
	if !ok || user.ID == 0 {
		apiReturn.ErrorByCode(c, 1000)
		return
	}
	if user.Role != 1 {
		apiReturn.ErrorByCode(c, 1005)
		return
	}
	if c.Request != nil && c.Request.Body != nil {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, (512<<20)+(1<<20))
	}
	if strings.TrimSpace(c.PostForm("confirm")) != backup.ConfirmOverwrite {
		apiReturn.Error(c, "恢复会覆盖现有数据。需要再次确认后才会执行，这次没有修改任何内容。")
		return
	}
	if global.Config == nil {
		apiReturn.Error(c, restoreMessage(backup.ErrSnapshot))
		return
	}
	drive := global.Config.GetValueStringOrDefault("base", "database_drive")
	if strings.EqualFold(drive, "mysql") {
		apiReturn.Error(c, restoreMessage(backup.ErrNotSQLite))
		return
	}
	if !backup.TryBegin() {
		apiReturn.Error(c, "已有备份正在导出或恢复，请稍后再试。")
		return
	}
	defer backup.End()
	if global.Db == nil {
		apiReturn.Error(c, restoreMessage(backup.ErrSnapshot))
		return
	}
	file, err := c.FormFile("file")
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			apiReturn.Error(c, restoreMessage(backup.ErrLimit))
			return
		}
		apiReturn.Error(c, "没有收到备份包，没有改动现有数据。")
		return
	}
	opened, err := file.Open()
	if err != nil {
		apiReturn.Error(c, restoreMessage(err))
		return
	}
	defer opened.Close()
	sqlDB, err := global.Db.DB()
	if err != nil {
		apiReturn.Error(c, restoreMessage(backup.ErrSnapshot))
		return
	}
	err = backup.Restore(c.Request.Context(), backup.RestoreOptions{
		BaseDir:       baseDir,
		DatabaseDrive: drive,
		UploadsPath:   global.Config.GetValueString("base", "source_path"),
		ConfFile:      "conf/conf.ini",
		CustomDir:     "web/custom",
		SQLitePath:    global.Config.GetValueStringOrDefault("sqlite", "file_path"),
		Version:       version,
		VersionCode:   versionCode,
		DB:            sqlDB,
	}, opened)
	if err != nil {
		apiReturn.Error(c, restoreMessage(err))
		return
	}
	// 旧备份没有分组卡片样式这一列。补上可空列后，那些分组继续跟随全局样式。
	if err := models.EnsureItemIconGroupIconStyle(global.Db); err != nil {
		apiReturn.Error(c, "备份已经覆盖，但分组卡片样式字段没有补上。请重启后再试。当前服务仍在运行。")
		return
	}
	apiReturn.Success(c)
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

func restoreMessage(err error) string {
	switch {
	case errors.Is(err, backup.ErrNotSQLite):
		return "当前使用的是 MySQL，这项功能不会恢复数据库，也不会覆盖文件。请使用 MySQL 自己的备份。"
	case errors.Is(err, backup.ErrIncompatible):
		return "这不是本程序可恢复的备份包。只接受本程序自己导出、显示版本不超过当前版本、数据库结构仍是内部版本 10 的 SQLite 包，并且包里的数据库路径和上传目录必须与当前配置一致。已拒绝官方或其他不兼容的备份，没有改动现有数据。"
	case errors.Is(err, backup.ErrChecksum):
		return "备份包的文件清单或 SHA-256 校验没有通过，没有改动现有数据。"
	case errors.Is(err, backup.ErrPackage):
		return "备份包缺少清单或内容无法读取，没有改动现有数据。"
	case errors.Is(err, backup.ErrOutside):
		return "备份包里的路径超出应用目录，或包含符号链接和路径穿越，已拒绝恢复，没有改动现有数据。"
	case errors.Is(err, backup.ErrLimit):
		return "备份包超过大小或文件数量上限，没有改动现有数据。当前服务仍在运行。"
	case errors.Is(err, backup.ErrIncomplete):
		return "备份包不完整，没有改动现有数据。当前服务仍在运行。"
	case errors.Is(err, backup.ErrSnapshot):
		return "数据库快照校验失败，没有改动现有数据。当前服务仍在运行。"
	case errors.Is(err, backup.ErrRollback):
		return "恢复失败，而且回滚也没有完成。请先检查数据后再继续使用。当前服务仍在运行。"
	case errors.Is(err, backup.ErrApply):
		return "恢复没有完成，已退回覆盖前的数据。当前服务仍在运行。"
	default:
		return "恢复失败，没有关闭正在运行的服务。"
	}
}
