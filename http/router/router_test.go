package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/lejianwen/rustdesk-api/v2/global"
)

func TestOnlyBundledWebClientRouteIsRegistered(t *testing.T) {
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
	sourcePath := filepath.Join(resourcesPath, "..", "webclient-v1")
	if err := os.Mkdir(sourcePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourcePath, "README.md"), []byte("complete source"), 0o644); err != nil {
		t.Fatal(err)
	}

	global.Config.App.WebClient = 1
	global.Config.Gin.ResourcesPath = resourcesPath

	gin.SetMode(gin.TestMode)
	engine := gin.New()
	WebInit(engine)

	request := httptest.NewRequest(http.MethodGet, "/webclient/", nil)
	response := httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected bundled client status %d, got %d", http.StatusOK, response.Code)
	}
	if response.Body.String() != "web client" {
		t.Fatalf("unexpected bundled client response %q", response.Body.String())
	}

	request = httptest.NewRequest(http.MethodGet, "/webclient-source/README.md", nil)
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("expected corresponding source status %d, got %d", http.StatusOK, response.Code)
	}
	if response.Body.String() != "complete source" {
		t.Fatalf("unexpected corresponding source response %q", response.Body.String())
	}

	removedPath := "/webclient" + strconv.Itoa(2) + "/"
	request = httptest.NewRequest(http.MethodGet, removedPath, nil)
	response = httptest.NewRecorder()
	engine.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("expected removed route status %d, got %d", http.StatusNotFound, response.Code)
	}
}
