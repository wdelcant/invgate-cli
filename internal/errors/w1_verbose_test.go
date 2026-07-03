package errors

import (
	"fmt"
	"testing"
)

// TestW1_VerboseFormat demonstrates the W1 fix: when verbose is true,
// the AppError's Verbose map (url, status, body) is appended to the
// error message in the user-facing stderr format.
func TestW1_VerboseFormat(t *testing.T) {
	e := &AppError{
		Code:    500,
		Message: "server error",
		Verbose: map[string]any{
			"url":    "https://api.example.com/v1/assets",
			"status": 500,
			"body":   `{"error": "internal"}`,
		},
	}
	out := FormatError(e, true)
	fmt.Printf("--- verbose output start ---\n%s\n--- verbose output end ---\n", out)
	if !contains(out, "url:") || !contains(out, "status:") || !contains(out, "body:") {
		t.Errorf("verbose output should print url/status/body: %q", out)
	}
	if !contains(out, "https://api.example.com/v1/assets") {
		t.Errorf("verbose output should print the request URL: %q", out)
	}
	if !contains(out, `internal`) {
		t.Errorf("verbose output should print the response body: %q", out)
	}
}

func contains(s, sub string) bool { return len(s) >= len(sub) && indexOf(s, sub) >= 0 }
func indexOf(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}