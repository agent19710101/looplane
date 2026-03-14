package app

import (
	"strings"
	"testing"
)

func TestImportDevportRadarJSONMergesRoutes(t *testing.T) {
	existing := []Route{{Name: "docs", URL: "http://127.0.0.1:4321"}}
	input := strings.NewReader(`[
  {"port":3000,"protocol":"http","alias":"api"},
  {"port":5173,"protocol":"http","process":"vite dev"}
]`)

	result, err := ImportDevportRadarJSON(existing, input, ImportOptions{})
	if err != nil {
		t.Fatalf("ImportDevportRadarJSON: %v", err)
	}
	if result.Added != 2 || result.Updated != 0 || result.Skipped != 0 {
		t.Fatalf("unexpected import summary: %#v", result)
	}
	if _, ok := FindRoute(result.Routes, "api"); !ok {
		t.Fatalf("expected imported api route: %#v", result.Routes)
	}
	if route, ok := FindRoute(result.Routes, "vite-dev"); !ok || route.URL != "http://127.0.0.1:5173" {
		t.Fatalf("expected sanitized process route, got %#v", result.Routes)
	}
	if _, ok := FindRoute(result.Routes, "docs"); !ok {
		t.Fatalf("expected existing route to remain: %#v", result.Routes)
	}
}

func TestImportDevportRadarJSONDisambiguatesNames(t *testing.T) {
	input := strings.NewReader(`[
  {"port":3000,"protocol":"http","alias":"web"},
  {"port":3001,"protocol":"http","alias":"web"}
]`)

	result, err := ImportDevportRadarJSON(nil, input, ImportOptions{})
	if err != nil {
		t.Fatalf("ImportDevportRadarJSON: %v", err)
	}
	if _, ok := FindRoute(result.Routes, "web"); !ok {
		t.Fatalf("expected base route name: %#v", result.Routes)
	}
	if route, ok := FindRoute(result.Routes, "web-3001"); !ok || route.URL != "http://127.0.0.1:3001" {
		t.Fatalf("expected port-suffixed route, got %#v", result.Routes)
	}
}

func TestImportDevportRadarJSONReplace(t *testing.T) {
	existing := []Route{{Name: "old", URL: "http://127.0.0.1:9000"}}
	input := strings.NewReader(`[{"port":8080,"protocol":"http","process":"api"}]`)

	result, err := ImportDevportRadarJSON(existing, input, ImportOptions{Replace: true})
	if err != nil {
		t.Fatalf("ImportDevportRadarJSON: %v", err)
	}
	if len(result.Routes) != 1 {
		t.Fatalf("expected exactly one route, got %#v", result.Routes)
	}
	if _, ok := FindRoute(result.Routes, "old"); ok {
		t.Fatalf("expected replace import to drop existing routes: %#v", result.Routes)
	}
}

func TestImportDockerPSJSONImportsPublishedPorts(t *testing.T) {
	input := strings.NewReader(`{"Names":"looplane-api-1","Image":"ghcr.io/acme/api:latest","Ports":"0.0.0.0:8080->80/tcp, [::]:8080->80/tcp"}
{"Names":"grafana","Image":"grafana/grafana","Ports":"127.0.0.1:3001->3000/tcp"}
{"Names":"db","Image":"postgres:16","Ports":"5432/tcp"}
`)

	result, err := ImportDockerPSJSON(nil, input, ImportOptions{})
	if err != nil {
		t.Fatalf("ImportDockerPSJSON: %v", err)
	}
	if result.Added != 2 || result.Updated != 0 || result.Skipped != 1 {
		t.Fatalf("unexpected docker import summary: %#v", result)
	}
	if route, ok := FindRoute(result.Routes, "looplane-api-1"); !ok || route.URL != "http://127.0.0.1:8080" {
		t.Fatalf("expected api route, got %#v", result.Routes)
	}
	if route, ok := FindRoute(result.Routes, "grafana"); !ok || route.URL != "http://127.0.0.1:3001" {
		t.Fatalf("expected grafana route, got %#v", result.Routes)
	}
}

func TestImportDockerPSJSONHandlesArraysAndMultiplePorts(t *testing.T) {
	input := strings.NewReader(`[
  {"Names":"traefik","Image":"traefik:v3","Ports":"0.0.0.0:80->80/tcp, 0.0.0.0:443->443/tcp"}
]`)

	result, err := ImportDockerPSJSON(nil, input, ImportOptions{})
	if err != nil {
		t.Fatalf("ImportDockerPSJSON: %v", err)
	}
	if _, ok := FindRoute(result.Routes, "traefik"); !ok {
		t.Fatalf("expected first docker port to keep base name: %#v", result.Routes)
	}
	if route, ok := FindRoute(result.Routes, "traefik-443"); !ok || route.URL != "http://127.0.0.1:443" {
		t.Fatalf("expected second docker port route, got %#v", result.Routes)
	}
}
