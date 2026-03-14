package app

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func TestServerHandlerRoutesByPrefix(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, r.URL.Path+"?"+r.URL.RawQuery+"|"+r.Header.Get("X-Forwarded-Prefix"))
	}))
	defer upstream.Close()

	server := &Server{
		Addr:   "127.0.0.1:7777",
		Routes: []Route{{Name: "api", URL: upstream.URL + "/base"}},
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
