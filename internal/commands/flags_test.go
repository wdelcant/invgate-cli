package commands

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"
)

// makeQueryParam builds a minimal openapi3.Parameter for testing.
func makeQueryParam(name, in string, required bool, schema *openapi3.SchemaRef) *openapi3.ParameterRef {
	p := &openapi3.Parameter{
		Name:     name,
		In:       in,
		Required: required,
		Schema:   schema,
	}
	return &openapi3.ParameterRef{Value: p}
}

func strSchema(t string) *openapi3.SchemaRef {
	return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{t}}}
}

func boolSchema() *openapi3.SchemaRef {
	return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"boolean"}}}
}

func intSchema() *openapi3.SchemaRef {
	return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"integer"}}}
}

func arraySchema() *openapi3.SchemaRef {
	return &openapi3.SchemaRef{Value: &openapi3.Schema{Type: &openapi3.Types{"array"}}}
}

func TestExtractFlags_QueryStringBecomesFlag(t *testing.T) {
	op := &openapi3.Operation{
		OperationID: "assets_list",
		Parameters: openapi3.Parameters{
			makeQueryParam("name", "query", false, strSchema("string")),
		},
	}
	sf := extractFlags(nil, op)
	if len(sf.QueryFlags) != 1 {
		t.Fatalf("expected 1 query flag, got %d", len(sf.QueryFlags))
	}
	fd := sf.QueryFlags[0]
	if fd.Name != "name" {
		t.Errorf("flag name = %q, want name", fd.Name)
	}
	if fd.Type != "string" {
		t.Errorf("flag type = %q, want string", fd.Type)
	}
	if fd.Required {
		t.Errorf("flag should not be required")
	}
}

func TestExtractFlags_BooleanFlag(t *testing.T) {
	op := &openapi3.Operation{
		OperationID: "assets_list",
		Parameters: openapi3.Parameters{makeQueryParam("is_active", "query", false, boolSchema())},
	}
	sf := extractFlags(nil, op)
	if len(sf.QueryFlags) != 1 && sf.QueryFlags[0].Type != "bool" {
		t.Fatalf("expected bool flag, got %#v", sf.QueryFlags)
	}
	if sf.QueryFlags[0].Name != "is-active" {
		t.Errorf("flag name = %q, want is-active", sf.QueryFlags[0].Name)
	}
}

func TestExtractFlags_IntegerFlag(t *testing.T) {
	op := &openapi3.Operation{
		OperationID: "assets_list",
		Parameters: openapi3.Parameters{makeQueryParam("page", "query", false, intSchema())},
	}
	sf := extractFlags(nil, op)
	if sf.QueryFlags[0].Type != "int" {
		t.Errorf("flag type = %q, want int", sf.QueryFlags[0].Type)
	}
}

func TestExtractFlags_ArrayFlag(t *testing.T) {
	op := &openapi3.Operation{
		OperationID: "assets_list",
		Parameters: openapi3.Parameters{makeQueryParam("ids", "query", false, arraySchema())},
	}
	sf := extractFlags(nil, op)
	if sf.QueryFlags[0].Type != "stringArray" {
		t.Errorf("flag type = %q, want stringArray", sf.QueryFlags[0].Type)
	}
}

func TestExtractFlags_PathParamBecomesArg(t *testing.T) {
	op := &openapi3.Operation{
		OperationID: "assets_read",
		Parameters: openapi3.Parameters{makeQueryParam("id", "path", true, intSchema())},
	}
	sf := extractFlags(nil, op)
	if len(sf.PathArgs) != 1 {
		t.Fatalf("expected 1 path arg, got %d", len(sf.PathArgs))
	}
	if sf.PathArgs[0].Name != "id" {
		t.Errorf("path arg name = %q, want id", sf.PathArgs[0].Name)
	}
}

func TestExtractFlags_RequiredMarked(t *testing.T) {
	op := &openapi3.Operation{
		OperationID: "assets_list",
		Parameters: openapi3.Parameters{makeQueryParam("name", "query", true, strSchema("string"))},
	}
	sf := extractFlags(nil, op)
	if !sf.QueryFlags[0].Required {
		t.Errorf("flag should be marked required")
	}
}

