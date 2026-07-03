package commands

import (
	"fmt"
	"os"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// FlagDef is a lightweight description of a Cobra flag produced from
// an OpenAPI 3 operation parameter. It is used by the builder to
// register flags on the leaf commands.
type FlagDef struct {
	Name        string
	Shorthand   string
	Description string
	Type        string // "string", "bool", "int", "stringArray"
	Required    bool
	IsBody      bool
	Default     any
}

// PathArg is a positional argument derived from a path parameter.
type PathArg struct {
	Name string
}

// SpecFlags bundles all flag/arg metadata extracted from an operation.
type SpecFlags struct {
	PathArgs  []PathArg
	QueryFlags []FlagDef
	BodyFlag  *FlagDef
}

// extractFlags inspects an openapi3.Operation and its path parameters
// to produce the SpecFlags used by the builder. Path parameters become
// positional args, query parameters become flags, and a requestBody
// becomes a `--data` flag (accepting "@file" and inline JSON).
func extractFlags(pathParamRefs openapi3.Parameters, op *openapi3.Operation) SpecFlags {
	var sf SpecFlags

	// op.Parameters may include both path and query (sometimes duplicated
	// from pathItem-level params). Count both.
	allParams := mergeParameters(pathParamRefs, op.Parameters)

	for _, pRef := range allParams {
		p := pRef.Value
		if p == nil {
			continue
		}
		switch p.In {
		case "path":
			sf.PathArgs = append(sf.PathArgs, PathArg{Name: p.Name})
		case "query":
			fd := queryFlagDef(p)
			if fd != nil {
				sf.QueryFlags = append(sf.QueryFlags, *fd)
			}
		}
	}

	if op.RequestBody != nil && op.RequestBody.Value != nil {
		sf.BodyFlag = &FlagDef{
			Name:        "data",
			Description: bodyDescription(op.RequestBody.Value),
			Type:        "string",
			Required:     op.RequestBody.Value.Required,
			IsBody:      true,
		}
	}
	return sf
}

func mergeParameters(a, b openapi3.Parameters) openapi3.Parameters {
	seen := make(map[string]bool)
	out := make(openapi3.Parameters, 0, len(a)+len(b))
	for _, p := range a {
		if p.Value != nil {
			seen[p.Value.In+":"+p.Value.Name] = true
		}
		out = append(out, p)
	}
	for _, p := range b {
		if p.Value != nil {
			key := p.Value.In + ":" + p.Value.Name
			if seen[key] {
				continue
			}
			seen[key] = true
		}
		out = append(out, p)
	}
	return out
}

func queryFlagDef(p *openapi3.Parameter) *FlagDef {
	if p.Schema == nil || p.Schema.Value == nil {
		// Fallback: string flag.
		fd := &FlagDef{
			Name:        flagName(p.Name),
			Description: parameterDescription(p),
			Type:        "string",
			Required:    p.Required,
		}
		return fd
	}
	s := p.Schema.Value
	fd := &FlagDef{
		Name:        flagName(p.Name),
		Description: parameterDescription(p),
		Required:    p.Required,
	}
	switch {
	case s.Type.Is("boolean"):
		fd.Type = "bool"
	case s.Type.Is("integer"):
		fd.Type = "int"
	case s.Type.Is("array"):
		fd.Type = "stringArray"
	case s.Type.Is("string"), s.Type == nil:
		fd.Type = "string"
	default:
		fd.Type = "string"
	}
	if s.Default != nil {
		fd.Default = s.Default
	}
	return fd
}

func parameterDescription(p *openapi3.Parameter) string {
	parts := make([]string, 0, 2)
	if p.Description != "" {
		parts = append(parts, p.Description)
	}
	if p.Example != nil {
		parts = append(parts, fmt.Sprintf("example: %v", p.Example))
	}
	return strings.Join(parts, " — ")
}

func bodyDescription(rb *openapi3.RequestBody) string {
	if rb == nil {
		return "Request body (JSON inline or @file.json)"
	}
	if rb.Description != "" {
		return rb.Description + " (JSON inline or @file.json)"
	}
	return "Request body (JSON inline or @file.json)"
}

// registerFlags applies the SpecFlags to a Cobra command, wiring up
// pflag declarations and marking required flags.
func (sf *SpecFlags) registerFlags(cmd *cobra.Command) {
	flags := cmd.Flags()
	for i := range sf.QueryFlags {
		fd := &sf.QueryFlags[i]
		switch fd.Type {
		case "bool":
			flags.Bool(fd.Name, false, fd.Description)
		case "int":
			flags.Int(fd.Name, 0, fd.Description)
		case "stringArray":
			flags.StringArray(fd.Name, nil, fd.Description)
		default:
			flags.String(fd.Name, "", fd.Description)
		}
		if fd.Required {
			_ = cmd.MarkFlagRequired(fd.Name)
		}
	}
	if sf.BodyFlag != nil {
		flags.String(sf.BodyFlag.Name, "", sf.BodyFlag.Description)
		if sf.BodyFlag.Required {
			_ = cmd.MarkFlagRequired(sf.BodyFlag.Name)
		}
	}
}

// resolveData reads the `--data` flag value, expanding "@file.json"
// syntax into the file's contents. Returns the raw value otherwise.
func resolveData(raw string) ([]byte, error) {
	if raw == "" {
		return nil, nil
	}
	if strings.HasPrefix(raw, "@") {
		path := strings.TrimPrefix(raw, "@")
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("could not read --data file %s: %w", path, err)
		}
		return data, nil
	}
	return []byte(raw), nil
}

// pflagHas returns true if a flag was explicitly set on the flag set.
func pflagHas(fs *pflag.FlagSet, name string) bool {
	if fs == nil {
		return false
	}
	flag := fs.Lookup(name)
	return flag != nil && flag.Changed
}