package project

import "kestrel/internal/projectprompt"

// FactRecordingIncrementalRhythmMarkdown delegates to projectprompt.
func FactRecordingIncrementalRhythmMarkdown(coordinator, subAgent bool) string {
	return projectprompt.FactRecordingIncrementalRhythmMarkdown(coordinator, subAgent)
}

// FactRecordingBlackboardSection delegates to projectprompt.
func FactRecordingBlackboardSection(coordinatorDelegate bool) string {
	return projectprompt.FactRecordingBlackboardSection(coordinatorDelegate)
}

// FactRecordingSubAgentSection delegates to projectprompt.
func FactRecordingSubAgentSection() string {
	return projectprompt.FactRecordingSubAgentSection()
}

// FactRecordingBlackboardSectionMarkdown delegates to projectprompt.
func FactRecordingBlackboardSectionMarkdown(coordinatorDelegate bool) string {
	return projectprompt.FactRecordingBlackboardSectionMarkdown(coordinatorDelegate)
}
