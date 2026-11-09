package system

import (
	"fmt"
	"os"
	"path"
	"strings"
	"sun-panel/api/api_v1/common/apiData/commonApiStructs"
	"sun-panel/api/api_v1/common/apiReturn"
	"sun-panel/api/api_v1/common/base"
	"sun-panel/global"
	"sun-panel/lib/cmn"
	"sun-panel/models"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"gorm.io/gorm"
)

type FileApi struct{}

func (a *FileApi) UploadImg(c *gin.Context) {
	userInfo, ok := base.GetCurrentUserInfo(c)
	if !ok || userInfo.ID == 0 {
		apiReturn.ErrorByCode(c, 1000)
		return
	}
	category, valid := uploadCategory(c.PostForm("category"))
	if !valid {
		apiReturn.ErrorByCode(c, 1400)
		return
	}
	configUpload := global.Config.GetValueString("base", "source_path")
	f, err := c.FormFile("imgfile")
	if err != nil {
		apiReturn.ErrorByCode(c, 1300)
		return
	} else {
		if !allowedImageExt(f.Filename) {
			apiReturn.ErrorByCode(c, 1301)
			return
		}
		fileExt := strings.ToLower(path.Ext(f.Filename))
		fileName := cmn.Md5(fmt.Sprintf("%s%s", f.Filename, time.Now().String()))
		fildDir := fmt.Sprintf("%s/%d/%d/%d/", configUpload, time.Now().Year(), time.Now().Month(), time.Now().Day())
		isExist, _ := cmn.PathExists(fildDir)
		if !isExist {
			os.MkdirAll(fildDir, os.ModePerm)
		}
		filepath := fmt.Sprintf("%s%s%s", fildDir, fileName, fileExt)
		c.SaveUploadedFile(f, filepath)

		// 像数据库添加记录
		mFile := models.File{}
		mFile.AddFile(userInfo.ID, f.Filename, fileExt, filepath, category)
		apiReturn.SuccessData(c, gin.H{
			"imageUrl": filepath[1:],
		})
	}
}

func (a *FileApi) UploadFiles(c *gin.Context) {
	userInfo, ok := base.GetCurrentUserInfo(c)
	if !ok || userInfo.ID == 0 {
		apiReturn.ErrorByCode(c, 1000)
		return
	}
	category, valid := uploadCategory(c.PostForm("category"))
	if !valid {
		apiReturn.ErrorByCode(c, 1400)
		return
	}
	// A user id in the form cannot assign these files to another account or mark them public.
	_ = c.PostForm("userId")
	configUpload := global.Config.GetValueString("base", "source_path")

	form, err := c.MultipartForm()
	if err != nil {
		apiReturn.ErrorByCode(c, 1300)
		return
	}
	files := form.File["files[]"]
	if len(files) == 0 {
		apiReturn.ErrorByCode(c, 1300)
		return
	}
	errFiles := []string{}
	succMap := map[string]string{}
	saved := []map[string]string{}
	for i, f := range files {
		if !allowedImageExt(f.Filename) {
			errFiles = append(errFiles, f.Filename)
			continue
		}
		fileExt := strings.ToLower(path.Ext(f.Filename))
		fileName := cmn.Md5(fmt.Sprintf("%s%s%d", f.Filename, time.Now().String(), i))
		fildDir := fmt.Sprintf("%s/%d/%d/%d/", configUpload, time.Now().Year(), time.Now().Month(), time.Now().Day())
		isExist, _ := cmn.PathExists(fildDir)
		if !isExist {
			os.MkdirAll(fildDir, os.ModePerm)
		}
		filepath := fmt.Sprintf("%s%s%s", fildDir, fileName, fileExt)
		if c.SaveUploadedFile(f, filepath) != nil {
			errFiles = append(errFiles, f.Filename)
			continue
		}
		if _, err := (&models.File{}).AddFile(userInfo.ID, f.Filename, fileExt, filepath, category); err != nil {
			os.Remove(filepath)
			errFiles = append(errFiles, f.Filename)
			continue
		}
		imageURL := ""
		if len(filepath) > 0 {
			imageURL = filepath[1:]
		}
		succMap[f.Filename] = imageURL
		saved = append(saved, map[string]string{
			"fileName": f.Filename,
			"imageUrl": imageURL,
			"category": category,
		})
	}

	apiReturn.SuccessData(c, gin.H{
		"succMap":  succMap,
		"saved":    saved,
		"errFiles": errFiles,
	})
}

func (a *FileApi) GetList(c *gin.Context) {
	userInfo, ok := base.GetCurrentUserInfo(c)
	if !ok || userInfo.ID == 0 {
		apiReturn.ErrorByCode(c, 1000)
		return
	}
	req := models.FileListQuery{}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
			apiReturn.ErrorParamFomat(c, err.Error())
			return
		}
	}
	// An account id in the body cannot list another account, including that account's public images.
	query, err := models.NormalizeFileListQuery(req.Page, req.Limit, req.Category, req.Keyword)
	if err != nil {
		apiReturn.ErrorByCode(c, 1400)
		return
	}

	var count int64
	if err := models.FileListScope(global.Db, userInfo.ID, query).Count(&count).Error; err != nil {
		apiReturn.ErrorDatabase(c, err.Error())
		return
	}
	list := []models.File{}
	offset := (query.Page - 1) * query.Limit
	if err := models.FileListScope(global.Db, userInfo.ID, query).Order("created_at desc").Offset(offset).Limit(query.Limit).Find(&list).Error; err != nil {
		apiReturn.ErrorDatabase(c, err.Error())
		return
	}

	data := []map[string]interface{}{}
	for _, v := range list {
		src := v.Src
		if len(src) > 0 {
			src = src[1:]
		}
		data = append(data, map[string]interface{}{
			"src":        src,
			"fileName":   v.FileName,
			"id":         v.ID,
			"createTime": v.CreatedAt,
			"updateTime": v.UpdatedAt,
			"path":       v.Src,
			"category":   models.DisplayFileCategory(v.Category),
			"public":     v.IsPublic,
		})
	}
	apiReturn.SuccessListData(c, data, count)
}

