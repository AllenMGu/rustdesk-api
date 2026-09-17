package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
)

// corsAllowMethods 预检响应允许的方法固定列表。
// 不能回显请求自身方法（c.Request.Method）：预检请求是 OPTIONS，
// 回显 "OPTIONS" 会让浏览器认为目标方法不可跨域而拒绝后续实际请求。
const corsAllowMethods = "GET, POST, PUT, DELETE, OPTIONS"

// Cors 跨域
// 仅允许 gin.cors-origins 中显式配置（Origin 精确匹配）的来源进行跨域访问，
// 不再反射任意请求方 Origin，避免任意站点携带凭证调用本服务 API。
// 白名单为空时不向任何来源下发跨域响应头（等价于 CORS 关闭）。
// 预检（OPTIONS）直接返回 204：白名单内的来源携带完整放行头，
// 其余来源只得到 204 无放行头，浏览器按 CORS 失败处理。
func Cors() gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && originAllowed(origin) {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Headers", "api-token,content-type,authorization")
			c.Header("Access-Control-Allow-Methods", corsAllowMethods)
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// originAllowed 判断 Origin 是否在配置的白名单内（精确匹配）
func originAllowed(origin string) bool {
	for _, allowed := range global.Config.Gin.CorsOrigins {
		if allowed == origin {
			return true
		}
	}
	return false
}
