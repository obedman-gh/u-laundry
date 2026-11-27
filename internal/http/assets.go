package http

import (
	"io/fs"
	"net/http"
	"path"
	"strings"

	"ulaundry/internal/http/response"
)

// Only reviewed public assets can be served, including if STATIC_DIR is
// accidentally pointed at the repository. No directory listing or backup files.
var publicAssetNames = func() map[string]bool {
	names := strings.Fields(`index.html login.html tracking.html customers.html services.html
		staff.html pos.html dashboard.html shifts.html audit.html offline.html
		login.js tracking.js customers.js services.js staff.js pos.js pos-model.js
		queue.js queue-model.js dashboard.js reports-model.js shifts.js audit.js receipt.js
		customers.css staff.css pos.css queue.css reports.css shifts.css audit.css receipt.css
		ui/ui.js ui/session.js ui/ui.css ui/tokens.css ui/favicon.svg
		pwa/pwa.js pwa/pwa.css manifest.webmanifest sw.js icons/icon-192.png icons/icon-512.png`)
	result := make(map[string]bool, len(names))
	for _, name := range names {
		result["/"+name] = true
	}
	return result
}()

type publicAssets struct{ source http.FileSystem }

var publicMediaTypes = map[string]string{
	".html":        "text/html; charset=utf-8",
	".js":          "text/javascript; charset=utf-8",
	".css":         "text/css; charset=utf-8",
	".webmanifest": "application/manifest+json",
	".svg":         "image/svg+xml",
	".png":         "image/png",
}

// ServeContent clears cache headers on range/precondition errors. Keep public
// asset errors under the same no-store policy as the other router responses.
type assetWriter struct{ http.ResponseWriter }

func (w assetWriter) WriteHeader(status int) {
	w.Header().Set("Cache-Control", "no-store")
	w.ResponseWriter.WriteHeader(status)
}

func (a publicAssets) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path == "/index.html" {
		w.Header().Set("Location", "./")
		w.WriteHeader(http.StatusMovedPermanently)
		return
	}
	f, err := a.Open(r.URL.Path)
	if err != nil {
		response.Fail(w, r, 404, "not_found", "Aset tidak ditemukan.")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		response.Fail(w, r, 500, "internal_error", "Aset tidak dapat dibaca.")
		return
	}
	// Do not rely on the host registry/sniffing for executable assets or the
	// manifest: nosniff must not break the same release on Windows and Ubuntu.
	w.Header().Set("Content-Type", publicMediaTypes[path.Ext(info.Name())])
	http.ServeContent(assetWriter{w}, r, info.Name(), info.ModTime(), f)
}

func (a publicAssets) Open(name string) (http.File, error) {
	if name == "/" {
		name = "/index.html"
	}
	if !publicAssetNames[name] {
		return nil, fs.ErrNotExist
	}
	f, err := a.source.Open(name)
	if err != nil {
		return nil, fs.ErrNotExist
	}
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		closeErr := f.Close()
		if closeErr != nil {
			return nil, closeErr
		}
		return nil, fs.ErrNotExist
	}
	return f, nil
}
