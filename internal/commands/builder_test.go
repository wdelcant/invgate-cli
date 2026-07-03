package commands

import (
	"os"
	"strings"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"
)

// stubRunner records the last operation the builder delegated to.
type stubRunner struct {
	called  bool
	lastMethod string
	lastPath   string
	lastOp     *openapi3.Operation
}

func (s *stubRunner) RunOperation(cmd *cobra.Command, method, path string, op *openapi3.Operation, sf SpecFlags) error {
	s.called = true
	s.lastMethod = method
	s.lastPath = path
	s.lastOp = op
	return nil
}

// loadTestSpec loads the test API spec from testdata/.
func loadTestSpec(t *testing.T) *openapi3.T {
	t.Helper()
	data, err := os.ReadFile("testdata/test-api.yaml")
	if err != nil {
		t.Fatalf("read testdata: %v", err)
	}
	loader := openapi3.NewLoader()
	loader.IsExternalRefsAllowed = false
	doc, err := loader.LoadFromData(data)
	if err != nil {
		t.Fatalf("load spec: %v", err)
	}
	return doc
}

func TestBuilder_Build_TagsBecomeGroups(t *testing.T) {
	doc := loadTestSpec(t)
	runner := &stubRunner{}
	b := NewBuilder(doc, runner)
	root := b.Build()

	if root == nil {
		t.Fatal("Build returned nil")
	}
	if root.Use != "invgate-cli" {
		t.Errorf("root Use = %q, want invgate-cli", root.Use)
	}

	// Find "assets" group via Find args
	_, _, err := root.Find([]string{"assets"})
	if err != nil {
		t.Fatalf("expected 'assets' group, got error: %v", err)
	}
	_, _, err = root.Find([]string{"vendors"})
	if err != nil {
		t.Fatalf("expected 'vendors' group, got error: %v", err)
	}
}

func TestBuilder_Build_NormalizedTagNames(t *testing.T) {
	doc := loadTestSpec(t)
	runner := &stubRunner{}
	root := NewBuilder(doc, runner).Build()

	// Tag "Assets" should become "assets" — kebab-case lowercase.
	for _, cmd := range root.Commands() {
		if strings.HasPrefix(cmd.Use, "assets") || strings.HasPrefix(cmd.Use, "vendors") || cmd.Use == "default" {
			continue // found a valid group
		}
	}
	// Ensure no group is named "Assets" with capital.
	for _, cmd := range root.Commands() {
		if cmd.Name() == "Assets" {
			t.Errorf("tag 'Assets' should be normalized to 'assets'")
		}
	}
}

func TestBuilder_Build_LeafCommands(t *testing.T) {
	doc := loadTestSpec(t)
	runner := &stubRunner{}
	root := NewBuilder(doc, runner).Build()

	cmd, _, err := root.Find([]string{"assets"})
	if err != nil {
		t.Fatalf("find assets: %v", err)
	}

	// Expect subcommands: list, create, read, update, delete
	wantCommands := map[string]bool{
		"list":   false,
		"create": false,
		"read":   false,
		"update": false,
		"delete": false,
	}
	for _, sub := range cmd.Commands() {
		// The leaf Use is like "list", "read <id>", etc. Name() is the first word.
		name := sub.Name()
		if _, ok := wantCommands[name]; ok {
			wantCommands[name] = true
		}
	}
	for name, found := range wantCommands {
		if !found {
			t.Errorf("expected 'assets %s' subcommand not found", name)
		}
	}
}

func TestBuilder_Build_PathArgInUse(t *testing.T) {
	doc := loadTestSpec(t)
	runner := &stubRunner{}
	root := NewBuilder(doc, runner).Build()

	cmd, _, err := root.Find([]string{"assets", "read"})
	if err != nil {
		t.Fatalf("find assets read: %v", err)
	}
	if !strings.Contains(cmd.Use, "<id>") {
		t.Errorf("'read' leaf Use should contain <id> positional, got %q", cmd.Use)
	}
}

func TestBuilder_Build_FlagsFromQuery(t *testing.T) {
	doc := loadTestSpec(t)
	runner := &stubRunner{}
	root := NewBuilder(doc, runner).Build()

	cmd, _, err := root.Find([]string{"assets", "list"})
	if err != nil {
		t.Fatalf("find assets list: %v", err)
	}
	// Expect flags: --name, --is-active, --ids, --required-field, and body? no body on list.
	for _, name := range []string{"name", "is-active", "ids"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("flag --%s should be registered on 'assets list'", name)
		}
	}
	// required_field renamed to kebab-case "required-field".
	if cmd.Flags().Lookup("required-field") == nil {
		t.Errorf("flag --required-field should be registered on 'assets list'")
	}
}

