package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunOpenUsesDefaultAddrAndExistingRoute(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := run([]string{"add", "api", "http://127.0.0.1:3000"}); err != nil {
		t.Fatalf("add route: %v", err)
	}

	stdout, stderr, err := captureRunOutput([]string{"open", "api"})
	if err != nil {
		t.Fatalf("open route: %v\nstderr=%s", err, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "http://127.0.0.1:7777/api/" {
		t.Fatalf("unexpected open output: %q", got)
	}
}

func TestRunOpenSupportsCustomAddrFlag(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := run([]string{"add", "docs", "http://127.0.0.1:4321/base"}); err != nil {
		t.Fatalf("add route: %v", err)
	}

	stdout, stderr, err := captureRunOutput([]string{"open", "docs", "--addr", "127.0.0.1:9090"})
	if err != nil {
		t.Fatalf("open route: %v\nstderr=%s", err, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "http://127.0.0.1:9090/docs/" {
		t.Fatalf("unexpected open output: %q", got)
	}
}

func TestRunOpenFailsForUnknownRoute(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	_, _, err := captureRunOutput([]string{"open", "missing"})
	if err == nil || !strings.Contains(err.Error(), "route missing not found") {
		t.Fatalf("expected missing route error, got %v", err)
	}
}

func TestRunLSJSON(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := run([]string{"add", "docs", "http://127.0.0.1:4321/base"}); err != nil {
		t.Fatalf("add docs: %v", err)
	}
	if err := run([]string{"add", "api", "http://127.0.0.1:3000"}); err != nil {
		t.Fatalf("add api: %v", err)
	}

	stdout, stderr, err := captureRunOutput([]string{"ls", "--json"})
	if err != nil {
		t.Fatalf("ls --json: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "\"name\": \"api\"") || !strings.Contains(stdout, "\"name\": \"docs\"") {
		t.Fatalf("json output missing routes: %s", stdout)
	}
	if !strings.Contains(stdout, "\"url\": \"http://127.0.0.1:3000\"") {
		t.Fatalf("json output missing api url: %s", stdout)
	}
}

func captureRunOutput(args []string) (string, string, error) {
	oldStdout := os.Stdout
	oldStderr := os.Stderr

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		return "", "", err
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		return "", "", err
	}

	os.Stdout = stdoutW
	os.Stderr = stderrW
	runErr := run(args)
	_ = stdoutW.Close()
	_ = stderrW.Close()
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	var stdoutBuf, stderrBuf bytes.Buffer
	_, _ = io.Copy(&stdoutBuf, stdoutR)
	_, _ = io.Copy(&stderrBuf, stderrR)
	_ = stdoutR.Close()
	_ = stderrR.Close()

	return stdoutBuf.String(), stderrBuf.String(), runErr
}

func TestDefaultStorePathUsesXDGConfigHome(t *testing.T) {
	cfg := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", cfg)
	path, err := defaultStorePath()
	if err != nil {
		t.Fatalf("defaultStorePath: %v", err)
	}
	want := filepath.Join(cfg, "looplane", "routes.json")
	if path != want {
		t.Fatalf("unexpected store path: got %q want %q", path, want)
	}
}