func (a *FileApi) PublicList(c *gin.Context) {
	userInfo, ok := base.GetCurrentUserInfo(c)
	if !ok || userInfo.ID == 0 {
		apiReturn.ErrorByCode(c, 1000)
		return
	}
	req := struct {
		Page     int    `json:"page"`
		Limit    int    `json:"limit"`
		Category string `json:"category"`
		Keyword  string `json:"keyword"`
		UserId   uint   `json:"userId"`
	}{}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
			apiReturn.ErrorParamFomat(c, err.Error())
			return
		}
	}
	// userId in the body cannot choose another account. The list is every public image.
	_ = req.UserId
	query, err := models.NormalizeFileListQuery(req.Page, req.Limit, req.Category, req.Keyword)
	if err != nil {
		apiReturn.ErrorByCode(c, 1400)
		return
	}

	var count int64
	if err := models.PublicFileListScope(global.Db, query).Count(&count).Error; err != nil {
		apiReturn.ErrorDatabase(c, err.Error())
		return
	}
	list := []models.File{}
	offset := (query.Page - 1) * query.Limit
	if err := models.PublicFileListScope(global.Db, query).Order("created_at desc").Offset(offset).Limit(query.Limit).Find(&list).Error; err != nil {
		apiReturn.ErrorDatabase(c, err.Error())
		return
	}
	data := []map[string]interface{}{}
	for _, v := range list {
		src := v.Src
		if len(src) > 0 {
			src = src[1:]
		}
		data = append(data, map[string]interface{}{
			"src":        src,
			"fileName":   v.FileName,
			"id":         v.ID,
			"createTime": v.CreatedAt,
			"category":   models.DisplayFileCategory(v.Category),
			"public":     true,
		})
	}
	apiReturn.SuccessListData(c, data, count)
}

func (a *FileApi) SetPublic(c *gin.Context) {
	userInfo, ok := base.GetCurrentUserInfo(c)
	if !ok || userInfo.ID == 0 {
		apiReturn.ErrorByCode(c, 1000)
		return
	}
	if userInfo.Role != 1 {
		apiReturn.ErrorByCode(c, 1005)
		return
	}
	req := struct {
		ID     uint `json:"id"`
		Public bool `json:"public"`
		UserId uint `json:"userId"`
	}{}
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}
	// userId in the body cannot publish another account's file or change the owner.
	_ = req.UserId
	updated, err := (&models.File{}).SetOwnPublic(global.Db, userInfo.ID, req.ID, req.Public)
	if err != nil {
		apiReturn.ErrorDatabase(c, err.Error())
		return
	}
	if !updated {
		apiReturn.ErrorDataNotFound(c)
		return
	}
	apiReturn.Success(c)
}

func (a *FileApi) UpdateCategory(c *gin.Context) {
	userInfo, ok := base.GetCurrentUserInfo(c)
	if !ok || userInfo.ID == 0 {
		apiReturn.ErrorByCode(c, 1000)
		return
	}
	req := struct {
		ID       uint   `json:"id"`
		Category string `json:"category"`
	}{}
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}
	updated, err := (&models.File{}).UpdateOwnCategory(global.Db, userInfo.ID, req.ID, req.Category)
	if err != nil {
		if err == models.ErrFileCategory {
			apiReturn.ErrorByCode(c, 1400)
			return
		}
		apiReturn.ErrorDatabase(c, err.Error())
		return
	}
	if !updated {
		apiReturn.ErrorDataNotFound(c)
		return
	}
	apiReturn.Success(c)
}

func (a *FileApi) Deletes(c *gin.Context) {
	req := commonApiStructs.RequestDeleteIds[uint]{}
	userInfo, ok := base.GetCurrentUserInfo(c)
	if !ok || userInfo.ID == 0 {
		apiReturn.ErrorByCode(c, 1000)
		return
	}
	if err := c.ShouldBindBodyWith(&req, binding.JSON); err != nil {
		apiReturn.ErrorParamFomat(c, err.Error())
		return
	}

	global.Db.Transaction(func(tx *gorm.DB) error {
		files := []models.File{}

		if err := tx.Order("created_at desc").Find(&files, "user_id=? AND id in ?", userInfo.ID, req.Ids).Error; err != nil {
			return err
		}

		for _, v := range files {
			os.Remove(v.Src)
		}

		if err := tx.Order("created_at desc").Delete(&files, "user_id=? AND id in ?", userInfo.ID, req.Ids).Error; err != nil {
			return err
		}

		return nil
	})

	apiReturn.Success(c)

}

func uploadCategory(raw string) (string, bool) {
	if strings.TrimSpace(raw) == "" {
		return models.FileCategoryUncategorized, true
	}
	return models.NormalizeFileCategory(raw)
}

func allowedImageExt(name string) bool {
	switch strings.ToLower(path.Ext(name)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".ico":
		return true
	default:
		return false
	}
}
