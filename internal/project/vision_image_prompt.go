package project

import "strings"

// VisionImageSectionMarker is the image analysis section heading (consistent with what AppendVisionImageAnalysisIfReady injects).
const VisionImageSectionMarker = "## Image Analysis"

// VisionImageAnalysisSection returns the image analysis prompt shared by single/multi-agent setups (analyze_image; context retains text summary only).
func VisionImageAnalysisSection() string {
	var b strings.Builder
	b.WriteString(VisionImageSectionMarker)
	b.WriteString("\n\n")
	b.WriteString("- When you encounter image files (screenshots, CAPTCHAs, login pages, report figures), if the analyze_image tool is available, pass the server file path to it for analysis.\n")
	b.WriteString("- Do not use read_file on binary images expecting to understand the content; a \"📎 xxx.png: /path\" in a user message is the path to pass to analyze_image.\n")
	b.WriteString("- For CAPTCHAs: if the image has already been saved from the page or API as a local file (e.g. captcha.png), use analyze_image with the question set to \"output only the CAPTCHA characters\"; if recognition fails, refresh the CAPTCHA and save/recognise again; do not expect a single image recognition call to succeed for complex slider/behaviour CAPTCHAs.\n")
	b.WriteString("- When delegating to a sub-agent, if the sub-task involves CAPTCHA/screenshot recognition, include the image path and expected output format in the task description.\n")
	return b.String()
}

// AppendVisionImageAnalysisIfReady appends the image analysis prompt only when vision.enabled and the model is configured.
func AppendVisionImageAnalysisIfReady(base string, visionReady bool) string {
	if !visionReady {
		return base
	}
	return AppendSystemPromptBlock(base, VisionImageAnalysisSection())
}
