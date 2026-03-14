package app

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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

func TestCheckRoutesReports4xxAsError(t *testing.T) {
	withHTTPClient(t, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return response(req, http.StatusNotFound, ""), nil
	}))

	statuses := CheckRoutes([]Route{{Name: "docs", URL: "http://docs.test"}}, time.Second)
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if statuses[0].OK {
		t.Fatalf("expected unhealthy status: %#v", statuses[0])
	}
	if statuses[0].StatusCode != http.StatusNotFound {
		t.Fatalf("unexpected status code: %#v", statuses[0])
	}
	if statuses[0].Message != "error (404)" {
		t.Fatalf("unexpected message: %q", statuses[0].Message)
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

func TestServerHandlerRejectsInvalidHostRouteNames(t *testing.T) {
	server := &Server{
		Addr:       "127.0.0.1:7777",
		HostSuffix: "localtest.me",
		Routes:     []Route{{Name: "api_v2", URL: "http://upstream.test"}},
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return response(req, http.StatusOK, "should not proxy"), nil
		}),
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://api_v2.localtest.me:7777/users", nil)
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for invalid host route, got %d", rec.Code)
	}
}

func TestServerHandlerReloadsRoutesWithoutRestart(t *testing.T) {
	routes := []Route{{Name: "api", URL: "http://api-v1.test"}}
	server := &Server{
		Addr: "127.0.0.1:7777",
		LoadRoutes: func() ([]Route, error) {
			cloned := make([]Route, len(routes))
			copy(cloned, routes)
			return cloned, nil
		},
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return response(req, http.StatusOK, req.URL.Host+req.URL.Path), nil
		}),
	}

	handler := server.Handler()

	first := httptest.NewRecorder()
	handler.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/users", nil))
	if got := strings.TrimSpace(first.Body.String()); got != "api-v1.test/users" {
		t.Fatalf("unexpected first proxy target: %q", got)
	}

	routes = []Route{{Name: "api", URL: "http://api-v2.test/base"}, {Name: "docs", URL: "http://docs.test"}}

	second := httptest.NewRecorder()
	handler.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/api/users", nil))
	if got := strings.TrimSpace(second.Body.String()); got != "api-v2.test/base/users" {
		t.Fatalf("unexpected reloaded proxy target: %q", got)
	}

	third := httptest.NewRecorder()
	handler.ServeHTTP(third, httptest.NewRequest(http.MethodGet, "/docs/", nil))
	if got := strings.TrimSpace(third.Body.String()); got != "docs.test/" {
		t.Fatalf("unexpected newly loaded route target: %q", got)
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

func TestServerHandlerRoutesByHostSuffix(t *testing.T) {
	var seenPath string
	var seenPrefix string
	var seenRoute string

	server := &Server{
		Addr:       "127.0.0.1:7777",
		HostSuffix: "localtest.me",
		Routes:     []Route{{Name: "api", URL: "http://upstream.test/base"}},
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			seenPath = req.URL.Path
			seenPrefix = req.Header.Get("X-Forwarded-Prefix")
			seenRoute = req.Header.Get("X-Looplane-Route")
			return response(req, http.StatusOK, req.URL.Path+"|"+seenPrefix+"|"+seenRoute), nil
		}),
	}

	req := httptest.NewRequest(http.MethodGet, "http://api.localtest.me:7777/users?id=42", nil)
	req.Host = "api.localtest.me:7777"
	rec := httptest.NewRecorder()
	server.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); !strings.Contains(got, "/base/users||api") {
		t.Fatalf("unexpected proxy output: %q", got)
	}
	if seenPath != "/base/users" || seenPrefix != "" || seenRoute != "api" {
		t.Fatalf("unexpected host-routed request: path=%q prefix=%q route=%q", seenPath, seenPrefix, seenRoute)
	}
}

func TestIndexIncludesHostBasedURLs(t *testing.T) {
	server := &Server{Addr: "127.0.0.1:7777", HostSuffix: "localtest.me", Routes: []Route{{Name: "web", URL: "http://127.0.0.1:3000"}}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	server.Handler().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "http://web.localtest.me:7777/ -> http://127.0.0.1:3000") {
		t.Fatalf("index missing host-based route: %q", rec.Body.String())
	}
}

func TestIndexIncludesHTTPSHostBasedURLsWhenTLSConfigured(t *testing.T) {
	server := &Server{Addr: "127.0.0.1:7777", HostSuffix: "localtest.me", TLSCert: "./cert.pem", TLSKey: "./key.pem", Routes: []Route{{Name: "web", URL: "http://127.0.0.1:3000"}}}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	server.Handler().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "https://web.localtest.me:7777/ -> http://127.0.0.1:3000") {
		t.Fatalf("index missing https host-based route: %q", rec.Body.String())
	}
}

func TestStoreSaveWritesAtomically(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "team", "routes.json")
	store := NewStore(storePath)

	if err := store.Save([]Route{{Name: "api", URL: "http://127.0.0.1:3000"}}); err != nil {
		t.Fatalf("initial save: %v", err)
	}
	before, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("read initial store: %v", err)
	}

	tempGlobs := []string{}
	previousCreateTemp := osCreateTemp
	osCreateTemp = func(dir string, pattern string) (*os.File, error) {
		file, err := previousCreateTemp(dir, pattern)
		if err == nil {
			tempGlobs = append(tempGlobs, file.Name())
		}
		return file, err
	}
	t.Cleanup(func() { osCreateTemp = previousCreateTemp })

	previousRename := osRename
	osRename = func(oldPath string, newPath string) error {
		return errors.New("simulated rename failure")
	}
	t.Cleanup(func() { osRename = previousRename })

	err = store.Save([]Route{{Name: "docs", URL: "http://127.0.0.1:4321"}})
	if err == nil || !strings.Contains(err.Error(), "simulated rename failure") {
		t.Fatalf("expected rename failure, got %v", err)
	}

	after, err := os.ReadFile(storePath)
	if err != nil {
		t.Fatalf("read store after failed atomic save: %v", err)
	}
	if string(after) != string(before) {
		t.Fatalf("atomic save should preserve previous file on failure\nbefore=%s\nafter=%s", before, after)
	}

	matches, err := filepath.Glob(filepath.Join(filepath.Dir(storePath), filepath.Base(storePath)+".*.tmp"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected temp files to be cleaned up, found %v", matches)
	}
}
