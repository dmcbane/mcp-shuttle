package filter

import (
	"testing"
)

func TestMatchesTool(t *testing.T) {
	tests := []struct {
		pattern string
		name    string
		want    bool
	}{
		{"delete*", "delete_user", true},
		{"delete*", "delete", true},
		{"delete*", "remove_user", false},
		{"*admin*", "super_admin_tool", true},
		{"*admin*", "admin", true},
		{"*admin*", "user_tool", false},
		{"exact_match", "exact_match", true},
		{"exact_match", "not_match", false},
		{"dangerous_*", "dangerous_delete", true},
		{"dangerous_*", "safe_delete", false},
	}

	for _, tt := range tests {
		got := MatchesTool(tt.pattern, tt.name)
		if got != tt.want {
			t.Errorf("MatchesTool(%q, %q) = %v, want %v", tt.pattern, tt.name, got, tt.want)
		}
	}
}

func TestShouldIgnoreTool(t *testing.T) {
	patterns := []string{"delete*", "*admin*", "dangerous_tool"}

	tests := []struct {
		name string
		want bool
	}{
		{"delete_user", true},
		{"super_admin_panel", true},
		{"dangerous_tool", true},
		{"safe_read", false},
		{"list_items", false},
	}

	for _, tt := range tests {
		got := ShouldIgnore(patterns, tt.name)
		if got != tt.want {
			t.Errorf("ShouldIgnore(%v, %q) = %v, want %v", patterns, tt.name, got, tt.want)
		}
	}
}

func TestFilterToolsList(t *testing.T) {
	// Simulate a tools/list response with tools array.
	input := `{"tools":[{"name":"read_file","description":"Read a file"},{"name":"delete_file","description":"Delete a file"},{"name":"admin_panel","description":"Admin panel"}]}`
	patterns := []string{"delete*", "*admin*"}

	result, err := FilterToolsListResponse([]byte(input), patterns)
	if err != nil {
		t.Fatalf("FilterToolsListResponse: %v", err)
	}

	// Parse result to verify.
	expected := `{"tools":[{"name":"read_file","description":"Read a file"}]}`
	if string(result) != expected {
		t.Errorf("got:\n%s\nwant:\n%s", result, expected)
	}
}

func TestFilterToolsList_EmptyPatterns(t *testing.T) {
	input := `{"tools":[{"name":"read_file"},{"name":"delete_file"}]}`
	result, err := FilterToolsListResponse([]byte(input), nil)
	if err != nil {
		t.Fatalf("FilterToolsListResponse: %v", err)
	}
	if string(result) != input {
		t.Errorf("with no patterns, output should equal input\ngot:  %s\nwant: %s", result, input)
	}
}

func TestShouldAllow(t *testing.T) {
	patterns := []string{"read_*", "list_*", "get_info"}

	tests := []struct {
		name string
		want bool
	}{
		{"read_file", true},
		{"list_items", true},
		{"get_info", true},
		{"delete_file", false},
		{"admin_panel", false},
	}

	for _, tt := range tests {
		got := ShouldAllow(patterns, tt.name)
		if got != tt.want {
			t.Errorf("ShouldAllow(%v, %q) = %v, want %v", patterns, tt.name, got, tt.want)
		}
	}
}

func TestShouldAllow_EmptyPatterns(t *testing.T) {
	// Empty allow-list means allow everything.
	if !ShouldAllow(nil, "anything") {
		t.Error("empty allow patterns should allow everything")
	}
}

func TestIsToolCallBlocked(t *testing.T) {
	patterns := []string{"delete*", "admin_*"}

	params := `{"name":"delete_user","arguments":{}}`
	if !IsToolCallBlocked([]byte(params), patterns) {
		t.Error("expected delete_user to be blocked")
	}

	params = `{"name":"read_file","arguments":{}}`
	if IsToolCallBlocked([]byte(params), patterns) {
		t.Error("expected read_file to NOT be blocked")
	}
}
