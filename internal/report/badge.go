package report

import (
	"fmt"
)

// GenerateBadgeSVG generates a clean shields.io style SVG status badge.
func GenerateBadgeSVG(critical, high, medium, low int) []byte {
	label := "kestrel"
	var status string
	var statusColor string

	if critical > 0 {
		status = fmt.Sprintf("%d critical", critical)
		statusColor = "#e11d48" // red
	} else if high > 0 {
		status = fmt.Sprintf("%d high", high)
		statusColor = "#f97316" // orange
	} else if medium > 0 {
		status = fmt.Sprintf("%d medium", medium)
		statusColor = "#eab308" // yellow
	} else if low > 0 {
		status = fmt.Sprintf("%d low", low)
		statusColor = "#3b82f6" // blue
	} else {
		status = "0 vulnerabilities"
		statusColor = "#10b981" // emerald green
	}

	labelWidth := len(label)*7 + 16
	statusWidth := len(status)*7 + 16
	totalWidth := labelWidth + statusWidth

	labelCenter := float64(labelWidth) / 2.0
	statusCenter := float64(labelWidth) + (float64(statusWidth) / 2.0)

	svg := fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="20" role="img" aria-label="%s: %s">
  <linearGradient id="s" x2="0" y2="100%%">
    <stop offset="0" stop-color="#bbb" stop-opacity=".1"/>
    <stop offset="1" stop-opacity=".1"/>
  </linearGradient>
  <clipPath id="r">
    <rect width="%d" height="20" rx="3" fill="#fff"/>
  </clipPath>
  <g clip-path="url(#r)">
    <rect width="%d" height="20" fill="#24292e"/>
    <rect x="%d" width="%d" height="20" fill="%s"/>
    <rect width="%d" height="20" fill="url(#s)"/>
  </g>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" text-rendering="geometricPrecision" font-size="11">
    <text aria-hidden="true" x="%.1f" y="15" fill="#010101" fill-opacity=".3">%s</text>
    <text x="%.1f" y="14">%s</text>
    <text aria-hidden="true" x="%.1f" y="15" fill="#010101" fill-opacity=".3">%s</text>
    <text x="%.1f" y="14">%s</text>
  </g>
</svg>`,
		totalWidth, label, status,
		totalWidth,
		labelWidth,
		labelWidth, statusWidth, statusColor,
		totalWidth,
		labelCenter, label,
		labelCenter, label,
		statusCenter, status,
		statusCenter, status,
	)

	return []byte(svg)
}
