package commands

import (
	"testing"
)

func TestNormalize(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"empty", "", ""},
		{"already kebab", "asset-types", "asset-types"},
		{"spaces", "Asset Types", "asset-types"},
		{"snake_case", "asset_types", "asset-types"},
		{"CamelCase", "AssetTypes", "asset-types"},
		{"mixed", "assetTypesLite", "asset-types-lite"},
		{"underscores and caps", "Asset_Types_List", "asset-types-list"},
		{"collapse hyphens", "assets--lite", "assets-lite"},
		{"trim hyphens", "-assets-", "assets"},
		{"numbers", "v2Assets", "v2-assets"},
		{"doublescore collapses", "assets-lite__detail", "assets-lite-detail"},
		{"preserves hyphen already", "assets-lite", "assets-lite"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalize(tt.in)
			if got != tt.want {
				t.Errorf("normalize(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestSplitOperationID(t *testing.T) {
	tests := []struct {
		name             string
		id               string
		wantResource     string
		wantAction       string
	}{
		{"empty", "", "", ""},
		{"simple", "assets_list", "assets", "list"},
		{"no underscore", "list", "list", ""},
		{"multi underscore splits on last", "tags_assign_tag", "tags_assign", "tag"},
		{"double underscore no split", "assets-lite__detail", "assets-lite_detail", ""},
		{"double underscore mixed", "assets__lite_list", "assets_lite", "list"},
		{"trailing underscore", "assets_", "assets", ""},
		{"leading underscore", "_list", "", "list"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, a := splitOperationID(tt.id)
			if r != tt.wantResource {
				t.Errorf("resource = %q, want %q", r, tt.wantResource)
			}
			if a != tt.wantAction {
				t.Errorf("action = %q, want %q", a, tt.wantAction)
			}
		})
	}
}

func TestMethodAction(t *testing.T) {
	tests := []struct {
		name   string
		method string
		path   string
		want   string
	}{
		{"GET collection", "GET", "/assets/", "list"},
		{"GET single", "GET", "/assets/{id}/", "read"},
		{"POST", "POST", "/assets/", "create"},
		{"PUT", "PUT", "/assets/{id}/", "update"},
		{"PATCH", "PATCH", "/assets/{id}/", "partial-update"},
		{"DELETE", "DELETE", "/assets/{id}/", "delete"},
		{"GET nested single", "GET", "/vendors/{vendor_id}/contacts/{id}/", "read"},
		{"GET nested collection", "GET", "/vendors/{vendor_id}/contacts/", "list"},
		{"lowercase method", "get", "/assets/", "list"},
		{"unknown method", "OPTIONS", "/assets/", "options"},
		{"empty path GET defaults to list", "GET", "", "list"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := methodAction(tt.method, tt.path)
			if got != tt.want {
				t.Errorf("methodAction(%q, %q) = %q, want %q", tt.method, tt.path, got, tt.want)
			}
		})
	}
}

func TestSplitPathSegments(t *testing.T) {
	tests := []struct {
		name string
		path string
		want []string
	}{
		{"empty", "", []string{}},
		{"root slash", "/", []string{}},
		{"simple", "/assets/", []string{"assets"}},
		{"nested", "/vendors/{id}/contacts/", []string{"vendors", "{id}", "contacts"}},
		{"no trailing slash", "/assets", []string{"assets"}},
		{"double slash collapses", "/a//b/", []string{"a", "b"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := splitPathSegments(tt.path)
			if len(got) != len(tt.want) {
				t.Fatalf("len = %d, want %d (%v)", len(got), len(tt.want), got)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("seg[%d] = %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestFlagName(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"is_active", "is-active"},
		{"vendorId", "vendor-id"},
		{"name", "name"},
		{"created_at", "created-at"},
		{"ID", "id"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			if got := flagName(tt.in); got != tt.want {
				t.Errorf("flagName(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}