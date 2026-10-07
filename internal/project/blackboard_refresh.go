package project

import "strings"

// FactIndexSectionHeading is the human-readable heading prefix for the blackboard index block (retained in the block for Agent reading).
const FactIndexSectionHeading = "## Project Blackboard Index"

// FactIndexSectionStartMarker / EndMarker: HTML comment boundaries used for programmatic replacement; carry no instruction semantics for the model.
const (
	FactIndexSectionStartMarker = "<!-- fact-index-start -->"
	FactIndexSectionEndMarker   = "<!-- fact-index-end -->"
)

// ReplaceFactIndexSection replaces the existing project blackboard index section in content with freshIndex.
// freshIndex must be the complete output of BuildFactIndexBlock. Returns (_, false) when the HTML comment markers are missing.
func ReplaceFactIndexSection(content, freshIndex string) (string, bool) {
	freshIndex = strings.TrimSpace(freshIndex)
	if freshIndex == "" {
		return content, false
	}
	start, ok := factIndexSectionStart(content)
	if !ok {
		return content, false
	}
	end, ok := factIndexSectionEnd(content, start)
	if !ok || end <= start {
		return content, false
	}
	return content[:start] + freshIndex + content[end:], true
}

// wrapFactIndexBlock wraps the BuildFactIndexBlock body with consistent start/end HTML comment markers.
func wrapFactIndexBlock(content string) string {
	content = strings.TrimSpace(content)
	return FactIndexSectionStartMarker + "\n" + content + "\n" + FactIndexSectionEndMarker + "\n"
}

func factIndexSectionStart(content string) (int, bool) {
	idx := strings.Index(content, FactIndexSectionStartMarker)
	if idx < 0 {
		return 0, false
	}
	return idx, true
}

func factIndexSectionEnd(content string, start int) (int, bool) {
	if start < 0 || start >= len(content) {
		return 0, false
	}
	tail := content[start:]
	idx := strings.LastIndex(tail, FactIndexSectionEndMarker)
	if idx < 0 {
		return 0, false
	}
	return start + idx + len(FactIndexSectionEndMarker), true
}
