package core

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSafeStaticFilePathAllowsChildPath(t *testing.T) {
	root := t.TempDir()
	got, err := safeStaticFilePath(root, "assets/app.js")
	if err != nil {
		t.Fatalf("safeStaticFilePath returned error: %v", err)
	}
	want := filepath.Join(root, "assets", "app.js")
	if filepath.Clean(got) != filepath.Clean(want) {
		t.Fatalf("safeStaticFilePath = %q, want %q", got, want)
	}
}

func TestSafeStaticFilePathRejectsTraversal(t *testing.T) {
	root := t.TempDir()
	cases := []string{
		"../secret.txt",
		"..\\secret.txt",
		"assets/../../secret.txt",
		"/etc/passwd",
		"C:\\Windows\\win.ini",
		"assets/\x00/app.js",
	}
	for _, item := range cases {
		if got, err := safeStaticFilePath(root, item); err == nil {
			t.Fatalf("safeStaticFilePath(%q) = %q, want error", item, got)
		}
	}
}

func TestFindFileRejectsTraversal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "public.txt"), []byte("public"), 0644); err != nil {
		t.Fatal(err)
	}
	uuid := strings.ReplaceAll(t.Name(), "/", "-")
	addStatic(uuid, root)
	defer remStatic(uuid)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "filename", Value: "..\\secret.txt"}}
	FindFile(c)

	if w.Code != http.StatusNotFound {
		t.Fatalf("FindFile traversal status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestFindFileServesChildFile(t *testing.T) {
	gin.SetMode(gin.TestMode)
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("console.log('ok');"), 0644); err != nil {
		t.Fatal(err)
	}
	uuid := strings.ReplaceAll(t.Name(), "/", "-")
	addStatic(uuid, root)
	defer remStatic(uuid)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/api/file/assets/app.js", nil)
	c.Params = gin.Params{{Key: "filename", Value: "assets/app.js"}}
	FindFile(c)

	if w.Code != http.StatusOK {
		t.Fatalf("FindFile child file status = %d, want %d", w.Code, http.StatusOK)
	}
	if body := w.Body.String(); body != "console.log('ok');" {
		t.Fatalf("FindFile body = %q", body)
	}
}
