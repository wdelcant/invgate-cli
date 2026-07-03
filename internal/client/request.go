// Package client builds and executes HTTP requests for invgate-cli.
// The Executor attaches OAuth2 bearer tokens via the auth.Manager
// and auto-retries once on 401 by calling Login (which forces a
// fresh token) before re-executing the request.
package client

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// Request is the high-level description of an API request, before
// auth and base URL are applied. PathTemplate uses {name} placeholders
// that get substituted by PathArgs in declaration order.
type Request struct {
	Method       string
	PathTemplate string
	PathArgs     []string
	Query        url.Values
	Body         []byte
	ContentType  string
}

// Response wraps the raw HTTP response after it's been fully read.
type Response struct {
	Status int
	Body   []byte
	Header http.Header
}

// BuildHTTPRequest converts a Request into an *http.Request against
// the given base URL. It substitutes path placeholders with URL-encoded
// args, encodes query params, sets Content-Type, and writes the body.
// No Authorization header is added here — that's the executor's job.
func BuildHTTPRequest(baseURL string, req *Request) (*http.Request, error) {
	if req.Method == "" {
		return nil, fmt.Errorf("request method is required")
	}
	if req.PathTemplate == "" {
		return nil, fmt.Errorf("request path template is required")
	}

	// Substitute {param} placeholders with URL-encoded args in order.
	path := req.PathTemplate
	for _, arg := range req.PathArgs {
		idx := strings.Index(path, "{")
		if idx < 0 {
			break
		}
		end := strings.Index(path[idx:], "}")
		if end < 0 {
			return nil, fmt.Errorf("malformed path template: %s", req.PathTemplate)
		}
		placeholder := path[idx : idx+end+1]
		path = strings.Replace(path, placeholder, url.PathEscape(arg), 1)
	}

	// Join base URL and path, avoiding duplicated slashes.
	fullURL := joinURL(baseURL, path)
	if len(req.Query) > 0 {
		fullURL = fullURL + "?" + req.Query.Encode()
	}

	var body io.Reader
	if len(req.Body) > 0 {
		body = bytes.NewReader(req.Body)
	}
	httpReq, err := http.NewRequest(req.Method, fullURL, body)
	if err != nil {
		return nil, fmt.Errorf("could not build request: %w", err)
	}
	if len(req.Body) > 0 {
		ct := req.ContentType
		if ct == "" {
			ct = "application/json"
		}
		httpReq.Header.Set("Content-Type", ct)
	}
	return httpReq, nil
}

// joinURL concatenates baseURL and path, preventing duplicated slashes
// at the junction.
func joinURL(base, path string) string {
	base = strings.TrimRight(base, "/")
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}