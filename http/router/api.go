package router

import (
	"github.com/gin-gonic/gin"
	_ "github.com/lejianwen/rustdesk-api/v2/docs/api"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/controller/api"
	"github.com/lejianwen/rustdesk-api/v2/http/middleware"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"net/http"
)

// maxAnonBodyBytes 匿名端点请求体大小上限（128 KiB）。
// 正常客户端报文都很短（sysinfo 为若干短字段，audit 为短 JSON），
// 超限即视为异常数据，在 JSON 解析前由 http.MaxBytesReader 强制拒绝，
// 防止超大报文耗尽内存/CPU。
const maxAnonBodyBytes = 128 * 1024

func ApiInit(g *gin.Engine) {

	// 跨域中间件统一挂载在 http.ApiInit() 的全局中间件链上（所有路由注册之前），
	// 这里不再重复挂载，保证管理端 / API 端跨域策略一致。
	//swagger
	if global.Config.App.ShowSwagger == 1 {
		g.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler, ginSwagger.InstanceName("api")))
	}
	// 加载 HTML 模板
	g.LoadHTMLGlob("resources/templates/*")

	frg := g.Group("/api")

	{
		i := &api.Index{}
		frg.GET("/", i.Index)
		frg.GET("/version", i.Version)

		// 心跳同样属于匿名写入端点：限制请求体大小 + 控制器内做 (id, uuid) 身份校验
		frg.POST("/heartbeat", middleware.BodyLimit(maxAnonBodyBytes), i.Heartbeat)
	}

	{
		l := &api.Login{}
		// 如果返回oidc则可以通过oidc登录
		frg.GET("/login-options", l.LoginOptions)
		frg.POST("/login", middleware.BodyLimit(maxAnonBodyBytes), l.Login)

	}

	{
		o := &api.Oauth{}
		// [method:POST] [uri:/api/oidc/auth]
		frg.POST("/oidc/auth", o.OidcAuth)
		// [method:GET] [uri:/api/oidc/auth-query?code=abc&id=xxxxx&uuid=xxxxx]
		frg.GET("/oidc/auth-query", o.OidcAuthQuery)
		//api/oauth/callback
		frg.GET("/oauth/callback", o.OauthCallback)
		frg.GET("/oauth/login", o.OauthCallback)
		frg.GET("/oauth/msg", o.Message)

		frg.GET("/oidc/callback", o.OauthCallback)
		frg.GET("/oidc/login", o.OauthCallback)
		frg.GET("/oidc/msg", o.Message)
	}
	{
		pe := &api.Peer{}
		//提交系统信息（限请求体大小，防止超大报文）
		frg.POST("/sysinfo", middleware.BodyLimit(maxAnonBodyBytes), pe.SysInfo)
		frg.POST("/sysinfo_ver", pe.SysInfoVer)
	}

	if global.Config.App.WebClient == 1 {
		WebClientRoutes(frg)
	}

	{
		au := &api.Audit{}
		//[method:POST] [uri:/api/audit/conn]
		frg.POST("/audit/conn", middleware.BodyLimit(maxAnonBodyBytes), au.AuditConn)
		//[method:POST] [uri:/api/audit/file]
		frg.POST("/audit/file", middleware.BodyLimit(maxAnonBodyBytes), au.AuditFile)
	}

	frg.Use(middleware.RustAuth())
	{
		u := &api.User{}
		frg.GET("/user/info", u.Info)
		frg.POST("/currentUser", u.Info)
	}
	{
		l := &api.Login{}
		frg.POST("/logout", l.Logout)
	}
	{
		gr := &api.Group{}
		frg.GET("/users", gr.Users)
		frg.GET("/peers", gr.Peers)
		// /api/device-group/accessible?current=1&pageSize=100
		frg.GET("/device-group/accessible", gr.Device)
	}

	{
		ab := &api.Ab{}
		//获取地址
		frg.GET("/ab", ab.Ab)
		//更新地址
		frg.POST("/ab", ab.UpAb)
	}

	PersonalRoutes(frg)
	//访问静态文件
	g.StaticFS("/upload", http.Dir(global.Config.Gin.ResourcesPath+"/public/upload"))
}

func PersonalRoutes(frg *gin.RouterGroup) {
	{
		ab := &api.Ab{}
		frg.POST("/ab/personal", ab.Personal)
		//[method:POST] [uri:/api/ab/settings] Request
		frg.POST("/ab/settings", ab.Settings)
		// [method:POST] [uri:/api/ab/shared/profiles?current=1&pageSize=100]
		frg.POST("/ab/shared/profiles", ab.SharedProfiles)
		//[method:POST] [uri:/api/ab/peers?current=1&pageSize=100&ab=1]
		frg.POST("/ab/peers", ab.Peers)
		// [method:POST] [uri:/api/ab/tags/1]
		frg.POST("/ab/tags/:guid", ab.PTags)
		//[method:POST] api/ab/peer/add/1
		frg.POST("/ab/peer/add/:guid", ab.PeerAdd)
		//[method:DELETE] [uri:/api/ab/peer/1]
		frg.DELETE("/ab/peer/:guid", ab.PeerDel)
		//[method:PUT] [uri:/api/ab/peer/update/1]
		frg.PUT("/ab/peer/update/:guid", ab.PeerUpdate)
		//[method:POST] [uri:/api/ab/tag/add/1]
		frg.POST("/ab/tag/add/:guid", ab.TagAdd)
		//[method:PUT] [uri:/api/ab/tag/rename/1]
		frg.PUT("/ab/tag/rename/:guid", ab.TagRename)
		//[method:PUT] [uri:/api/ab/tag/update/1]
		frg.PUT("/ab/tag/update/:guid", ab.TagUpdate)
		//[method:DELETE] [uri:/api/ab/tag/1]
		frg.DELETE("/ab/tag/:guid", ab.TagDel)

	}

}

func WebClientRoutes(frg *gin.RouterGroup) {
	w := &api.WebClient{}
	{
		frg.POST("/shared-peer", middleware.BodyLimit(maxAnonBodyBytes), w.SharedPeer)
	}
	{
		frg.POST("/server-config", middleware.RustAuth(), w.ServerConfig)
	}

}
