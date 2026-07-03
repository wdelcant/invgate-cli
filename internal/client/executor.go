package client

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/wdelcant/invgate-cli/internal/errors"
)

// Manager is the subset of auth.Manager required by the executor.
// We redeclare it locally to avoid an import cycle and keep the
// client package decoupled from the auth implementation.
type AuthManager interface {
	Token(ctx context.Context) (string, error)
	Login(ctx context.Context) error
}

// Executor runs HTTP requests against the API, attaching a bearer
// token fetched via the AuthManager. On 401 it forces a Login and
// retries once before surfacing the error.
type Executor struct {
	BaseURL  string
	Auth     AuthManager
	HTTP     *http.Client
	Verbose  bool
	Logf     func(format string, args ...any)
}

// NewExecutor constructs an Executor with a default HTTP client if
// none is provided. The AuthManager may be nil for unauthenticated
// operations (e.g. health checks).
func NewExecutor(baseURL string, auth AuthManager, client *http.Client) *Executor {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Executor{
		BaseURL: baseURL,
		Auth:    auth,
		HTTP:    client,
	}
}

// Execute sends the given Request, returns the parsed Response.
// Non-2xx status codes produce an *errors.AppError with the status
// and a brief message. On 401 it calls Auth.Login and retries once.
func (e *Executor) Execute(ctx context.Context, req *Request) (*Response, error) {
	return e.do(ctx, req, true, true)
}

// do performs the actual HTTP round-trip. When attachToken is false no
// Authorization header is attached. canRetry gates the 401 auto-refresh
// path so a persistently-unauthorized server cannot recurse forever —
// the 401 retry happens at most ONCE per Execute call, per the spec.
func (e *Executor) do(ctx context.Context, req *Request, attachToken, canRetry bool) (*Response, error) {
	httpReq, err := BuildHTTPRequest(e.BaseURL, req)
	if err != nil {
		return nil, err
	}
	if attachToken && e.Auth != nil {
		tok, err := e.Auth.Token(ctx)
		if err != nil {
			return nil, fmt.Errorf("could not obtain access token: %w", err)
		}
		httpReq.Header.Set("Authorization", "Bearer "+tok)
	}
	if e.Verbose && e.Logf != nil {
		e.Logf("%s %s", httpReq.Method, httpReq.URL.String())
	}
	resp, err := e.HTTP.Do(httpReq.WithContext(ctx))
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr == context.Canceled || ctxErr == context.DeadlineExceeded {
			return nil, errors.NewError(0, "request timed out")
		}
		return nil, errors.NewError(0, "request failed: "+err.Error())
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	// 401: force a token refresh and retry at most once (canRetry gate).
	if resp.StatusCode == http.StatusUnauthorized && canRetry && e.Auth != nil {
		if e.Verbose && e.Logf != nil {
			e.Logf("401 received; refreshing token and retrying once")
		}
		_ = e.Auth.Login(ctx)
		// Retry with a fresh token, but disable further 401 retries.
		return e.do(ctx, req, true, false)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		verboseExtra := map[string]any{
			"url":    httpReq.URL.String(),
			"status": resp.StatusCode,
			"body":   string(body),
		}
		appErr := errors.NewError(resp.StatusCode, statusMessage(resp.StatusCode, body))
		appErr.Verbose = verboseExtra
		return &Response{Status: resp.StatusCode, Body: body, Header: resp.Header}, appErr
	}
	return &Response{Status: resp.StatusCode, Body: body, Header: resp.Header}, nil
}

// statusMessage maps well-known HTTP status codes to a user-facing
// message, falling back to the raw response body or a generic phrase.
func statusMessage(status int, body []byte) string {
	switch status {
	case 400:
		return extractErrorFromBody(body)
	case 401:
		return "unauthorized. Run 'invgate-cli setup'"
	case 403:
		return "forbidden"
	case 404:
		return "resource not found"
	case 500:
		return "server error"
	case 502:
		return "bad gateway"
	case 503:
		return "service unavailable"
	case 504:
		return "gateway timeout"
	}
	if len(body) > 0 {
		return extractErrorFromBody(body)
	}
	return fmt.Sprintf("HTTP %d", status)
}

// extractErrorFromBody tries to pull a "message" or "error" field out
// of a JSON body, or strips trailing whitespace, otherwise returns a
// generic "request failed" message.
func extractErrorFromBody(body []byte) string {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return "request failed"
	}
	// Look for a "message": "..." pattern in the response.
	if idx := bytes.Index(trimmed, []byte(`"message"`)); idx >= 0 {
		// capture up to next 200 bytes
		end := idx + 200
		if end > len(trimmed) {
			end = len(trimmed)
		}
		return string(trimmed[idx:end])
	}
	if len(trimmed) < 256 {
		return string(trimmed)
	}
	return string(trimmed[:256])
}