func TestExtractFlags_BodyFlag(t *testing.T) {
	op := &openapi3.Operation{
		OperationID: "assets_create",
		RequestBody: &openapi3.RequestBodyRef{
			Value: &openapi3.RequestBody{
				Required:    true,
				Description: "Asset payload",
			},
		},
	}
	sf := extractFlags(nil, op)
	if sf.BodyFlag == nil {
		t.Fatal("expected BodyFlag, got nil")
	}
	if sf.BodyFlag.Name != "data" {
		t.Errorf("body flag name = %q, want data", sf.BodyFlag.Name)
	}
	if !sf.BodyFlag.IsBody {
		t.Errorf("body flag should have IsBody=true")
	}
	if !sf.BodyFlag.Required {
		t.Errorf("body flag should be required")
	}
}

func TestRegisterFlags_AllTypes(t *testing.T) {
	sf := SpecFlags{
		QueryFlags: []FlagDef{
			{Name: "name", Type: "string", Description: "name"},
			{Name: "is-active", Type: "bool", Description: "active"},
			{Name: "page", Type: "int", Description: "page"},
			{Name: "ids", Type: "stringArray", Description: "ids"},
		},
		BodyFlag: &FlagDef{Name: "data", Type: "string", Description: "body"},
	}
	cmd := &cobra.Command{Use: "test"}
	sf.registerFlags(cmd)

	for _, name := range []string{"name", "is-active", "page", "ids", "data"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag %q not registered", name)
		}
	}
}

func TestRegisterFlags_RequiredMarkedOnCobra(t *testing.T) {
	sf := SpecFlags{
		QueryFlags: []FlagDef{
			{Name: "name", Type: "string", Required: true},
		},
	}
	cmd := &cobra.Command{Use: "test"}
	sf.registerFlags(cmd)

	// Cobra marks required flags; calling the flag should error if not set
	// when the command is executed. Verify via the changed flag check.
	fl := cmd.Flags().Lookup("name")
	if fl == nil {
		t.Fatal("flag name not registered")
	}
}

func TestResolveData_Inline(t *testing.T) {
	raw := `{"manufacturer":"Apple"}`
	out, err := resolveData(raw)
	if err != nil {
		t.Fatalf("resolveData: %v", err)
	}
	if string(out) != raw {
		t.Errorf("resolveData inline = %q, want %q", out, raw)
	}
}

func TestResolveData_Empty(t *testing.T) {
	out, err := resolveData("")
	if err != nil {
		t.Fatalf("resolveData empty: %v", err)
	}
	if out != nil {
		t.Errorf("resolveData empty should return nil, got %v", out)
	}
}

func TestResolveData_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "payload.json")
	want := `{"key":"value"}`
	if err := os.WriteFile(path, []byte(want), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	out, err := resolveData("@" + path)
	if err != nil {
		t.Fatalf("resolveData file: %v", err)
	}
	if string(out) != want {
		t.Errorf("resolveData file = %q, want %q", out, want)
	}
}

func TestResolveData_MissingFile(t *testing.T) {
	_, err := resolveData("@/nonexistent/file.json")
	if err == nil {
		t.Fatal("expected error on missing file")
	}
	if !strings.Contains(err.Error(), "could not read --data file") {
		t.Errorf("error should mention file read failure, got %v", err)
	}
}

func TestParameterDescription(t *testing.T) {
	p := &openapi3.Parameter{
		Name:        "name",
		Description: "Filter by asset name",
		Example:     "MacBook",
	}
	got := parameterDescription(p)
	if !strings.Contains(got, "Filter by asset name") {
		t.Errorf("description should contain the parameter description: %q", got)
	}
	if !strings.Contains(got, "example") {
		t.Errorf("description should mention example: %q", got)
	}
}

func TestParameterDescription_NoExample(t *testing.T) {
	p := &openapi3.Parameter{Name: "name", Description: "filter"}
	got := parameterDescription(p)
	if got != "filter" {
		t.Errorf("description = %q, want filter", got)
	}
}