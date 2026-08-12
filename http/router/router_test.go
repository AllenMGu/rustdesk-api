package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
)

func TestLegacyWebClient2RedirectsToBundledWebClient(t *testing.T) {
	previousConfig := global.Config
	t.Cleanup(func() {
		global.Config = previousConfig
	})

	resourcesPath := t.TempDir()
	webPath := filepath.Join(resourcesPath, "web")
	if err := os.Mkdir(webPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webPath, "index.html"), []byte("web client"), 0o644); err != nil {
		t.Fatal(err)
	}

	global.Config.App.WebClient = 1
	global.Config.Gin.ResourcesPath = resourcesPath

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	WebInit(engine)

	tests := []string{
		"/webclient2/",
		"/webclient2/index.js?source=legacy",
	}

	for _, path := range tests {
		t.Run(path, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, path, nil)
			response := httptest.NewRecorder()
			engine.ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("expected status %d, got %d", http.StatusOK, response.Code)
			}
			body := response.Body.String()
			for _, expected := range []string{
				"window.location.replace('/webclient/' + targetFragment)",
				"'#/?id=' + encodeURIComponent(id)",
			} {
				if !strings.Contains(body, expected) {
					t.Fatalf("legacy redirect page does not contain %q", expected)
				}
			}
		})
	}
}
