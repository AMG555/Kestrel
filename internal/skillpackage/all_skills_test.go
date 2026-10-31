package skillpackage_test

import (
	"os"
	"path/filepath"
	"testing"

	"kestrel/internal/skillpackage"
)

func TestAllInstalledSkills(t *testing.T) {
	skillsRoot := filepath.Join("..", "..", "skills")
	dirEntries, err := os.ReadDir(skillsRoot)
	if err != nil {
		t.Fatalf("failed to read skills dir: %v", err)
	}

	skillCount := 0
	for _, entry := range dirEntries {
		if !entry.IsDir() {
			continue
		}
		skillDirName := entry.Name()
		skillMDPath := filepath.Join(skillsRoot, skillDirName, "SKILL.md")
		data, err := os.ReadFile(skillMDPath)
		if err != nil {
			t.Errorf("skill %q missing SKILL.md: %v", skillDirName, err)
			continue
		}

		skillCount++
		// Validate skill package
		if err := skillpackage.ValidateSkillMDPackage(data, skillDirName); err != nil {
			t.Errorf("skill %q failed validation: %v", skillDirName, err)
		}

		// Also test loading skill
		opt := skillpackage.LoadOptions{Depth: "full"}
		skill, err := skillpackage.LoadSkill(skillsRoot, skillDirName, opt)
		if err != nil {
			t.Errorf("skill %q failed LoadSkill: %v", skillDirName, err)
		} else {
			if skill.Name == "" {
				t.Errorf("skill %q loaded with empty name", skillDirName)
			}
		}
	}

	if skillCount == 0 {
		t.Errorf("no skills found in %q", skillsRoot)
	} else {
		t.Logf("Successfully validated %d skills in %q", skillCount, skillsRoot)
	}
}

func TestListSkillSummaries(t *testing.T) {
	skillsRoot := filepath.Join("..", "..", "skills")
	summaries, err := skillpackage.ListSkillSummaries(skillsRoot)
	if err != nil {
		t.Fatalf("ListSkillSummaries failed: %v", err)
	}
	if len(summaries) < 25 {
		t.Errorf("expected at least 25 skills, got %d", len(summaries))
	}
	for _, s := range summaries {
		if s.Name == "" || s.Description == "" {
			t.Errorf("skill %s missing name or description", s.DirName)
		}
	}
}
