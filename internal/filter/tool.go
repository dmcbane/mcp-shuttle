package filter

import (
	"encoding/json"
	"path"
)

// MatchesTool checks if a tool name matches a wildcard pattern.
// Uses path.Match which supports *, ?, and [] wildcards.
func MatchesTool(pattern, name string) bool {
	matched, _ := path.Match(pattern, name)
	return matched
}

// ShouldIgnore returns true if the tool name matches any of the ignore patterns.
func ShouldIgnore(patterns []string, toolName string) bool {
	for _, p := range patterns {
		if MatchesTool(p, toolName) {
			return true
		}
	}
	return false
}

// ShouldAllow returns true if the tool name matches any of the allow patterns.
// An empty allow list permits all tools (no filtering).
func ShouldAllow(patterns []string, toolName string) bool {
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if MatchesTool(p, toolName) {
			return true
		}
	}
	return false
}

// ValidatePattern checks if a glob pattern is syntactically valid for path.Match.
func ValidatePattern(pattern string) error {
	_, err := path.Match(pattern, "")
	return err
}

// toolEntry represents a single tool in a tools/list response.
// We use a raw approach to preserve all fields.
type toolEntry struct {
	Name string `json:"name"`
	rest json.RawMessage
}

// FilterToolsListResponse filters tools from a tools/list JSON-RPC result payload.
// It removes any tools whose name matches the given ignore patterns.
func FilterToolsListResponse(data []byte, patterns []string) ([]byte, error) {
	if len(patterns) == 0 {
		return data, nil
	}

	// Parse the result to extract tools array while preserving structure.
	var result map[string]json.RawMessage
	if err := json.Unmarshal(data, &result); err != nil {
		return data, err
	}

	toolsRaw, ok := result["tools"]
	if !ok {
		return data, nil
	}

	var tools []json.RawMessage
	if err := json.Unmarshal(toolsRaw, &tools); err != nil {
		return data, err
	}

	filtered := make([]json.RawMessage, 0, len(tools))
	for _, raw := range tools {
		var entry struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(raw, &entry); err != nil {
			filtered = append(filtered, raw) // keep tools we can't parse
			continue
		}
		if !ShouldIgnore(patterns, entry.Name) {
			filtered = append(filtered, raw)
		}
	}

	result["tools"], _ = json.Marshal(filtered)
	return json.Marshal(result)
}

// IsToolCallBlocked returns true if the tools/call params reference an ignored tool.
func IsToolCallBlocked(params []byte, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	var call struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(params, &call); err != nil {
		return false
	}
	return ShouldIgnore(patterns, call.Name)
}
