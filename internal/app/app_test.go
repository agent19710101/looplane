package app

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestValidateRoute(t *testing.T) {
	route, err := ValidateRoute("api", "http://127.0.0.1:3000")
	if err != nil {
		t.Fatalf("ValidateRoute returned error: %v", err)
	}
	if route.Name != "api" || route.URL != "http://127.0.0.1:3000" {
		t.Fatalf("unexpected route: %#v", route)
	}
}

func TestValidateRouteRejectsBadName(t *testing.T) {
	if _, err := ValidateRoute("API!", "http://127.0.0.1:3000"); err == nil {
		t.Fatal("expected invalid name error")
	}
}

func TestCheckRoutesReportsHealthyTarget(t *testing.T) {
	withHTTPClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodHead {
			t.Fatalf("expected HEAD probe, got %s", req.Method)
		}
		return response(req, http.StatusNoContent, ""), nil
	}))

	statuses := CheckRoutes([]Route{{Name: "api", URL: "http://api.test"}}, time.Second)
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if !statuses[0].OK || statuses[0].StatusCode != http.StatusNoContent {
		t.Fatalf("unexpected status: %#v", statuses[0])
	}
	if statuses[0].Message != "ok (204)" {
		t.Fatalf("unexpected message: %q", statuses[0].Message)
	}
}

func TestRouteNamesFiltersByPrefix(t *testing.T) {
	names := RouteNames([]Route{
		{Name: "admin", URL: "http://127.0.0.1:9000"},
		{Name: "api", URL: "http://127.0.0.1:3000"},
		{Name: "docs", URL: "http://127.0.0.1:4321/base"},
	}, "a")
	if got := strings.Join(names, ","); got != "admin,api" {
		t.Fatalf("unexpected names: %q", got)
	}
}

func TestCheckRoutesFallsBackToGetWhenHeadNotAllowed(t *testing.T) {
	var requests []string
	withHTTPClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests = append(requests, req.Method)
		if req.Method == http.MethodHead {
			return response(req, http.StatusMethodNotAllowed, ""), nil
		}
		return response(req, http.StatusOK, ""), nil
	}))

	statuses := CheckRoutes([]Route{{Name: "docs", URL: "http://docs.test"}}, time.Second)
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if !statuses[0].OK || statuses[0].StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %#v", statuses[0])
	}
	if got := strings.Join(requests, ","); got != "HEAD,GET" {
		t.Fatalf("unexpected probe sequence: %q", got)
	}
}

func TestCheckRoutesReportsUnreachableTarget(t *testing.T) {
	withHTTPClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return nil, errors.New("dial tcp 127.0.0.1:1: connect: connection refused")
	}))

	statuses := CheckRoutes([]Route{{Name: "dead", URL: "http://127.0.0.1:1"}}, 50*time.Millisecond)
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].OK {
		t.Fatalf("expected unhealthy status: %#v", statuses[0])
	}
	if !strings.HasPrefix(statuses[0].Message, "down (") {
		t.Fatalf("unexpected message: %q", statuses[0].Message)
	}
}

func TestServerHandlerRoutesByPrefix(t *testing.T) {
	var seenPath string
	var seenQuery string
	var seenPrefix string
	var seenHost string

	server := &Server{
		Addr:   "127.0.0.1:7777",
		Routes: []Route{{Name: "api", URL: "http://upstream.test/base"}},
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			seenPath = req.URL.Path
			seenQuery = req.URL.RawQuery
			seenPrefix = req.Header.Get("X-Forwarded-Prefix")
			seenHost = req.Host
			return response(req, http.StatusOK, req.URL.Path+"?"+req.URL.RawQuery+"|"+seenPrefix), nil
		}),
	}

	req := httptest.NewRequest(http.MethodGet, "/api/users?id=42", nil)
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	got := rec.Body.String()
	if !strings.Contains(got, "/base/users?id=42|/api") {
		t.Fatalf("unexpected proxy output: %q", got)
	}
	if seenPath != "/base/users" || seenQuery != "id=42" || seenPrefix != "/api" || seenHost != "upstream.test" {
		t.Fatalf("unexpected proxy request: path=%q query=%q prefix=%q host=%q", seenPath, seenQuery, seenPrefix, seenHost)
	}
}

func TestIndexIncludesRoutes(t *testing.T) {
	server := &Server{Addr: "127.0.0.1:7777", Routes: []Route{{Name: "web", URL: "http://127.0.0.1:3000"}}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	server.Handler().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "/web/ -> http://127.0.0.1:3000") {
		t.Fatalf("index missing route: %q", rec.Body.String())
	}
}

func withHTTPClient(t *testing.T, transport http.RoundTripper) {
	t.Helper()

	previous := newHTTPClient
	newHTTPClient = func(timeout time.Duration) *http.Client {
		return &http.Client{Timeout: timeout, Transport: transport}
	}
	t.Cleanup(func() {
		newHTTPClient = previous
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func response(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}
