package server

import (
	"errors"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

type frontendHandler struct {
	distDir string
	fsys    fs.FS
}

func newFrontendHandler(distDir string) http.Handler {
	cleanDistDir := strings.TrimSpace(distDir)
	return &frontendHandler{
		distDir: cleanDistDir,
		fsys:    os.DirFS(cleanDistDir),
	}
}

func (h *frontendHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}

	name := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
	if name == "." || name == "" {
		name = "index.html"
	}

	if h.serveIfExists(w, r, name) {
		return
	}
	if h.serveIfExists(w, r, "index.html") {
		return
	}

	http.Error(w, "frontend assets not found; run `make build` or install the service bundle", http.StatusServiceUnavailable)
}

func (h *frontendHandler) serveIfExists(w http.ResponseWriter, r *http.Request, name string) bool {
	file, err := h.fsys.Open(name)
	if err != nil {
		return false
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil || info.IsDir() {
		return false
	}

	http.ServeFileFS(w, r, h.fsys, name)
	return true
}

func frontendReady(distDir string) bool {
	if strings.TrimSpace(distDir) == "" {
		return false
	}
	_, err := fs.Stat(os.DirFS(distDir), "index.html")
	return err == nil || !errors.Is(err, fs.ErrNotExist)
}
