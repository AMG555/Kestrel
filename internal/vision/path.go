package vision

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var allowedImageExt = map[string]struct{}{
	".png": {}, ".jpg": {}, ".jpeg": {}, ".webp": {}, ".gif": {},
	".bmp": {}, ".tif": {}, ".tiff": {},
}

// ResolveImagePath resolves and validates an image file path safely.
func ResolveImagePath(path string, cwd string) (string, error) {
	p := strings.TrimSpace(path)
	if p == "" {
		return "", fmt.Errorf("path is empty")
	}

	clean := filepath.Clean(p)
	if !filepath.IsAbs(clean) && cwd != "" {
		clean = filepath.Join(cwd, clean)
	}

	ext := strings.ToLower(filepath.Ext(clean))
	if _, ok := allowedImageExt[ext]; !ok {
		return "", fmt.Errorf("unsupported image format %q; allowed: png, jpg, jpeg, webp, gif, bmp, tiff", ext)
	}

	fi, err := os.Stat(clean)
	if err != nil {
		return "", fmt.Errorf("cannot access image file: %w", err)
	}
	if fi.IsDir() {
		return "", fmt.Errorf("path is a directory: %s", clean)
	}

	return clean, nil
}
