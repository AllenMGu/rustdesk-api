package admin

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
)

const defaultRdgenInternalURL = "http://127.0.0.1:8000"
const defaultRdgenClientDefaultsFile = "/app/data/rdgen-client-defaults.json"

// Rdgen securely exposes the co-located client generator through rustdesk-api.
// Every endpoint is protected by the existing backend login and administrator
// middleware. GitHub Actions never connects back to this service.
type Rdgen struct{}

func serveRdgenClientDefaults(c *gin.Context) {
	path := strings.TrimSpace(os.Getenv("RDGEN_CLIENT_DEFAULTS_FILE"))
	if path == "" {
		path = defaultRdgenClientDefaultsFile
	}

	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		c.JSON(http.StatusOK, gin.H{"defaults": gin.H{}})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Unable to read client defaults"})
		return
	}
	if len(content) > 1024*1024 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Client defaults file is too large"})
		return
	}

	defaults := map[string]any{}
	if err := json.Unmarshal(content, &defaults); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Client defaults file is not valid JSON"})
		return
	}

	for _, field := range []string{
		"csrfmiddlewaretoken",
		"permanentPassword",
		"unlockPin",
		"sh_secret_field",
	} {
		delete(defaults, field)
	}
	c.JSON(http.StatusOK, gin.H{"defaults": defaults})
}

func rdgenTarget() (*url.URL, error) {
	target := strings.TrimSpace(os.Getenv("RDGEN_INTERNAL_URL"))
	if target == "" {
		target = defaultRdgenInternalURL
	}
	return url.Parse(target)
}

func proxyRdgen(c *gin.Context, targetPath string) {
	target, err := rdgenTarget()
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Invalid internal generator URL"})
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	originalDirector := proxy.Director
	proxy.Director = func(req *http.Request) {
		proto := c.GetHeader("X-Forwarded-Proto")
		if proto == "" {
			proto = "http"
			if c.Request.TLS != nil {
				proto = "https"
			}
		}
		originalDirector(req)
		req.URL.Path = "/" + strings.TrimLeft(targetPath, "/")
		req.URL.RawPath = ""
		req.Host = target.Host
		req.Header.Del("api-token")
		req.Header.Del("Authorization")
		req.Header.Del("Cookie")
		req.Header.Del("X-RDGEN-Token")
		if internalToken := strings.TrimSpace(os.Getenv("RDGEN_INTERNAL_TOKEN")); internalToken != "" {
			req.Header.Set("X-RDGEN-Token", internalToken)
		}
		req.Header.Set("X-Forwarded-Host", c.Request.Host)
		req.Header.Set("X-Forwarded-Proto", proto)
	}
	proxy.ErrorHandler = func(_ http.ResponseWriter, _ *http.Request, _ error) {
		c.JSON(http.StatusBadGateway, gin.H{"error": "Client generator is unavailable"})
	}
	proxy.ServeHTTP(c.Writer, c.Request)
}

func (ct *Rdgen) AdminProxy(c *gin.Context) {
	path := strings.TrimLeft(c.Param("path"), "/")
	if c.Request.Method == http.MethodGet && path == "defaults" {
		serveRdgenClientDefaults(c)
		return
	}
	switch {
	case c.Request.Method == http.MethodPost && path == "generator":
	case c.Request.Method == http.MethodGet && path == "check_for_file":
	case c.Request.Method == http.MethodGet && path == "artifacts":
	case c.Request.Method == http.MethodGet && path == "download":
	case c.Request.Method == http.MethodDelete && path == "delete_artifact_build":
	default:
		c.JSON(http.StatusNotFound, gin.H{"error": "Unknown generator endpoint"})
		return
	}
	proxyRdgen(c, path)
}
