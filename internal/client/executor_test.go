package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	ierrors "github.com/wdelcant/invgate-cli/internal/errors"
)

// fakeAuth is a controllable AuthManager for executor tests.
type fakeAuth struct {
	token           string
	postLoginToken  string // if set, Login copies this into token (simulates refresh)
	err             error
	loginCalls      int32
	tokenCalls      int32
	loginErr        error
}

func (f *fakeAuth) Token(ctx context.Context) (string, error) {
	atomic.AddInt32(&f.tokenCalls, 1)
	if f.err != nil {
		return "", f.err
	}
	return f.token, nil
}

func (f *fakeAuth) Login(ctx context.Context) error {
	atomic.AddInt32(&f.loginCalls, 1)
	if f.postLoginToken != "" {
		f.token = f.postLoginToken
	}
	return f.loginErr
}

func newExecutorWithServer(t *testing.T, srv *httptest.Server, auth AuthManager) *Executor {
	t.Helper()
	e := NewExecutor(srv.URL, auth, srv.Client())
	e.Logf = func(format string, args ...any) {} // silence verbose in tests
	return e
}

func TestExecutor_GetSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-tok" {
			t.Errorf("missing/wrong auth header: %q", r.Header.Get("Authorization"))
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"id":1,"name":"MacBook"}`))
	}))
	defer srv.Close()

	e := newExecutorWithServer(t, srv, &fakeAuth{token: "test-tok"})
	resp, err := e.Execute(context.Background(), &Request{
		Method: "GET", PathTemplate: "/assets/", Query: nil,
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("status = %d, want 200", resp.Status)
	}
	if !strings.Contains(string(resp.Body), "MacBook") {
		t.Errorf("body should contain MacBook: %q", resp.Body)
	}
}

func TestExecutor_PostWithBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("method = %q, want POST", r.Method)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), "Apple") {
			t.Errorf("request body missing Apple: %q", body)
		}
		w.WriteHeader(201)
		_, _ = w.Write([]byte(`{"id":42}`))
	}))
	defer srv.Close()

	e := newExecutorWithServer(t, srv, &fakeAuth{token: "tok"})
	resp, err := e.Execute(context.Background(), &Request{
		Method: "POST", PathTemplate: "/assets/",
		Body:   []byte(`{"manufacturer":"Apple"}`),
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Status != 201 {
		t.Errorf("status = %d, want 201", resp.Status)
	}
}

func TestExecutor_PathParamSubstitution(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/assets/77/") {
			t.Errorf("path = %q, want /assets/77/", r.URL.Path)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	e := newExecutorWithServer(t, srv, &fakeAuth{token: "tok"})
	_, err := e.Execute(context.Background(), &Request{
		Method: "GET", PathTemplate: "/assets/{id}/", PathArgs: []string{"77"},
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
}

func TestExecutor_204EmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(204)
	}))
	defer srv.Close()

	e := newExecutorWithServer(t, srv, &fakeAuth{token: "tok"})
	resp, err := e.Execute(context.Background(), &Request{
		Method: "DELETE", PathTemplate: "/assets/{id}/", PathArgs: []string{"1"},
	})
	if err != nil {
		t.Fatalf("Execute 204: %v", err)
	}
	if resp.Status != 204 {
		t.Errorf("status = %d, want 204", resp.Status)
	}
	if len(resp.Body) != 0 {
		t.Errorf("204 body should be empty, got %q", resp.Body)
	}
}

func TestExecutor_400ReturnsAppError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"errors":[{"message":"Invalid field"}]}`))
	}))
	defer srv.Close()

	e := newExecutorWithServer(t, srv, &fakeAuth{token: "tok"})
	_, err := e.Execute(context.Background(), &Request{
		Method: "POST", PathTemplate: "/assets/", Body: []byte(`{}`),
	})
	if err == nil {
		t.Fatal("expected error on 400")
	}
	if !ierrors.IsCode(err, 400) {
		t.Errorf("error should be AppError code 400, got %v", err)
	}
}

func TestExecutor_404ReturnsAppError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
	}))
	defer srv.Close()

	e := newExecutorWithServer(t, srv, &fakeAuth{token: "tok"})
	_, err := e.Execute(context.Background(), &Request{
		Method: "GET", PathTemplate: "/assets/{id}/", PathArgs: []string{"9999"},
	})
	if err == nil {
		t.Fatal("expected error on 404")
	}
	if !ierrors.IsCode(err, 404) {
		t.Errorf("error should be code 404, got %v", err)
	}
}

func TestExecutor_500ReturnsAppError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(500)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	}))
	defer srv.Close()

	e := newExecutorWithServer(t, srv, &fakeAuth{token: "tok"})
	_, err := e.Execute(context.Background(), &Request{
		Method: "GET", PathTemplate: "/assets/",
	})
	if err == nil {
		t.Fatal("expected error on 500")
	}
	if !ierrors.IsCode(err, 500) {
		t.Errorf("error should be code 500, got %v", err)
	}
}

