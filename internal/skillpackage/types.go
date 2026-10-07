// Package skillpackage provides discovery, validation, and rendering for Agent Skills (SKILL.md).
package skillpackage

// SkillManifest is parsed from SKILL.md YAML front matter.
type SkillManifest struct {
	Name          string                 `yaml:"name" json:"name"`
	Description   string                 `yaml:"description" json:"description"`
	License       string                 `yaml:"license,omitempty" json:"license,omitempty"`
	Compatibility string                 `yaml:"compatibility,omitempty" json:"compatibility,omitempty"`
	AllowedTools  string                 `yaml:"allowed-tools,omitempty" json:"allowed_tools,omitempty"`
	Metadata      map[string]interface{} `yaml:"metadata,omitempty" json:"metadata,omitempty"`
}

// SkillSummary provides index metadata for a skill directory.
type SkillSummary struct {
	ID          string   `json:"id"`
	DirName     string   `json:"dir_name"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Path        string   `json:"path"`
	Tags        []string `json:"tags"`
	FileCount   int      `json:"file_count"`
	ModTime     string   `json:"mod_time"`
}

// SkillSection represents an H2 heading section in SKILL.md.
type SkillSection struct {
	ID      string `json:"id"`
	Title   string `json:"title"`
	Heading string `json:"heading"`
	Level   int    `json:"level"`
}

// PackageFileInfo describes a file inside a skill package directory.
type PackageFileInfo struct {
	Path  string `json:"path"`
	Size  int64  `json:"size"`
	IsDir bool   `json:"is_dir,omitempty"`
}

// SkillView represents a loaded skill package with full content and metadata.
type SkillView struct {
	ID           string            `json:"id"`
	DirName      string            `json:"dir_name"`
	Name         string            `json:"name"`
	Description  string            `json:"description"`
	Content      string            `json:"content"`
	AllowedTools string            `json:"allowed_tools,omitempty"`
	Path         string            `json:"path"`
	Tags         []string          `json:"tags"`
	Sections     []SkillSection    `json:"sections,omitempty"`
	PackageFiles []PackageFileInfo `json:"package_files,omitempty"`
}
