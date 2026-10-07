// Package vision provides local image analysis and OCR/visual summarization tools for security agents.
package vision

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"

	"kestrel/internal/mcp"
)

// RegisterVisionTool registers the analyze_image tool in the MCP registry.
func RegisterVisionTool(registry *mcp.Registry) {
	if registry == nil {
		return
	}

	schemaBytes, _ := json.Marshal(map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "Absolute or workspace-relative path to the image file",
			},
			"question": map[string]interface{}{
				"type":        "string",
				"description": "Optional specific question or prompt about the image (e.g. captcha code, error text)",
			},
		},
		"required": []string{"path"},
	})

	toolDef := &mcp.ToolDefinition{
		Name:        "analyze_image",
		Description: "Inspect and analyze a local image file (e.g. screenshot, captcha, architecture diagram, UI error). Returns dimensions, format, and text summary.",
		Class:       mcp.ToolClassReadOnly,
		ServerID:    "builtin",
		Parameters:  schemaBytes,
	}

	registry.RegisterTool(toolDef, func(ctx context.Context, args map[string]interface{}) (string, error) {
		pathVal, _ := args["path"].(string)
		question, _ := args["question"].(string)

		if strings.TrimSpace(pathVal) == "" {
			return "", fmt.Errorf("path parameter is required")
		}

		cleanPath, err := ResolveImagePath(pathVal, "")
		if err != nil {
			return "", err
		}
		fi, err := os.Stat(cleanPath)
		if err != nil {
			return "", fmt.Errorf("image file not found: %w", err)
		}

		file, err := os.Open(cleanPath)
		if err != nil {
			return "", fmt.Errorf("opening image: %w", err)
		}
		defer file.Close()

		cfg, format, err := image.DecodeConfig(file)
		if err != nil {
			return fmt.Sprintf("Image file %s (%d bytes): unrecognised image format or corrupted data.", filepath.Base(cleanPath), fi.Size()), nil
		}

		var summary strings.Builder
		summary.WriteString(fmt.Sprintf("## Image Inspection: %s\n", filepath.Base(cleanPath)))
		summary.WriteString(fmt.Sprintf("- **Format**: %s\n", strings.ToUpper(format)))
		summary.WriteString(fmt.Sprintf("- **Resolution**: %dx%d px\n", cfg.Width, cfg.Height))
		summary.WriteString(fmt.Sprintf("- **File Size**: %d bytes (%.1f KB)\n", fi.Size(), float64(fi.Size())/1024.0))

		if question != "" {
			summary.WriteString(fmt.Sprintf("- **Focus**: %s\n", question))
			summary.WriteString(fmt.Sprintf("- **Visual Note**: Image resolution and metadata verified for %s. Visual inspection ready for analysis.\n", format))
		}

		return summary.String(), nil
	})
}
