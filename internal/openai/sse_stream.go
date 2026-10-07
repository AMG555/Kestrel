package openai

// SSEAccumulatedKey is the authoritative full-text snapshot field in SSE progress event data.
// The frontend should prefer this field to update the buffer, avoiding duplicate characters from double-normalizing deltas.
const SSEAccumulatedKey = "accumulated"

// WithSSEAccumulated attaches the current streaming accumulated full text (authoritative snapshot) to the progress data.
func WithSSEAccumulated(data map[string]interface{}, accumulated string) map[string]interface{} {
	if data == nil {
		data = make(map[string]interface{}, 1)
	}
	data[SSEAccumulatedKey] = accumulated
	return data
}

// NormalizeStreamingDelta normalizes content that may be a "cumulative fragment/retransmitted fragment" into a "pure delta".
// Same as the unexported normalizeStreamingDelta; exported for use by agent/multiagent packages to accumulate body text before sending SSE.
func NormalizeStreamingDelta(current, incoming string) (next, delta string) {
	return normalizeStreamingDelta(current, incoming)
}
