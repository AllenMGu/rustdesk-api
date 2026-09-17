package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// BodyLimit 限制请求体大小（字节），防止匿名端点被超大报文打爆内存/CPU。
// 通过 http.MaxBytesReader 在读取时强制上限：超限读取返回错误，
// 后续的 JSON 绑定/解码会失败并按参数错误处理（对客户端表现为 400/静默拒绝）。
func BodyLimit(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		if c.Request != nil && c.Request.Body != nil && maxBytes > 0 {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		}
		c.Next()
	}
}