func TestExecutor_401AutoRefreshAndRetry(t *testing.T) {
	var attempt int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempt, 1)
		if n == 1 {
			// First attempt: 401.
			w.WriteHeader(401)
			_, _ = w.Write([]byte(`{"message":"token expired"}`))
			return
		}
		// Retry after Login: succeed.
		if r.Header.Get("Authorization") != "Bearer fresh-tok" {
			t.Errorf("retry should use fresh token, got %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	auth := &fakeAuth{token: "old-tok", postLoginToken: "fresh-tok"}
	e := newExecutorWithServer(t, srv, auth)
	resp, err := e.Execute(context.Background(), &Request{
		Method: "GET", PathTemplate: "/assets/",
	})
	if err != nil {
		t.Fatalf("Execute after retry: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("status = %d, want 200 after retry", resp.Status)
	}
	if atomic.LoadInt32(&auth.loginCalls) != 1 {
		t.Errorf("Login should be called once for 401 refresh, got %d", auth.loginCalls)
	}
}

func TestExecutor_401TwiceStillErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always 401.
		w.WriteHeader(401)
		_, _ = w.Write([]byte(`{"message":"nope"}`))
	}))
	defer srv.Close()

	auth := &fakeAuth{token: "tok"}
	e := newExecutorWithServer(t, srv, auth)
	_, err := e.Execute(context.Background(), &Request{
		Method: "GET", PathTemplate: "/assets/",
	})
	if err == nil {
		t.Fatal("expected error when 401 persists after retry")
	}
	// Login should have been invoked once (the retry path), not looped forever.
	if atomic.LoadInt32(&auth.loginCalls) != 1 {
		t.Errorf("Login should be called once, got %d", auth.loginCalls)
	}
}

func TestExecutor_TokenError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	authErr := errors.New("no credentials")
	e := newExecutorWithServer(t, srv, &fakeAuth{err: authErr})
	_, err := e.Execute(context.Background(), &Request{
		Method: "GET", PathTemplate: "/assets/",
	})
	if err == nil {
		t.Fatal("expected error when token fetch fails")
	}
	if !strings.Contains(err.Error(), "access token") {
		t.Errorf("error should mention access token, got %v", err)
	}
}

func TestExecutor_NilAuthSkipsToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("no auth header should be set when Auth is nil: %q", r.Header.Get("Authorization"))
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	e := NewExecutor(srv.URL, nil, srv.Client())
	resp, err := e.Execute(context.Background(), &Request{
		Method: "GET", PathTemplate: "/ping/",
	})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if resp.Status != 200 {
		t.Errorf("status = %d, want 200", resp.Status)
	}
}

func TestExecutor_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	srv.Close() // close immediately to force a connection error

	e := newExecutorWithServer(t, srv, &fakeAuth{token: "tok"})
	_, err := e.Execute(context.Background(), &Request{
		Method: "GET", PathTemplate: "/assets/",
	})
	if err == nil {
		t.Fatal("expected network error")
	}
}

func TestExecutor_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block longer than the context deadline.
		time.Sleep(200 * time.Millisecond)
		w.WriteHeader(200)
	}))
	defer srv.Close()

	e := newExecutorWithServer(t, srv, &fakeAuth{token: "tok"})
ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := e.Execute(ctx, &Request{
		Method: "GET", PathTemplate: "/assets/",
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestExecutor_VerboseLogging(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	var logged strings.Builder
	e := NewExecutor(srv.URL, &fakeAuth{token: "tok"}, srv.Client())
	e.Verbose = true
	e.Logf = func(format string, args ...any) {
		logged.WriteString("[" + "verbose" + "] ")
	}
	_, _ = e.Execute(context.Background(), &Request{
		Method: "GET", PathTemplate: "/assets/",
	})
	if logged.Len() == 0 {
		t.Error("verbose mode should have logged something")
	}
}

func TestStatusMessage(t *testing.T) {
	tests := []struct {
		status int
		body   string
		want   string
	}{
		{400, `{"message":"Invalid field"}`, "Invalid field"},
		{401, "", "unauthorized. Run 'invgate-cli setup'"},
		{403, "", "forbidden"},
		{404, "", "resource not found"},
		{500, "", "server error"},
		{502, "", "bad gateway"},
		{503, "", "service unavailable"},
		{504, "", "gateway timeout"},
		{418, "", "HTTP 418"},
	}
	for _, tt := range tests {
		t.Run(strconv.Itoa(tt.status), func(t *testing.T) {
			got := statusMessage(tt.status, []byte(tt.body))
			if !strings.Contains(got, tt.want) {
				t.Errorf("statusMessage(%d, %q) = %q, want to contain %q", tt.status, tt.body, got, tt.want)
			}
		})
	}
}

func TestExtractErrorFromBody(t *testing.T) {
	tests := []struct {
		name string
		body string
		want string
	}{
		{"empty", "", "request failed"},
		{"with message", `{"message":"boom"}`, "boom"},
		{"plain text", "just some text", "just some text"},
		{"whitespace", "  \n trimmed \n", "trimmed"}, // trimmed returns inner
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractErrorFromBody([]byte(tt.body))
			if !strings.Contains(got, tt.want) {
				t.Errorf("extractErrorFromBody(%q) = %q, want to contain %q", tt.body, got, tt.want)
			}
		})
	}
}