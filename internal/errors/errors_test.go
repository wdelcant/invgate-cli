package errors

import (
	stderrors "errors"
	"fmt"
	"strings"
	"testing"
)

func TestNewError(t *testing.T) {
	e := NewError(404, "resource not found")
	if e.Code != 404 {
		t.Errorf("Code = %d, want 404", e.Code)
	}
	if e.Message != "resource not found" {
		t.Errorf("Message = %q, want %q", e.Message, "resource not found")
	}
	if e.Cause != nil {
		t.Errorf("Cause should be nil")
	}
	want := "Error (404): resource not found"
	if e.Error() != want {
		t.Errorf("Error() = %q, want %q", e.Error(), want)
	}
}

func TestWrap(t *testing.T) {
	inner := fmt.Errorf("connection refused")
	e := Wrap(inner, 500, "server error")
	if e.Code != 500 {
		t.Errorf("Code = %d, want 500", e.Code)
	}
	if !stderrors.Is(e, inner) {
		t.Errorf("errors.Is should find wrapped cause")
	}
	if e.Unwrap() != inner {
		t.Errorf("Unwrap should return cause")
	}
}

func TestNew_noCode(t *testing.T) {
	e := New("something went wrong")
	if e.Code != 0 {
		t.Errorf("Code = %d, want 0", e.Code)
	}
	want := "Error: something went wrong"
	if e.Error() != want {
		t.Errorf("Error() = %q, want %q", e.Error(), want)
	}
}

func TestErrorf(t *testing.T) {
	e := Errorf("field %s is %s", "name", "required")
	want := "Error: field name is required"
	if e.Error() != want {
		t.Errorf("Error() = %q, want %q", e.Error(), want)
	}
}

func TestAppError_Unwrap(t *testing.T) {
	inner := fmt.Errorf("dns error")
	e := Wrap(inner, 502, "bad gateway")
	unwrapped := e.Unwrap()
	if unwrapped == nil || unwrapped.Error() != "dns error" {
		t.Errorf("Unwrap returned %v, want dns error", unwrapped)
	}
}

func TestAppError_VerboseOutput(t *testing.T) {
	tests := []struct {
		name  string
		verbose map[string]any
		empty bool
	}{
		{"nil map", nil, true},
		{"empty map", map[string]any{}, true},
		{"with values", map[string]any{"url": "https://api.example.com", "status": 500}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := &AppError{Code: 500, Message: "fail", Verbose: tt.verbose}
			out := e.VerboseOutput()
			if tt.empty {
				if out != "" {
					t.Errorf("VerboseOutput() = %q, want empty", out)
				}
			} else {
				if !strings.Contains(out, "url") || !strings.Contains(out, "status") {
					t.Errorf("VerboseOutput() should contain keys, got %q", out)
				}
			}
		})
	}
}

func TestVerboseOutput_sorted(t *testing.T) {
	e := &AppError{Verbose: map[string]any{"zebra": 1, "apple": 2, "mango": 3}}
	out := e.VerboseOutput()
	appleIdx := strings.Index(out, "apple")
	mangoIdx := strings.Index(out, "mango")
	zebraIdx := strings.Index(out, "zebra")
	if !(appleIdx < mangoIdx && mangoIdx < zebraIdx) {
		t.Errorf("VerboseOutput keys not sorted alphabetically: %q", out)
	}
}

func TestFormatError_withCode(t *testing.T) {
	e := NewError(400, "Invalid field")
	out := FormatError(e, false)
	want := "Error (400): Invalid field"
	if out != want {
		t.Errorf("FormatError(false) = %q, want %q", out, want)
	}
}

func TestFormatError_verbose(t *testing.T) {
	e := &AppError{
		Code:    500,
		Message: "server error",
		Verbose: map[string]any{"url": "https://api.example.com"},
	}
	out := FormatError(e, true)
	if !strings.Contains(out, "server error") {
		t.Errorf("FormatError(true) should include error message, got %q", out)
	}
	if !strings.Contains(out, "url") {
		t.Errorf("FormatError(true) should include url verbose key, got %q", out)
	}
}

func TestFormatError_nonAppError(t *testing.T) {
	inner := fmt.Errorf("plain error")
	out := FormatError(inner, true)
	want := "Error: plain error"
	if out != want {
		t.Errorf("FormatError for plain error = %q, want %q", out, want)
	}
}

func TestFormatError_verboseEmpty(t *testing.T) {
	e := NewError(404, "not found")
	out := FormatError(e, true)
	// Without verbose map keys, output should just be the error string
	// (no indented key:value lines).
	want := "Error (404): not found"
	if out != want {
		t.Errorf("FormatError(true) with empty Verbose = %q, want %q", out, want)
	}
}

func TestIsCode(t *testing.T) {
	e := NewError(401, "unauthorized")
	if !IsCode(e, 401) {
		t.Errorf("IsCode should be true for 401")
	}
	if IsCode(e, 404) {
		t.Errorf("IsCode should be false for 404")
	}
	inner := fmt.Errorf("x")
	we := Wrap(inner, 500, "fail")
	if !IsCode(we, 500) {
		t.Errorf("IsCode should find wrapped code")
	}
	plain := fmt.Errorf("not AppError")
	if IsCode(plain, 404) {
		t.Errorf("IsCode should be false for plain error")
	}
}

func TestAppError_Is(t *testing.T) {
	a := NewError(404, "not found")
	b := NewError(404, "not found")
	c := NewError(404, "different")
	d := NewError(500, "not found")
	if !stderrors.Is(a, b) {
		t.Errorf("identical AppErrors should be Is-equal")
	}
	if stderrors.Is(a, c) {
		t.Errorf("different message should not be Is-equal")
	}
	if stderrors.Is(a, d) {
		t.Errorf("different code should not be Is-equal")
	}
}

func TestAppError_As(t *testing.T) {
	e := NewError(403, "forbidden")
	var ae *AppError
	if !stderrors.As(e, &ae) {
		t.Errorf("errors.As should find AppError")
	}
	if ae.Code != 403 {
		t.Errorf("As extracted code = %d, want 403", ae.Code)
	}
}