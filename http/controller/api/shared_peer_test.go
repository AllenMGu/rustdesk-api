package api

import (
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newSharedPeerEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.POST("/api/shared-peer", (&WebClient{}).SharedPeer)
	return engine
}

// TestSharedPeerMalformedBodyNoPanic 审查 P2：此前 (*j)["share_token"].(string)
// 无类型断言保护，{} 或非字符串 share_token 会直接 panic（Gin Recovery → 500，
// 可被用于 500/日志型 DoS）。现在必须返回 code:101 业务错误，而不是 500。
func TestSharedPeerMalformedBodyNoPanic(t *testing.T) {
	cases := []string{
		`{}`,
		`{"share_token":123}`,
		`{"share_token":["x"]}`,
		`not-json`,
	}
	for _, body := range cases {
		w := postJSON(t, newSharedPeerEngine(), "/api/shared-peer", body)
		if w.Code == 500 {
			t.Fatalf("body=%s: must not panic to 500, got 500 body=%q", body, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), `"code":101`) {
			t.Fatalf("body=%s: expected business error code 101, got %d %q", body, w.Code, w.Body.String())
		}
	}
}
