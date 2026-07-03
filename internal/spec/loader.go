// Package spec provides runtime loading of Swagger 2.0 and OpenAPI 3
// specifications, converting Swagger 2.0 to OpenAPI 3 via openapi2conv.
package spec

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/getkin/kin-openapi/openapi2"
	"github.com/getkin/kin-openapi/openapi2conv"
	"github.com/getkin/kin-openapi/openapi3"
	"gopkg.in/yaml.v3"
)

// Loader loads an OpenAPI/Swagger spec from a file path and returns
// a normalized OpenAPI 3 document.
type Loader interface {
	Load(path string) (*openapi3.T, error)
}

// FileLoader reads specs from the filesystem, detecting Swagger 2.0
// and converting to OpenAPI 3 automatically.
type FileLoader struct {
	Logger *log.Logger
}

// NewFileLoader creates a FileLoader with a stderr logger.
func NewFileLoader() *FileLoader {
	return &FileLoader{Logger: log.New(os.Stderr, "", 0)}
}

// Load reads the file at path, detects the spec version, and returns
// a validated OpenAPI 3 document. Swagger 2.0 specs are converted via
// openapi2conv. YAML and JSON are both supported.
func (l *FileLoader) Load(path string) (*openapi3.T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("could not read spec file %s: %w", path, err)
	}

	// Parse as YAML first (YAML is a superset of JSON).
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("could not parse spec file %s: %w", path, err)
	}

	// Detect Swagger 2.0 by the "swagger" field.
	if isSwagger2(raw) {
		// Some real-world Swagger 2.0 specs use "examples" as an array
		// where kin-openapi's openapi2.T expects a map[string]any.
		// Strip "examples" from response objects before conversion —
		// examples are documentation-only and not needed at runtime.
		sanitizeSwagger2(raw)
		// Re-encode to JSON so openapi2 can unmarshal it.
		jsonData, err := json.Marshal(raw)
		if err != nil {
			return nil, fmt.Errorf("could not re-encode Swagger 2.0 spec: %w", err)
		}
		var doc2 openapi2.T
		if err := json.Unmarshal(jsonData, &doc2); err != nil {
			return nil, fmt.Errorf("could not parse Swagger 2.0 spec %s: %w", path, err)
		}
		doc3, err := openapi2conv.ToV3(&doc2)
		if err != nil {
			return nil, fmt.Errorf("could not convert Swagger 2.0 to OpenAPI 3: %w", err)
		}
		// Validation is best-effort: real-world Swagger 2.0 specs often
		// have non-conforming identifiers (e.g. security schemes with
		// spaces) that the strict OpenAPI 3 validator rejects. Log
		// validation warnings but do not block loading.
		loader := openapi3.NewLoader()
		if err := doc3.Validate(loader.Context); err != nil {
			if l.Logger != nil {
				l.Logger.Printf("warning: spec validation issue: %v", err)
			}
		}
		return doc3, nil
	}

	// OpenAPI 3: parse directly.
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromData(data)
	if err != nil {
		return nil, fmt.Errorf("could not parse OpenAPI 3 spec %s: %w", path, err)
	}
	if doc.OpenAPI == "" {
		return nil, fmt.Errorf("unsupported spec version: file %s is neither Swagger 2.0 nor OpenAPI 3", path)
	}
	if err := doc.Validate(loader.Context); err != nil {
		if l.Logger != nil {
			l.Logger.Printf("warning: spec validation issue: %v", err)
		}
	}
	return doc, nil
}

// isSwagger2 returns true if the raw map has a "swagger" field with value "2.0".
func isSwagger2(raw map[string]any) bool {
	v, ok := raw["swagger"]
	if !ok {
		return false
	}
	s, ok := v.(string)
	if !ok {
		return fmt.Sprintf("%v", v) == "2.0" || fmt.Sprintf("%v", v) == "2"
	}
	return s == "2.0" || s == "2"
}

// DetectFormat returns "json" or "yaml" based on the file extension.
func DetectFormat(path string) string {
	if strings.HasSuffix(path, ".json") {
		return "json"
	}
	return "yaml"
}

