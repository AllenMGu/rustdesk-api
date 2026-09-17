package http

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/middleware"
	"github.com/lejianwen/rustdesk-api/v2/http/router"
	"github.com/sirupsen/logrus"
)

// applyTrustedProxyConfig 按配置设置 gin 的受信任代理列表。
// gin 1.9 的 New() 默认信任所有代理（0.0.0.0/0 与 ::/0），
// 任何客户端都能伪造 X-Forwarded-For 来绕过基于 ClientIP() 的限流/封禁。
// 未显式配置时改为“不信任任何代理”：ClientIP() 直接取 TCP 远端地址；
// 显式配置（逗号分隔的 IP/CIDR）时只信任列出的上游代理。
func applyTrustedProxyConfig(g *gin.Engine, trustProxy string) {
	if trustProxy != "" {
		pro := strings.Split(trustProxy, ",")
		if err := g.SetTrustedProxies(pro); err != nil {
			panic(err)
		}
		return
	}
	// SetTrustedProxies(nil) 后 isTrustedProxy 恒为 false，
	// ClientIP() 不再读取 X-Forwarded-For / X-Real-IP
	g.SetTrustedProxies(nil)
}

func ApiInit() {
	gin.SetMode(global.Config.Gin.Mode)
	g := gin.New()

	applyTrustedProxyConfig(g, global.Config.Gin.TrustProxy)

	if global.Config.Gin.Mode == gin.ReleaseMode {
		//修改gin Recovery日志 输出为logger的输出点
		if global.Logger != nil {
			gin.DefaultErrorWriter = global.Logger.WriterLevel(logrus.ErrorLevel)
		}
	}
	g.NoRoute(func(c *gin.Context) {
		c.String(http.StatusNotFound, "404 not found")
	})
	// 全局中间件必须挂在任何路由注册之前：gin 的 g.Use() 只对之后注册的
	// 路由生效。Cors() 放在这里保证管理端、API 端、WebClient 端点的跨域
	// 策略完全一致（此前挂在 ApiInit 内、admin 路由注册之后，admin 端点不受约束）。
	g.Use(middleware.Cors(), middleware.Logger(), middleware.Limiter(), gin.Recovery())
	router.WebInit(g)
	router.Init(g)
	router.ApiInit(g)
	Run(g, global.Config.Gin.ApiAddr)
}
