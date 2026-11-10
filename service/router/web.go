package router

import (
	"net/http"
	"strings"

	"sun-panel/global"
	"sun-panel/router/openness"
	"sun-panel/router/panel"
	"sun-panel/router/system"

	"github.com/gin-gonic/gin"
)

// hashFrontendRoutes 是 src/router/index.ts 里除首页和兜底匹配以外的具体页面。
// 前端使用 hash history，所以登录页的真实地址是 /#/login。
// 这里只给这些地址一个入口，不把未知路径、缺失文件或接口回成页面。
var hashFrontendRoutes = []string{"/login", "/404", "/500", "/test"}

// NewEngine 组装接口、静态文件和前端入口。sourcePath 仍按原规则去掉首字符后作为 URL 前缀。
func NewEngine(webPath, sourcePath string) *gin.Engine {
	engine := gin.Default()
	rootRouter := engine.Group("/")
	routerGroup := rootRouter.Group("api")

	system.Init(routerGroup)
	panel.Init(routerGroup)
	openness.Init(routerGroup)

	engine.StaticFile("/", webPath+"/index.html")
	engine.Static("/assets", webPath+"/assets")
	engine.Static("/custom", webPath+"/custom")
	engine.StaticFile("/favicon.ico", webPath+"/favicon.ico")
	engine.StaticFile("/favicon.svg", webPath+"/favicon.svg")
	engine.Static(sourcePath[1:], sourcePath)

	for _, route := range hashFrontendRoutes {
		registerHashEntry(engine, route)
	}
	return engine
}

func registerHashEntry(engine *gin.Engine, route string) {
	engine.Any(route, func(c *gin.Context) {
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.Header("Allow", "GET, HEAD")
			c.Status(http.StatusMethodNotAllowed)
			return
		}
		location, ok := hashLocation(route, c.Request.URL.RawQuery)
		if !ok {
			c.Status(http.StatusBadRequest)
			return
		}
		c.Header("Location", location)
		c.Status(http.StatusFound)
	})
}

func hashLocation(route, rawQuery string) (string, bool) {
	if strings.ContainsAny(rawQuery, "\r\n\x00") {
		return "", false
	}
	location := "/#" + route
	if rawQuery != "" {
		location += "?" + rawQuery
	}
	return location, true
}

// InitRouters 保持原来的监听地址由调用方传入，这里不改绑定方式。
func InitRouters(addr string) error {
	engine := NewEngine("./web", global.Config.GetValueString("base", "source_path"))
	global.Logger.Info("Leon Panel is started. Listening and serving HTTP on ", addr)
	return engine.Run(addr)
}
