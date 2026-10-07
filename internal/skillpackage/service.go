package skillpackage

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var headingRegex = regexp.MustCompile(`(?m)^(#+)\s+(.+)$`)

// ParseSkillMD splits a SKILL.md into its YAML frontmatter and markdown body.
func ParseSkillMD(raw []byte) (*SkillManifest, string, error) {
	rawStr := string(raw)
	if !strings.HasPrefix(strings.TrimSpace(rawStr), "---") {
		return &SkillManifest{}, rawStr, nil
	}

	trimmed := strings.TrimLeft(rawStr, "\r\n ")
	if !strings.HasPrefix(trimmed, "---") {
		return &SkillManifest{}, rawStr, nil
	}

	parts := strings.SplitN(trimmed[3:], "---", 2)
	if len(parts) < 2 {
		return &SkillManifest{}, rawStr, nil
	}

	frontMatterYAML := parts[0]
	body := strings.TrimLeft(parts[1], "\r\n")

	var manifest SkillManifest
	if err := yaml.Unmarshal([]byte(frontMatterYAML), &manifest); err != nil {
		return nil, body, fmt.Errorf("parsing frontmatter yaml: %w", err)
	}

	return &manifest, body, nil
}

// ListSkills scans the skills root directory and returns metadata summaries.
func ListSkills(skillsRoot string) ([]SkillSummary, error) {
	entries, err := os.ReadDir(skillsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return []SkillSummary{}, nil
		}
		return nil, err
	}

	var results []SkillSummary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirName := entry.Name()
		if strings.HasPrefix(dirName, ".") {
			continue
		}

		skillDir := filepath.Join(skillsRoot, dirName)
		skillMDPath := filepath.Join(skillDir, "SKILL.md")
		data, err := os.ReadFile(skillMDPath)
		if err != nil {
			// Check lowercase or alternative
			skillMDPath = filepath.Join(skillDir, "skill.md")
			data, err = os.ReadFile(skillMDPath)
			if err != nil {
				continue
			}
		}

		manifest, _, _ := ParseSkillMD(data)
		fi, _ := os.Stat(skillMDPath)
		modTime := ""
		if fi != nil {
			modTime = fi.ModTime().Format(time.RFC3339)
		}

		name := dirName
		desc := ""
		if manifest != nil {
			if manifest.Name != "" {
				name = manifest.Name
			}
			desc = manifest.Description
		}

		// Count package files
		files, _ := os.ReadDir(skillDir)
		fileCount := len(files)

		tags := extractTags(dirName, manifest)

		results = append(results, SkillSummary{
			ID:          dirName,
			DirName:     dirName,
			Name:        name,
			Description: desc,
			Path:        skillDir,
			Tags:        tags,
			FileCount:   fileCount,
			ModTime:     modTime,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].DirName < results[j].DirName
	})

	return results, nil
}

// GetSkill retrieves full details and package files for a specific skill.
func GetSkill(skillsRoot, id string) (*SkillView, error) {
	skillDir := filepath.Join(skillsRoot, id)
	skillMDPath := filepath.Join(skillDir, "SKILL.md")
	data, err := os.ReadFile(skillMDPath)
	if err != nil {
		skillMDPath = filepath.Join(skillDir, "skill.md")
		data, err = os.ReadFile(skillMDPath)
		if err != nil {
			return nil, fmt.Errorf("skill not found: %w", err)
		}
	}

	manifest, body, err := ParseSkillMD(data)
	if err != nil {
		return nil, err
	}

	sections := extractSections(body)
	pkgFiles := listPackageFiles(skillDir)

	name := id
	desc := ""
	allowedTools := ""
	if manifest != nil {
		if manifest.Name != "" {
			name = manifest.Name
		}
		desc = manifest.Description
		allowedTools = manifest.AllowedTools
	}

	return &SkillView{
		ID:           id,
		DirName:      id,
		Name:         name,
		Description:  desc,
		Content:      string(data),
		AllowedTools: allowedTools,
		Path:         skillDir,
		Tags:         extractTags(id, manifest),
		Sections:     sections,
		PackageFiles: pkgFiles,
	}, nil
}

func extractSections(body string) []SkillSection {
	matches := headingRegex.FindAllStringSubmatch(body, -1)
	var sections []SkillSection
	for _, m := range matches {
		level := len(m[1])
		title := strings.TrimSpace(m[2])
		id := strings.ToLower(strings.ReplaceAll(title, " ", "-"))
		sections = append(sections, SkillSection{
			ID:      id,
			Title:   title,
			Heading: m[0],
			Level:   level,
		})
	}
	return sections
}

func listPackageFiles(dir string) []PackageFileInfo {
	var files []PackageFileInfo
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil || rel == "." {
			return nil
		}
		files = append(files, PackageFileInfo{
			Path:  filepath.ToSlash(rel),
			Size:  info.Size(),
			IsDir: info.IsDir(),
		})
		return nil
	})
	return files
}

func extractTags(dirName string, manifest *SkillManifest) []string {
	tagsSet := make(map[string]bool)
	parts := strings.Split(dirName, "-")
	for _, p := range parts {
		if len(p) > 2 {
			tagsSet[p] = true
		}
	}
	if manifest != nil && manifest.Metadata != nil {
		if rawTags, ok := manifest.Metadata["tags"].([]interface{}); ok {
			for _, t := range rawTags {
				if s, ok := t.(string); ok {
					tagsSet[s] = true
				}
			}
		}
	}
	var tags []string
	for t := range tagsSet {
		tags = append(tags, t)
	}
	sort.Strings(tags)
	return tags
}
