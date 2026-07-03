package spec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestFileLoader_LoadSwagger2(t *testing.T) {
	loader := NewFileLoader()
	doc, err := loader.Load(filepath.Join("testdata", "swagger2-petstore.json"))
	if err != nil {
		t.Fatalf("Load swagger2: %v", err)
	}
	if doc == nil {
		t.Fatal("doc should not be nil")
	}
	if doc.OpenAPI == "" {
		t.Error("OpenAPI field should be set after conversion")
	}
	verifyPetstore(t, doc)
}

func TestFileLoader_LoadOAS3YAML(t *testing.T) {
	loader := NewFileLoader()
	doc, err := loader.Load(filepath.Join("testdata", "oas3-petstore.yaml"))
	if err != nil {
		t.Fatalf("Load oas3 yaml: %v", err)
	}
	if doc.OpenAPI == "" {
		t.Error("OpenAPI field should be set")
	}
	verifyPetstore(t, doc)
}

func TestFileLoader_LoadMissingFile(t *testing.T) {
	loader := NewFileLoader()
	_, err := loader.Load(filepath.Join("testdata", "does-not-exist.json"))
	if err == nil {
		t.Fatal("expected error for missing file")
	}
	if !strings.Contains(err.Error(), "could not read spec file") {
		t.Errorf("error should mention read failure, got: %v", err)
	}
}

func TestFileLoader_LoadBrokenJSON(t *testing.T) {
	loader := NewFileLoader()
	_, err := loader.Load(filepath.Join("testdata", "broken.json"))
	if err == nil {
		t.Fatal("expected error for broken JSON")
	}
	if !strings.Contains(err.Error(), "could not parse") {
		t.Errorf("error should mention parse failure, got: %v", err)
	}
}

func TestFileLoader_LoadEmptyJSON(t *testing.T) {
	loader := NewFileLoader()
	_, err := loader.Load(filepath.Join("testdata", "empty.json"))
	if err == nil {
		t.Fatal("expected error for empty JSON (no version)")
	}
	if !strings.Contains(err.Error(), "unsupported spec version") {
		t.Errorf("error should mention unsupported, got: %v", err)
	}
}

func TestLoadActualInvGateSpec(t *testing.T) {
	specPath := filepath.Join("..", "..", "..", "invgate-swagger-v2.json")
	if _, err := os.Stat(specPath); err != nil {
		t.Skip("invgate swagger spec not found, skipping")
	}
	loader := NewFileLoader()
	doc, err := loader.Load(specPath)
	if err != nil {
		t.Fatalf("Load invgate spec: %v", err)
	}
	if doc.Paths == nil || len(doc.Paths.Map()) == 0 {
		t.Error("invgate spec should have paths")
	}
}

func TestSanitizeSchemeName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"OAuth2 Application", "OAuth2-Application"},
		{"valid_name", "valid_name"},
		{"a.b-c_d", "a.b-c_d"},
		{"x/y", "x-y"},
		{"", ""},
		{"name with spaces!", "name-with-spaces-"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := sanitizeSchemeName(tt.in); got != tt.want {
				t.Errorf("sanitizeSchemeName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestIsOAS3ValidName(t *testing.T) {
	for _, name := range []string{"abc", "A-B_C.d", "scheme123"} {
		if !isOAS3ValidName(name) {
			t.Errorf("expected %q to be valid", name)
		}
	}
	for _, name := range []string{"", "a b", "a/b", "a:b"} {
		if isOAS3ValidName(name) {
			t.Errorf("expected %q to be invalid", name)
		}
	}
}

func TestIsSwagger2(t *testing.T) {
	tests := []struct {
		name string
		raw  map[string]any
		want bool
	}{
		{"swagger 2.0 string", map[string]any{"swagger": "2.0"}, true},
		{"swagger 2 string", map[string]any{"swagger": "2"}, true},
		{"openapi 3", map[string]any{"openapi": "3.0.0"}, false},
		{"empty", map[string]any{}, false},
		{"swagger 1.0", map[string]any{"swagger": "1.0"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isSwagger2(tt.raw); got != tt.want {
				t.Errorf("isSwagger2() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"spec.json", "json"},
		{"spec.yaml", "yaml"},
		{"spec.yml", "yaml"},
		{"spec.txt", "yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := DetectFormat(tt.path); got != tt.want {
				t.Errorf("DetectFormat(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}

func verifyPetstore(t *testing.T, doc *openapi3.T) {
	t.Helper()
	if doc.Paths == nil {
		t.Fatal("Paths should not be nil")
	}
	paths := doc.Paths.Map()
	if len(paths) < 2 {
		t.Errorf("expected at least 2 paths, got %d", len(paths))
	}
	if _, ok := paths["/pet/"]; !ok {
		t.Error("path /pet/ should exist")
	}
	if _, ok := paths["/pet/{id}/"]; !ok {
		t.Error("path /pet/{id}/ should exist")
	}
}