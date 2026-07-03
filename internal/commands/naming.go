// Package commands builds the Cobra command tree from an OpenAPI 3
// spec. It handles tag-to-resource mapping, HTTP method-to-action
// verbs, parameter-to-flag mapping, and nested resource extraction.
package commands

import (
	"regexp"
	"strings"
)

// camelBoundaryRegex matches lowercase→CamelCase boundaries so we can
// insert hyphens when normalizing "Asset Types" → "asset-types".
var camelBoundaryRegex = regexp.MustCompile(`([a-z0-9])([A-Z])`)

var multiHyphenRegex = regexp.MustCompile(`-{2,}`)

// normalize converts "Asset Types", "asset_types", "AssetTypes" into
// "asset-types". Steps:
//  1. Insert hyphens at lowercase→CamelCase boundaries
//  2. Lowercase
//  3. Replace underscores with hyphens
//  4. Collapse repeated hyphens
//  5. Trim leading/trailing hyphens
func normalize(s string) string {
	s = camelBoundaryRegex.ReplaceAllString(s, "$1-$2")
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	s = multiHyphenRegex.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// splitOperationID divides an operationId into (resource, action) on
// the last single underscore. Double underscores ("__") collapse to a
// literal underscore in the resource name, not a separator.
//
// Examples:
//   "assets-lite_list"      → ("assets-lite", "list")
//   "tags_assign_tag"       → ("tags", "assign-tag")
//   "assets-lite__detail"   → ("assets-lite_detail", "") — no split
//
// If no underscore is found, action is empty and resource is the full ID.
func splitOperationID(id string) (resource, action string) {
	if id == "" {
		return "", ""
	}
	// Temporarily collapse "__" to a sentinel so it isn't split.
	sentinel := "\x00"
	s := strings.ReplaceAll(id, "__", sentinel)
	idx := strings.LastIndex(s, "_")
	if idx < 0 {
		// No split — entire id is the resource.
		return strings.ReplaceAll(s, sentinel, "_"), ""
	}
	resource = strings.ReplaceAll(s[:idx], sentinel, "_")
	action = strings.ReplaceAll(s[idx+1:], sentinel, "_")
	return resource, action
}

// methodAction derives an action verb from the HTTP method and path.
//
//	GET    → "list"  (collection path like "/assets/")
//	GET    → "read"  (single-resource path like "/assets/{id}/")
//	POST   → "create"
//	PUT    → "update"
//	PATCH  → "partial-update"
//	DELETE → "delete"
func methodAction(method, path string) string {
	segs := splitPathSegments(path)
	// read vs list is decided by the LAST path segment: a trailing {param}
	// means a single resource (read); a trailing collection name means list.
	lastIsParam := false
	if len(segs) > 0 {
		last := segs[len(segs)-1]
		lastIsParam = strings.HasPrefix(last, "{") && strings.HasSuffix(last, "}")
	}
	switch strings.ToUpper(method) {
	case "GET":
		if lastIsParam {
			return "read"
		}
		return "list"
	case "POST":
		return "create"
	case "PUT":
		return "update"
	case "PATCH":
		return "partial-update"
	case "DELETE":
		return "delete"
	}
	return strings.ToLower(method)
}

// splitPathSegments returns the non-empty segments of a URL path.
// A leading/trailing slash produces no empty entries.
func splitPathSegments(p string) []string {
	parts := strings.Split(p, "/")
	out := make([]string, 0, len(parts))
	for _, s := range parts {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// flagName converts a snake_case parameter name to a kebab-case flag name.
// Example: "is_active" → "is-active", "vendorId" → "vendor-id".
func flagName(name string) string {
	return normalize(name)
}