package router

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
	"github.com/lejianwen/rustdesk-api/v2/http/controller/web"
)

const legacyWebClientRedirectHTML = `<!doctype html>
<html><head><meta charset="utf-8"><title>Redirecting…</title></head>
<body><script>
const fragment = window.location.hash;
let targetFragment = fragment;
if (fragment.startsWith('#/') && !fragment.startsWith('#/?')) {
  const id = fragment.slice(2);
  targetFragment = id ? '#/?id=' + encodeURIComponent(id) : '';
}
window.location.replace('/webclient/' + targetFragment);
</script></body></html>`

func serveLegacyWebClientRedirect(c *gin.Context) {
	c.Data(http.StatusOK, "text/html; charset=utf-8", []byte(legacyWebClientRedirectHTML))
}

func WebInit(g *gin.Engine) {
	i := &web.Index{}
	g.GET("/", i.Index)

	if global.Config.App.WebClient == 1 {
		g.GET("/webclient-config/index.js", i.ConfigJs)
	}

	if global.Config.App.WebClient == 1 {
		g.StaticFS("/webclient", http.Dir(global.Config.Gin.ResourcesPath+"/web"))
		// Web Client v2 was removed upstream. Keep old cached admin links working
		// with a small fragment-aware redirect page. The fragment contains the
		// peer ID and is never sent to the server, so a normal HTTP redirect
		// cannot translate the old #/<id> format into #/?id=<id>.
		g.GET("/webclient2/*path", serveLegacyWebClientRedirect)
	}
	g.StaticFS("/_admin", http.Dir(global.Config.Gin.ResourcesPath+"/admin"))
}
