package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRdgenAdminProxyAllowsKnownRoute(t *testing.T) {
	var receivedPath string
	var receivedToken string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.RequestURI()
		receivedToken = r.Header.Get("api-token")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer backend.Close()
	t.Setenv("RDGEN_INTERNAL_URL", backend.URL)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Any("/api/admin/rdgen/*path", (&Rdgen{}).AdminProxy)
	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()

	request, err := http.NewRequest(
		http.MethodGet,
		proxyServer.URL+"/api/admin/rdgen/artifacts?format=json",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("api-token", "backend-session")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusAccepted {
		t.Fatalf("expected status %d, got %d", http.StatusAccepted, response.StatusCode)
	}
	if receivedPath != "/artifacts?format=json" {
		t.Fatalf("unexpected proxied URL %q", receivedPath)
	}
	if receivedToken != "" {
		t.Fatal("api-token must not be forwarded to rdgen")
	}
}

func TestRdgenAdminProxyRejectsUnknownRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Any("/api/admin/rdgen/*path", (&Rdgen{}).AdminProxy)
	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()

	request, err := http.NewRequest(
		http.MethodGet,
		proxyServer.URL+"/api/admin/rdgen/get_zip",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, response.StatusCode)
	}
}

func TestRdgenPublicProxyAllowsWorkflowCallbackOnly(t *testing.T) {
	var receivedPath string
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.WriteHeader(http.StatusCreated)
	}))
	defer backend.Close()
	t.Setenv("RDGEN_INTERNAL_URL", backend.URL)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Any("/rdgen/*path", (&Rdgen{}).PublicProxy)
	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()

	allowed, err := http.Post(
		proxyServer.URL+"/rdgen/save_custom_client",
		"application/octet-stream",
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer allowed.Body.Close()
	if allowed.StatusCode != http.StatusCreated || receivedPath != "/save_custom_client" {
		t.Fatalf("allowed callback was not proxied: status=%d path=%q", allowed.StatusCode, receivedPath)
	}

	rejected, err := http.Get(proxyServer.URL + "/rdgen/artifacts")
	if err != nil {
		t.Fatal(err)
	}
	defer rejected.Body.Close()
	if rejected.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, rejected.StatusCode)
	}
}

func TestRdgenClientDefaultsStayLocalAndStripSecrets(t *testing.T) {
	defaultsFile := filepath.Join(t.TempDir(), "rdgen-client-defaults.json")
	err := os.WriteFile(
		defaultsFile,
		[]byte(`{
			"serverIP": "rustdesk.internal.example",
			"appname": "ExampleDesk",
			"permanentPassword": "must-not-leave-server",
			"unlockPin": "must-not-leave-server",
			"csrfmiddlewaretoken": "not-a-client-setting"
		}`),
		0o600,
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("RDGEN_CLIENT_DEFAULTS_FILE", defaultsFile)

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Any("/api/admin/rdgen/*path", (&Rdgen{}).AdminProxy)
	proxyServer := httptest.NewServer(router)
	defer proxyServer.Close()

	response, err := http.Get(proxyServer.URL + "/api/admin/rdgen/defaults")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, response.StatusCode)
	}
	body := response.Body
	var payload struct {
		Defaults map[string]any `json:"defaults"`
	}
	if err := json.NewDecoder(body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Defaults["serverIP"] != "rustdesk.internal.example" {
		t.Fatalf("unexpected defaults: %#v", payload.Defaults)
	}
	for _, field := range []string{
		"permanentPassword",
		"unlockPin",
		"csrfmiddlewaretoken",
	} {
		if _, found := payload.Defaults[field]; found {
			t.Fatalf("sensitive field %q was returned", field)
		}
	}
}
