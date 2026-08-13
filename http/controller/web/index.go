package web

import (
	"fmt"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
)

type Index struct {
}

func (i *Index) Index(c *gin.Context) {
	c.Redirect(302, "/_admin/")
}

func (i *Index) ConfigJs(c *gin.Context) {
	apiServer := global.Config.Rustdesk.ApiServer
	tmp := fmt.Sprintf("localStorage.setItem('api-server', %s);\n", strconv.Quote(apiServer))

	c.Header("Content-Type", "application/javascript")
	c.String(200, tmp)
}
