package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"kestrel/internal/skillpackage"
)

// SkillsHandler exposes Agent Skills via REST.
type SkillsHandler struct {
	skillsDir string
}

// NewSkillsHandler creates a new skills handler pointing to the skills root directory.
func NewSkillsHandler(skillsDir string) *SkillsHandler {
	return &SkillsHandler{skillsDir: skillsDir}
}

// ListSkills returns all discovered skills.
// GET /api/skills
func (h *SkillsHandler) ListSkills(c *gin.Context) {
	skills, err := skillpackage.ListSkills(h.skillsDir)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"skills": skills,
		"total":  len(skills),
	})
}

// GetSkill returns details for a single skill.
// GET /api/skills/:id
func (h *SkillsHandler) GetSkill(c *gin.Context) {
	id := c.Param("id")
	skill, err := skillpackage.GetSkill(h.skillsDir, id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, skill)
}
