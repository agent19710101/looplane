package app

import (
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
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	statuses := CheckRoutes([]Route{{Name: "api", URL: upstream.URL}}, time.Second)
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

func TestCheckRoutesFallsBackToGetWhenHeadNotAllowed(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	statuses := CheckRoutes([]Route{{Name: "docs", URL: upstream.URL}}, time.Second)
	if len(statuses) != 1 {
		t.Fatalf("expected 1 status, got %d", len(statuses))
	}
	if !statuses[0].OK || statuses[0].StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %#v", statuses[0])
	}
}

func TestCheckRoutesReportsUnreachableTarget(t *testing.T) {
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
