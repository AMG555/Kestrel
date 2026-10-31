package web

import (
	"embed"
	"io/fs"
	"net/http"
)

//go:embed all:dist
var DistFS embed.FS

// ServerFS returns an http.FileSystem and fs.FS rooted at the dist directory.
func ServerFS() (http.FileSystem, fs.FS, error) {
	sub, err := fs.Sub(DistFS, "dist")
	if err != nil {
		return nil, nil, err
	}
	return http.FS(sub), sub, nil
}