func TestBuilder_Build_BodyFlagOnPost(t *testing.T) {
	doc := loadTestSpec(t)
	runner := &stubRunner{}
	root := NewBuilder(doc, runner).Build()

	cmd, _, err := root.Find([]string{"assets", "create"})
	if err != nil {
		t.Fatalf("find assets create: %v", err)
	}
	if cmd.Flags().Lookup("data") == nil {
		t.Errorf("--data flag should be registered on 'assets create'")
	}
}

func TestBuilder_Build_UntaggedToDefault(t *testing.T) {
	doc := loadTestSpec(t)
	runner := &stubRunner{}
	root := NewBuilder(doc, runner).Build()

	// The /ping/ operation has no tags — should go to "default" group.
	defCmd, _, err := root.Find([]string{"default"})
	if err != nil {
		t.Fatalf("expected 'default' group for untagged ops: %v", err)
	}
	// ping_read → splitOperationID → ("ping", "read") → leaf name is the action "read".
	found := false
	for _, sub := range defCmd.Commands() {
		if sub.Name() == "read" {
			found = true
		}
	}
	if !found {
		t.Errorf("untagged ping_read operation should be under 'default' as leaf 'read'")
	}
}

func TestBuilder_Build_VendorsGroup(t *testing.T) {
	doc := loadTestSpec(t)
	runner := &stubRunner{}
	root := NewBuilder(doc, runner).Build()

	// Tagged "Vendors" → group "vendors".
	vCmd, _, err := root.Find([]string{"vendors"})
	if err != nil {
		t.Fatalf("find vendors: %v", err)
	}
	// The nested resource "vendor_contacts_list" — top-level tag is "vendors".
	// OperationID split → ("vendor", "contacts") — wait, split_on_last_underscore
	// yields ("vendor_contacts", "list"). So leaf "list" under vendors.
	foundList := false
	for _, sub := range vCmd.Commands() {
		if sub.Name() == "list" {
			foundList = true
		}
	}
	if !foundList {
		t.Errorf("expected 'vendors list' subcommand for vendor_contacts_list")
	}
}

func TestBuilder_RunE_DelegatesToRunner(t *testing.T) {
	doc := loadTestSpec(t)
	runner := &stubRunner{}
	root := NewBuilder(doc, runner).Build()

	cmd, _, err := root.Find([]string{"assets", "list"})
	if err != nil {
		t.Fatalf("find assets list: %v", err)
	}
	cmd.SetArgs([]string{})
	_ = cmd.RunE(cmd, []string{})

	if !runner.called {
		t.Error("RunE should have delegated to the runner")
	}
	if runner.lastMethod != "GET" {
		t.Errorf("runner method = %q, want GET", runner.lastMethod)
	}
	if runner.lastPath != "/assets/" {
		t.Errorf("runner path = %q, want /assets/", runner.lastPath)
	}
}

func TestBuilder_Build_ShortFromSummary(t *testing.T) {
	doc := loadTestSpec(t)
	runner := &stubRunner{}
	root := NewBuilder(doc, runner).Build()

	cmd, _, err := root.Find([]string{"assets", "list"})
	if err != nil {
		t.Fatalf("find assets list: %v", err)
	}
	if !strings.Contains(cmd.Short, "List assets") {
		t.Errorf("Short should come from summary, got %q", cmd.Short)
	}
}

func TestBuilder_Build_LongFromDescription(t *testing.T) {
	doc := loadTestSpec(t)
	runner := &stubRunner{}
	root := NewBuilder(doc, runner).Build()

	cmd, _, err := root.Find([]string{"assets", "list"})
	if err != nil {
		t.Fatalf("find assets list: %v", err)
	}
	if !strings.Contains(cmd.Long, "paginated list of assets") {
		t.Errorf("Long should come from description, got %q", cmd.Long)
	}
}

func TestBuilder_Build_NilSpecReturnsEmptyRoot(t *testing.T) {
	runner := &stubRunner{}
	root := NewBuilder(nil, runner).Build()
	if root == nil {
		t.Fatal("Build with nil spec should still return a root command")
	}
	if len(root.Commands()) != 0 {
		t.Errorf("nil spec → no subcommands, got %d", len(root.Commands()))
	}
}

func TestValidatePathArgs(t *testing.T) {
	tests := []struct {
		name    string
		numArgs int
		args    []string
		want    bool
	}{
		{"match", 1, []string{"abc"}, true},
		{"too few", 2, []string{"abc"}, false},
		{"too many", 1, []string{"a", "b"}, false},
		{"exact zero", 0, []string{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sf := SpecFlags{PathArgs: make([]PathArg, tt.numArgs)}
			if got := validatePathArgs(sf, tt.args); got != tt.want {
				t.Errorf("validatePathArgs = %v, want %v", got, tt.want)
			}
		})
	}
}