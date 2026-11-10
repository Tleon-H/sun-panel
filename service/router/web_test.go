package router

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	os.Exit(m.Run())
}

func TestFrontendEntriesStayOnTheRealHashRoutes(t *testing.T) {
	webPath, uploadPath := siteFixture(t)
	engine := NewEngine(webPath, uploadPath)
	shell := "LEON_PANEL_SHELL"

	root := get(engine, "/")
	if root.Code != http.StatusOK || !strings.Contains(root.Body.String(), shell) {
		t.Fatalf("GET / = %d %q", root.Code, root.Body.String())
	}

	for _, route := range hashFrontendRoutes {
		t.Run(route, func(t *testing.T) {
			response := get(engine, route)
			if response.Code != http.StatusFound {
				t.Fatalf("status = %d", response.Code)
			}
			if response.Header().Get("Location") != "/#"+route {
				t.Fatalf("location = %q", response.Header().Get("Location"))
			}
			if strings.Contains(response.Body.String(), shell) {
				t.Fatal("frontend entry returned the SPA document instead of the hash route")
			}
		})
	}

	withQuery := get(engine, "/login?next=%2Fhome")
	if withQuery.Header().Get("Location") != "/#/login?next=%2Fhome" {
		t.Fatalf("location = %q", withQuery.Header().Get("Location"))
	}

	head := request(engine, http.MethodHead, "/login")
	if head.Code != http.StatusFound || head.Header().Get("Location") != "/#/login" {
		t.Fatalf("HEAD /login = %d %q", head.Code, head.Header().Get("Location"))
	}
	if strings.Contains(head.Body.String(), shell) {
		t.Fatal("HEAD returned the SPA document")
	}

	broken := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	req.URL.RawQuery = "next=bad\r\nSet-Cookie:x"
	engine.ServeHTTP(broken, req)
	if broken.Code != http.StatusBadRequest || strings.Contains(broken.Body.String(), shell) {
		t.Fatalf("newline query = %d %q", broken.Code, broken.Body.String())
	}
}

func TestStaticUploadsAndUnknownPathsAreNotReplacedByThePage(t *testing.T) {
	webPath, uploadPath := siteFixture(t)
	engine := NewEngine(webPath, uploadPath)
	shell := "LEON_PANEL_SHELL"

	asset := get(engine, "/assets/app.js")
	if asset.Code != http.StatusOK || asset.Body.String() != "asset-js" {
		t.Fatalf("asset = %d %q", asset.Code, asset.Body.String())
	}
	custom := get(engine, "/custom/theme.css")
	if custom.Code != http.StatusOK || custom.Body.String() != "theme-css" {
		t.Fatalf("custom = %d %q", custom.Code, custom.Body.String())
	}
	icon := get(engine, "/favicon.ico")
	if icon.Code != http.StatusOK || icon.Body.String() != "icon" {
		t.Fatalf("favicon = %d %q", icon.Code, icon.Body.String())
	}
	upload := get(engine, "/uploads/pic.png")
	if upload.Code != http.StatusOK || upload.Body.String() != "png-bytes" {
		t.Fatalf("upload = %d %q", upload.Code, upload.Body.String())
	}

	missing := []string{
		"/assets/missing.js",
		"/custom/missing.css",
		"/uploads/missing.png",
		"/uploads/../index.html",
		"/does-not-exist",
		"/login/extra",
		"/Login",
		"/index.html",
		"/logo.png",
		"/api/does-not-exist",
		"/api/login",
	}
	for _, path := range missing {
		t.Run("GET "+path, func(t *testing.T) {
			response := get(engine, path)
			if response.Code != http.StatusNotFound {
				t.Fatalf("status = %d body = %q", response.Code, response.Body.String())
			}
			assertNoFileBody(t, response, shell)
		})
	}

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch, http.MethodOptions} {
		t.Run(method+" /login", func(t *testing.T) {
			response := request(engine, method, "/login")
			if response.Code != http.StatusMethodNotAllowed {
				t.Fatalf("status = %d", response.Code)
			}
			if response.Header().Get("Allow") != "GET, HEAD" {
				t.Fatalf("allow = %q", response.Header().Get("Allow"))
			}
			assertNoFileBody(t, response, shell)
		})
	}

	for _, item := range []struct {
		method string
		path   string
	}{
		{http.MethodPost, "/api/does-not-exist"},
		{http.MethodPut, "/api/login"},
		{http.MethodPost, "/"},
		{http.MethodPost, "/assets/app.js"},
		{http.MethodDelete, "/uploads/pic.png"},
		{http.MethodPost, "/does-not-exist"},
	} {
		t.Run(item.method+" "+item.path, func(t *testing.T) {
			response := request(engine, item.method, item.path)
			if response.Code == http.StatusOK || response.Code == http.StatusFound {
				t.Fatalf("status = %d body = %q", response.Code, response.Body.String())
			}
			assertNoFileBody(t, response, shell)
		})
	}
}

func TestListenAddressStaysCompatible(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "main.go"))
	if err != nil {
		t.Fatal(err)
	}
	source := string(raw)
	if strings.Contains(source, "127.0.0.1") {
		t.Fatal("loopback bind is a local preview measure and must not be part of the server")
	}
	if !strings.Contains(source, `router.InitRouters(":" + httpPort)`) {
		t.Fatalf("listen address changed: %s", source)
	}
}

func assertNoFileBody(t *testing.T, response *httptest.ResponseRecorder, shell string) {
	t.Helper()
	text := response.Body.String()
	if strings.Contains(text, shell) || strings.Contains(text, "asset-js") || strings.Contains(text, "png-bytes") || strings.Contains(text, "outside-secret") || strings.Contains(text, "logo") {
		t.Fatalf("response returned file contents: %q", text)
	}
}

func siteFixture(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	webPath := filepath.Join(root, "web")
	uploadPath := filepath.Join(root, "uploads")
	writeFile(t, filepath.Join(webPath, "index.html"), "LEON_PANEL_SHELL")
	writeFile(t, filepath.Join(webPath, "assets", "app.js"), "asset-js")
	writeFile(t, filepath.Join(webPath, "custom", "theme.css"), "theme-css")
	writeFile(t, filepath.Join(webPath, "favicon.ico"), "icon")
	writeFile(t, filepath.Join(webPath, "favicon.svg"), "svg")
	writeFile(t, filepath.Join(webPath, "logo.png"), "logo-bytes")
	writeFile(t, filepath.Join(uploadPath, "pic.png"), "png-bytes")
	writeFile(t, filepath.Join(root, "outside.txt"), "outside-secret")

	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(wd); err != nil {
			t.Error(err)
		}
	})
	return webPath, "./uploads"
}

func writeFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
}

func get(engine *gin.Engine, path string) *httptest.ResponseRecorder {
	return request(engine, http.MethodGet, path)
}

func request(engine *gin.Engine, method, path string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	engine.ServeHTTP(recorder, req)
	return recorder
}
