package commands

import (
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/spf13/cobra"
)

// CLIRunner is the dependency-injected execution surface for leaf
// commands. The CLI wiring provides a concrete implementation in
// internal/cli that calls the HTTP executor and output formatter.
type CLIRunner interface {
	RunOperation(cmd *cobra.Command, method, path string, op *openapi3.Operation, sf SpecFlags) error
}

// Builder turns an OpenAPI 3 spec into a Cobra command tree grouped
// by tag, with one Cobra subcommand per operation.
type Builder struct {
	Spec  *openapi3.T
	Runner CLIRunner
}

// NewBuilder constructs a Builder from a loaded spec and a CLIRunner.
func NewBuilder(spec *openapi3.T, runner CLIRunner) *Builder {
	return &Builder{Spec: spec, Runner: runner}
}

// Build produces the root cobra.Command populated with all the dynamic
// command groups derived from the spec paths. Each tag becomes a
// subcommand group, with leaf commands for each operation.
func (b *Builder) Build() *cobra.Command {
	root := &cobra.Command{
		Use:   "invgate-cli",
		Short: "Runtime OpenAPI/Swagger CLI",
	}
	if b.Spec != nil && b.Spec.Info != nil && b.Spec.Info.Title != "" {
		root.Short = b.Spec.Info.Title
	}
	// Defensive guard: a nil Spec yields an empty root command (no subcommands).
	if b.Spec == nil || b.Spec.Paths == nil {
		return root
	}

	// Group operations by tag, preserving insertion order.
	type opRef struct {
		method string
		path   string
		op     *openapi3.Operation
	}
	groups := make(map[string][]opRef)
	groupOrder := []string{}

	addToGroup := func(tag string, r opRef) {
		if _, ok := groups[tag]; !ok {
			groupOrder = append(groupOrder, tag)
		}
		groups[tag] = append(groups[tag], r)
	}

	for path, pathItem := range b.Spec.Paths.Map() {
		if pathItem == nil {
			continue
		}
		// GET, POST, PUT, PATCH, DELETE in spec-defined order.
		ops := []struct {
			method string
			op     *openapi3.Operation
		}{
			{"GET", pathItem.Get},
			{"POST", pathItem.Post},
			{"PUT", pathItem.Put},
			{"PATCH", pathItem.Patch},
			{"DELETE", pathItem.Delete},
		}
		for _, entry := range ops {
			if entry.op == nil {
				continue
			}
			tag := pickTag(entry.op)
			addToGroup(tag, opRef{method: entry.method, path: path, op: entry.op})
		}
	}

	// Sort group order alphabetically (kebab-case) for stable --help.
	sort.Strings(groupOrder)

	// Build one Cobra command per tag, with leaf commands inside.
	for _, tag := range groupOrder {
		groupCmd := &cobra.Command{
			Use:   tag,
			Short: fmt.Sprintf("Operations on %s", tag),
		}
		for _, r := range groups[tag] {
			leaf := b.buildLeaf(r.method, r.path, r.op, tag, pathItemFor(b.Spec, r.path))
			groupCmd.AddCommand(leaf)
		}
		root.AddCommand(groupCmd)
	}

	return root
}

func pathItemFor(spec *openapi3.T, path string) openapi3.Parameters {
	pi := spec.Paths.Map()[path]
	if pi == nil {
		return nil
	}
	return pi.Parameters
}

func (b *Builder) buildLeaf(method, path string, op *openapi3.Operation, tag string, pathParams openapi3.Parameters) *cobra.Command {
	sf := extractFlags(pathParams, op)
	leaf := &cobra.Command{
		Use:   leafUse(method, path, op, sf),
		Short: pickShort(op),
		Long:  pickLong(op),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !validatePathArgs(sf, args) {
				return fmt.Errorf("expected %d positional argument(s), got %d", len(sf.PathArgs), len(args))
			}
			return b.Runner.RunOperation(cmd, method, path, op, sf)
		},
	}
	sf.registerFlags(leaf)
	return leaf
}

// leafUse constructs the leaf command name, e.g. "list" or "read <id>".
// Nested resources like /vendors/{vendor_id}/contacts/ produce
// "contacts list --vendor-id <vendor-id>" via a sub-group at the tag
// level — handled in this v0.1 by collapsing the tag path and letting
// the OperationID drive the leaf name.
func leafUse(method, path string, op *openapi3.Operation, sf SpecFlags) string {
	_, action := splitOperationID(op.OperationID)
	if action == "" {
		action = methodAction(method, path)
	}
	// Double-underscore collapses to hyphens in the action alone.
	action = strings.ReplaceAll(action, "__", "-")
	if len(sf.PathArgs) > 0 {
		var b strings.Builder
		b.WriteString(action)
		for _, pa := range sf.PathArgs {
			b.WriteString(" <")
			b.WriteString(flagName(pa.Name))
			b.WriteString(">")
		}
		return b.String()
	}
	return action
}

func pickTag(op *openapi3.Operation) string {
	if len(op.Tags) > 0 {
		return normalize(op.Tags[0])
	}
	return "default"
}

func pickShort(op *openapi3.Operation) string {
	if op.Summary != "" {
		return op.Summary
	}
	if op.Description != "" {
		return truncate(op.Description, 80)
	}
	return op.OperationID
}

func pickLong(op *openapi3.Operation) string {
	if op.Description != "" {
		return op.Description
	}
	if op.Summary != "" {
		return op.Summary
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

func validatePathArgs(sf SpecFlags, args []string) bool {
	return len(args) == len(sf.PathArgs)
}