// sanitizeSwagger2 cleans up non-conforming parts of real-world Swagger 2.0
// specs so that kin-openapi's openapi2.T can unmarshal them:
//   - "examples" arrays on responses become maps (or are dropped),
//   - security scheme names with spaces are renamed and references updated.
func sanitizeSwagger2(raw map[string]any) {
	stripArrayExamples(raw)
	renameSecuritySchemes(raw)
}

// stripArrayExamples removes example arrays from responses that openapi2.T
// expects as map[string]any.
func stripArrayExamples(raw map[string]any) {
	paths, ok := raw["paths"].(map[string]any)
	if !ok {
		return
	}
	for _, pathItem := range paths {
		pi, ok := pathItem.(map[string]any)
		if !ok {
			continue
		}
		for _, op := range pi {
			opMap, ok := op.(map[string]any)
			if !ok {
				continue
			}
			responses, ok := opMap["responses"].(map[string]any)
			if !ok {
				continue
			}
			for _, resp := range responses {
				respMap, ok := resp.(map[string]any)
				if !ok {
					continue
				}
				if _, ok := respMap["examples"].([]any); ok {
					delete(respMap, "examples")
				}
			}
		}
	}
}

// nameMap records old→new names for security schemes whose original keys
// contain characters not allowed in OpenAPI 3 identifiers
// (charset: A-Za-z0-9._-).
var oas3InvalidChars = " !\"#$%&'()*+,/:;<=>?@[\\]^`{|}~"

func sanitizeSchemeName(name string) string {
	if isOAS3ValidName(name) {
		return name
	}
	var b strings.Builder
	for _, r := range name {
		if strings.ContainsRune(oas3InvalidChars, r) {
			b.WriteByte('-')
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isOAS3ValidName(name string) bool {
	for _, r := range name {
		if !(r >= 'a' && r <= 'z') &&
			!(r >= 'A' && r <= 'Z') &&
			!(r >= '0' && r <= '9') &&
			r != '.' && r != '_' && r != '-' {
			return false
		}
	}
	return name != ""
}

// renameSecuritySchemes renames security scheme keys containing invalid
// characters and updates all security references under "security" arrays.
func renameSecuritySchemes(raw map[string]any) {
	sd, ok := raw["securityDefinitions"].(map[string]any)
	if !ok {
		return
	}
	nameMap := make(map[string]string)
	for k, v := range sd {
		scheme, ok := v.(map[string]any)
		if !ok {
			continue
		}
		nk := sanitizeSchemeName(k)
		if nk != k {
			nameMap[k] = nk
			delete(sd, k)
			scheme["x-original-name"] = k
			sd[nk] = v
		}
	}
	if len(nameMap) == 0 {
		return
	}
	replaceSecurityRefs(raw, nameMap)
}

func replaceSecurityRefs(raw map[string]any, nameMap map[string]string) {
	// Top-level security
	if sec, ok := raw["security"].([]any); ok {
		raw["security"] = remapSecurityList(sec, nameMap)
	}
	// Per-operation security
	paths, ok := raw["paths"].(map[string]any)
	if !ok {
		return
	}
	for _, pathItem := range paths {
		pi, ok := pathItem.(map[string]any)
		if !ok {
			continue
		}
		for _, op := range pi {
			opMap, ok := op.(map[string]any)
			if !ok {
				continue
			}
			if sec, ok := opMap["security"].([]any); ok {
				opMap["security"] = remapSecurityList(sec, nameMap)
			}
		}
	}
}

func remapSecurityList(sec []any, nameMap map[string]string) []any {
	out := make([]any, 0, len(sec))
	for _, item := range sec {
		m, ok := item.(map[string]any)
		if !ok {
			out = append(out, item)
			continue
		}
		newM := make(map[string]any, len(m))
		for k, v := range m {
			if nk, changed := nameMap[k]; changed {
				newM[nk] = v
			} else {
				newM[k] = v
			}
		}
		out = append(out, newM)
	}
	return out
}

func sanitizeSwagger2Examples(raw map[string]any) {
	stripArrayExamples(raw)
}