package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agent19710101/looplane/internal/app"
)

func TestPrintDashboardIncludesHealthAndStableURL(t *testing.T) {
	var out bytes.Buffer
	printDashboard(&out,
		[]app.Route{{Name: "api", URL: "http://127.0.0.1:3000"}},
		[]app.RouteStatus{{Name: "api", URL: "http://127.0.0.1:3000", OK: true, StatusCode: http.StatusOK, Message: "ok (200)"}},
		dashboardOptions{Addr: "127.0.0.1:7777", HostSuffix: "localtest.me"},
	)

	got := out.String()
	for _, want := range []string{
		"looplane dashboard",
		"OK ok (200)",
		"http://api.localtest.me:7777/",
		"looplane open NAME --addr 127.0.0.1:7777 --host-suffix localtest.me",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("dashboard output missing %q:\n%s", want, got)
		}
	}
}

func TestPrintDashboardShowsHostFallbackForInvalidDNSName(t *testing.T) {
	var out bytes.Buffer
	printDashboard(&out,
		[]app.Route{{Name: "api_v2", URL: "http://127.0.0.1:3000"}},
		[]app.RouteStatus{{Name: "api_v2", URL: "http://127.0.0.1:3000", OK: true, StatusCode: http.StatusOK, Message: "ok (200)"}},
		dashboardOptions{Addr: "127.0.0.1:7777", HostSuffix: "localtest.me"},
	)

	got := out.String()
	if !strings.Contains(got, "not valid for host-based routing") {
		t.Fatalf("expected invalid host-routing note in dashboard:\n%s", got)
	}
	if !strings.Contains(got, "path fallback: http://127.0.0.1:7777/api_v2/") {
		t.Fatalf("expected path fallback stable url in dashboard:\n%s", got)
	}
}

func TestRunDashboardChecksRoutes(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	if err := run([]string{"add", "api", upstream.URL}); err != nil {
		t.Fatalf("add route: %v", err)
	}

	stdout, stderr, err := captureRunOutput([]string{"dashboard", "--timeout", "500ms"})
	if err != nil {
		t.Fatalf("dashboard: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "OK ok (204)") {
		t.Fatalf("expected healthy route in dashboard output:\n%s", stdout)
	}
	if !strings.Contains(stdout, "http://127.0.0.1:7777/api/") {
		t.Fatalf("expected stable path URL in dashboard output:\n%s", stdout)
	}
}
