package project

import "strings"

// VisionImageSectionMarker 图片analyze section title（与 AppendVisionImageAnalysisIfReady 注入一致）。
const VisionImageSectionMarker = "## 图片analyze"

// VisionImageAnalysisSection 单/多代理共用的图片analyze提示（analyze_image；上下文仅保留文字summary）。
func VisionImageAnalysisSection() string {
	var b strings.Builder
	b.WriteString(VisionImageSectionMarker)
	b.WriteString("\n\n")
	b.WriteString("- 遇到图片file（截图、validate码、登录页、report配图）时，若存在tool analyze_image，请传入服务器上的file path进行analyze。\n")
	b.WriteString("- 不要对二进制图片使用 read_file 指望理解内容；user message中「📎 xxx.png: /path」即为可传给 analyze_image 的path。\n")
	b.WriteString("- validate码类：若已从页面或接口save为本地图片（如 captcha.png），用 analyze_image，question 写明「只输出validate码字符」；识别failed则refreshvalidate码后重新save再识；复杂滑块/行为validate码勿指望单次识图successful。\n")
	b.WriteString("- 委派sub-agent时，若子task含validate码/截图识读，在 task description 中写明图片path与期望输出format。\n")
	return b.String()
}

// AppendVisionImageAnalysisIfReady 仅在 vision.enabled 且 model 已config时追加图片analyze提示。
func AppendVisionImageAnalysisIfReady(base string, visionReady bool) string {
	if !visionReady {
		return base
	}
	return AppendSystemPromptBlock(base, VisionImageAnalysisSection())
}
