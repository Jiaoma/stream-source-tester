package rtsp

import (
	"fmt"
	"strings"
)

// parseParameterBody parses an RTSP SET_PARAMETER/GET_PARAMETER body.
// SET_PARAMETER bodies use "name: value" lines.
// GET_PARAMETER bodies use bare "name" lines (one per line).
func parseParameterLines(body string) []parameterLine {
	lines := strings.Split(strings.ReplaceAll(body, "\r\n", "\n"), "\n")
	result := make([]parameterLine, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if key, value, ok := strings.Cut(trimmed, ":"); ok {
			result = append(result, parameterLine{
				name:     strings.TrimSpace(key),
				value:    strings.TrimSpace(value),
				hasValue: true,
			})
			continue
		}
		result = append(result, parameterLine{name: trimmed})
	}
	return result
}

type parameterLine struct {
	name     string
	value    string
	hasValue bool
}

// applySetParameter stores key/value parameters into the session state.
// It returns the count of stored parameters and the first unsupported name (if any).
func applySetParameter(state *sessionState, body string) (stored int, unsupported string) {
	if strings.TrimSpace(body) == "" {
		// empty body acts as keepalive
		return 0, ""
	}
	if state.parameters == nil {
		state.parameters = map[string]string{}
	}
	for _, line := range parseParameterLines(body) {
		if !line.hasValue {
			// SET_PARAMETER without a value is not a valid assignment.
			return stored, line.name
		}
		if !isSupportedParameter(line.name) {
			return stored, line.name
		}
		state.parameters[line.name] = line.value
		stored++
	}
	return stored, ""
}

// buildGetParameterBody returns the response body for GET_PARAMETER queries.
func buildGetParameterBody(state *sessionState, body string) string {
	if strings.TrimSpace(body) == "" {
		return ""
	}
	var builder strings.Builder
	for _, line := range parseParameterLines(body) {
		value := ""
		if state.parameters != nil {
			value = state.parameters[line.name]
		}
		builder.WriteString(fmt.Sprintf("%s: %s\r\n", line.name, value))
	}
	return builder.String()
}

// isSupportedParameter limits SET_PARAMETER to a known, camera-style allowlist.
func isSupportedParameter(name string) bool {
	switch strings.ToLower(name) {
	case "framerate", "bitrate", "resolution", "gop", "brightness", "contrast":
		return true
	default:
		return false
	}
}
