package client

import (
	"net/url"
	"strings"
	"testing"
)

func TestBuildHTTPRequest_GetWithQuery(t *testing.T) {
	req := &Request{
		Method:       "GET",
		PathTemplate: "/assets/",
		Query:        url.Values{"name": {"MacBook"}, "page": {"1"}},
	}
	httpReq, err := BuildHTTPRequest("https://api.example.com", req)
	if err != nil {
		t.Fatalf("BuildHTTPRequest: %v", err)
	}
	if httpReq.Method != "GET" {
		t.Errorf("method = %q, want GET", httpReq.Method)
	}
	full := httpReq.URL.String()
	if !strings.HasPrefix(full, "https://api.example.com/assets/") {
		t.Errorf("URL = %q, expected base + /assets/", full)
	}
	if !strings.Contains(full, "name=MacBook") {
		t.Errorf("URL should contain name=MacBook: %q", full)
	}
	if !strings.Contains(full, "page=1") {
		t.Errorf("URL should contain page=1: %q", full)
	}
}

func TestBuildHTTPRequest_PostWithBody(t *testing.T) {
	req := &Request{
		Method:       "POST",
		PathTemplate: "/assets/",
		Body:         []byte(`{"name":"MacBook"}`),
		ContentType:  "application/json",
	}
	httpReq, err := BuildHTTPRequest("https://api.example.com", req)
	if err != nil {
		t.Fatalf("BuildHTTPRequest: %v", err)
	}
	if httpReq.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", httpReq.Header.Get("Content-Type"))
	}
	if httpReq.Body == nil {
		t.Error("body should be set for POST")
	}
}

func TestBuildHTTPRequest_DefaultContentType(t *testing.T) {
	req := &Request{
		Method:       "POST",
		PathTemplate: "/assets/",
		Body:         []byte(`{}`),
	}
	httpReq, err := BuildHTTPRequest("https://api.example.com", req)
	if err != nil {
		t.Fatalf("BuildHTTPRequest: %v", err)
	}
	if httpReq.Header.Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type default = %q, want application/json", httpReq.Header.Get("Content-Type"))
	}
}

func TestBuildHTTPRequest_PathSubstitution(t *testing.T) {
	req := &Request{
		Method:       "GET",
		PathTemplate: "/assets/{id}/",
		PathArgs:     []string{"123"},
	}
	httpReq, err := BuildHTTPRequest("https://api.example.com", req)
	if err != nil {
		t.Fatalf("BuildHTTPRequest: %v", err)
	}
	if !strings.Contains(httpReq.URL.String(), "/assets/123/") {
		t.Errorf("path substitution failed: %q", httpReq.URL.String())
	}
}

func TestBuildHTTPRequest_PathSubstitutionEncoding(t *testing.T) {
	req := &Request{
		Method:       "GET",
		PathTemplate: "/assets/{id}/",
		PathArgs:     []string{"with space"},
	}
	httpReq, err := BuildHTTPRequest("https://api.example.com", req)
	if err != nil {
		t.Fatalf("BuildHTTPRequest: %v", err)
	}
	if !strings.Contains(httpReq.URL.String(), "with%20space") {
		t.Errorf("path arg should be URL-escaped: %q", httpReq.URL.String())
	}
}

func TestBuildHTTPRequest_MultiplePathArgs(t *testing.T) {
	req := &Request{
		Method:       "GET",
		PathTemplate: "/vendors/{vendor_id}/contacts/{id}/",
		PathArgs:     []string{"v1", "c9"},
	}
	httpReq, err := BuildHTTPRequest("https://api.example.com", req)
	if err != nil {
		t.Fatalf("BuildHTTPRequest: %v", err)
	}
	full := httpReq.URL.String()
	if !strings.Contains(full, "/vendors/v1/contacts/c9/") {
		t.Errorf("multi path arg substitution failed: %q", full)
	}
}

func TestBuildHTTPRequest_MissingMethod(t *testing.T) {
	req := &Request{PathTemplate: "/assets/"}
	_, err := BuildHTTPRequest("https://api.example.com", req)
	if err == nil {
		t.Error("expected error on missing method")
	}
}

func TestBuildHTTPRequest_MissingPath(t *testing.T) {
	req := &Request{Method: "GET"}
	_, err := BuildHTTPRequest("https://api.example.com", req)
	if err == nil {
		t.Error("expected error on missing path")
	}
}

func TestBuildHTTPRequest_NoDoubledSlash(t *testing.T) {
	req := &Request{
		Method:       "GET",
		PathTemplate: "/assets/",
	}
	httpReq, err := BuildHTTPRequest("https://api.example.com/", req)
	if err != nil {
		t.Fatalf("BuildHTTPRequest: %v", err)
	}
	if strings.Contains(httpReq.URL.String(), "//assets") {
		t.Errorf("should not have doubled slash: %q", httpReq.URL.String())
	}
}

func TestJoinURL(t *testing.T) {
	tests := []struct {
		base, path, want string
	}{
		{"https://api.example.com", "/assets/", "https://api.example.com/assets/"},
		{"https://api.example.com/", "/assets/", "https://api.example.com/assets/"},
		{"https://api.example.com/", "assets/", "https://api.example.com/assets/"},
		{"https://api.example.com", "assets", "https://api.example.com/assets"},
	}
	for _, tt := range tests {
		got := joinURL(tt.base, tt.path)
		if got != tt.want {
			t.Errorf("joinURL(%q, %q) = %q, want %q", tt.base, tt.path, got, tt.want)
		}
	}
}