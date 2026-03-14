package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
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

func TestRunImportDevportRadar(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()
	input, err := os.CreateTemp(t.TempDir(), "radar-*.json")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := input.WriteString(`[{"port":3000,"protocol":"http","alias":"api"}]`); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	os.Stdin = input

	stdout, stderr, err := captureRunOutput([]string{"import", "devport-radar"})
	if err != nil {
		t.Fatalf("import devport-radar: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "added=1") {
		t.Fatalf("unexpected import output: %s", stdout)
	}

	stdout, stderr, err = captureRunOutput([]string{"ls", "--json"})
	if err != nil {
		t.Fatalf("ls --json: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "\"name\": \"api\"") || !strings.Contains(stdout, "\"url\": \"http://127.0.0.1:3000\"") {
		t.Fatalf("json output missing imported route: %s", stdout)
	}
}

func TestRunImportDockerPS(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()
	input, err := os.CreateTemp(t.TempDir(), "docker-*.jsonl")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := input.WriteString("{\"Names\":\"api\",\"Image\":\"ghcr.io/acme/api:latest\",\"Ports\":\"0.0.0.0:8080->80/tcp\"}\n"); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if _, err := input.Seek(0, 0); err != nil {
		t.Fatalf("Seek: %v", err)
	}
	os.Stdin = input

	stdout, stderr, err := captureRunOutput([]string{"import", "docker-ps"})
	if err != nil {
		t.Fatalf("import docker-ps: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "imported docker-ps routes: added=1") {
		t.Fatalf("unexpected docker import output: %s", stdout)
	}

	stdout, stderr, err = captureRunOutput([]string{"ls", "--json"})
	if err != nil {
		t.Fatalf("ls --json: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "\"name\": \"api\"") || !strings.Contains(stdout, "\"url\": \"http://127.0.0.1:8080\"") {
		t.Fatalf("json output missing docker route: %s", stdout)
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

func TestRunLSJSONCheckUsesStableFlatSchema(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()

	if err := run([]string{"add", "api", upstream.URL}); err != nil {
		t.Fatalf("add api: %v", err)
	}

	stdout, stderr, err := captureRunOutput([]string{"ls", "--json", "--check"})
	if err != nil {
		t.Fatalf("ls --json --check: %v\nstderr=%s", err, stderr)
	}

	var got []map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("unmarshal checked json: %v\noutput=%s", err, stdout)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 checked route, got %d", len(got))
	}
	item := got[0]
	for _, key := range []string{"name", "url", "ok", "status_code", "message"} {
		if _, ok := item[key]; !ok {
			t.Fatalf("missing key %q in checked json: %#v", key, item)
		}
	}
	if _, ok := item["Route"]; ok {
		t.Fatalf("unexpected legacy Route field in checked json: %#v", item)
	}
	if _, ok := item["StatusCode"]; ok {
		t.Fatalf("unexpected legacy StatusCode field in checked json: %#v", item)
	}
}

func TestRunCompletionBash(t *testing.T) {
	stdout, stderr, err := captureRunOutput([]string{"completion", "bash"})
	if err != nil {
		t.Fatalf("completion bash: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "complete -F _looplane looplane") {
		t.Fatalf("unexpected bash completion output: %s", stdout)
	}
	if !strings.Contains(stdout, "devport-radar") || !strings.Contains(stdout, "docker-ps") {
		t.Fatalf("bash completion missing import sources: %s", stdout)
	}
	if !strings.Contains(stdout, "looplane __complete routes \"$cur\" \"${store_args[@]}\"") {
		t.Fatalf("bash completion missing shared-store route completion: %s", stdout)
	}
}

func TestRunCompletionFish(t *testing.T) {
	stdout, stderr, err := captureRunOutput([]string{"completion", "fish"})
	if err != nil {
		t.Fatalf("completion fish: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "complete -c looplane") {
		t.Fatalf("unexpected fish completion output: %s", stdout)
	}
	if !strings.Contains(stdout, "__fish_seen_subcommand_from completion") {
		t.Fatalf("fish completion missing shell completion entries: %s", stdout)
	}
	if !strings.Contains(stdout, "(__looplane_store_args)") {
		t.Fatalf("fish completion missing shared-store route completion: %s", stdout)
	}
}

func TestRunCompletionZsh(t *testing.T) {
	stdout, stderr, err := captureRunOutput([]string{"completion", "zsh"})
	if err != nil {
		t.Fatalf("completion zsh: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "#compdef looplane") {
		t.Fatalf("unexpected zsh completion output: %s", stdout)
	}
	if !strings.Contains(stdout, "_looplane_store_args") {
		t.Fatalf("zsh completion missing shared-store helper: %s", stdout)
	}
	if !strings.Contains(stdout, "store_args=(${reply[@]})") {
		t.Fatalf("zsh completion missing shared-store route completion: %s", stdout)
	}
	if !strings.Contains(stdout, "rm)") || !strings.Contains(stdout, "open)") {
		t.Fatalf("zsh completion missing route-aware rm/open handling: %s", stdout)
	}
}

func TestRunCompletionPowerShell(t *testing.T) {
	stdout, stderr, err := captureRunOutput([]string{"completion", "powershell"})
	if err != nil {
		t.Fatalf("completion powershell: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "Register-ArgumentCompleter -Native -CommandName looplane") {
		t.Fatalf("unexpected PowerShell completion output: %s", stdout)
	}
	if !strings.Contains(stdout, "$storeArgs = @()") {
		t.Fatalf("PowerShell completion missing shared-store helper: %s", stdout)
	}
	if !strings.Contains(stdout, "looplane __complete routes $wordToComplete @storeArgs") {
		t.Fatalf("PowerShell completion missing shared-store route completion: %s", stdout)
	}
	if !strings.Contains(stdout, "'open'") || !strings.Contains(stdout, "'rm'") {
		t.Fatalf("PowerShell completion missing route-aware open/rm handling: %s", stdout)
	}
}

func TestRunCompletionRejectsUnsupportedShell(t *testing.T) {
	_, _, err := captureRunOutput([]string{"completion", "nushell"})
	if err == nil || !strings.Contains(err.Error(), "unsupported shell") {
		t.Fatalf("expected unsupported shell error, got %v", err)
	}
}

func TestRunCompleteRoutesUsesStoreAndPrefix(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := run([]string{"add", "api", "http://127.0.0.1:3000"}); err != nil {
		t.Fatalf("add api: %v", err)
	}
	if err := run([]string{"add", "docs", "http://127.0.0.1:4321/base"}); err != nil {
		t.Fatalf("add docs: %v", err)
	}
	if err := run([]string{"add", "admin", "http://127.0.0.1:9000"}); err != nil {
		t.Fatalf("add admin: %v", err)
	}

	stdout, stderr, err := captureRunOutput([]string{"__complete", "routes", "a"})
	if err != nil {
		t.Fatalf("__complete routes: %v\nstderr=%s", err, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "admin\napi" {
		t.Fatalf("unexpected completion output: %q", got)
	}
}

func TestRunCompleteRoutesUsesSharedStorePath(t *testing.T) {
	sharedDir := t.TempDir()
	sharedStore := filepath.Join(sharedDir, "team", "routes.json")

	if err := run([]string{"add", "api", "http://127.0.0.1:3000", "--store", sharedStore}); err != nil {
		t.Fatalf("add api with shared store: %v", err)
	}
	if err := run([]string{"add", "admin", "http://127.0.0.1:9000", "--store", sharedStore}); err != nil {
		t.Fatalf("add admin with shared store: %v", err)
	}

	stdout, stderr, err := captureRunOutput([]string{"__complete", "routes", "a", "--store", sharedStore})
	if err != nil {
		t.Fatalf("__complete routes with shared store: %v\nstderr=%s", err, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "admin\napi" {
		t.Fatalf("unexpected shared-store completion output: %q", got)
	}
}

func TestRunCompleteRoutesRejectsUnknownTarget(t *testing.T) {
	_, _, err := captureRunOutput([]string{"__complete", "shells"})
	if err == nil || !strings.Contains(err.Error(), "unknown completion target") {
		t.Fatalf("expected unknown completion target error, got %v", err)
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

func TestRunSupportsSharedStorePath(t *testing.T) {
	sharedDir := t.TempDir()
	sharedStore := filepath.Join(sharedDir, "team", "routes.json")

	if err := run([]string{"add", "api", "http://127.0.0.1:3000", "--store", sharedStore}); err != nil {
		t.Fatalf("add route with shared store: %v", err)
	}

	stdout, stderr, err := captureRunOutput([]string{"ls", "--json", "--store", sharedStore})
	if err != nil {
		t.Fatalf("ls --json with shared store: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "\"name\": \"api\"") {
		t.Fatalf("shared store output missing route: %s", stdout)
	}

	stdout, stderr, err = captureRunOutput([]string{"open", "api", "--store", sharedStore})
	if err != nil {
		t.Fatalf("open with shared store: %v\nstderr=%s", err, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "http://127.0.0.1:7777/api/" {
		t.Fatalf("unexpected open output from shared store: %q", got)
	}
}

func TestResolveCommandStoreRejectsMissingPath(t *testing.T) {
	_, _, _, err := resolveCommandStore([]string{"ls", "--store"})
	if err == nil || !strings.Contains(err.Error(), "--store requires a path") {
		t.Fatalf("expected missing store path error, got %v", err)
	}
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

func TestRunOpenSupportsHostSuffix(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := run([]string{"add", "api", "http://127.0.0.1:3000"}); err != nil {
		t.Fatalf("add route: %v", err)
	}

	stdout, stderr, err := captureRunOutput([]string{"open", "api", "--host-suffix", "localtest.me"})
	if err != nil {
		t.Fatalf("open route with host suffix: %v\nstderr=%s", err, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "http://api.localtest.me:7777/" {
		t.Fatalf("unexpected host-based open output: %q", got)
	}
}

func TestRunOpenSupportsHTTPS(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	if err := run([]string{"add", "api", "http://127.0.0.1:3000"}); err != nil {
		t.Fatalf("add route: %v", err)
	}

	stdout, stderr, err := captureRunOutput([]string{"open", "api", "--host-suffix", "localtest.me", "--https"})
	if err != nil {
		t.Fatalf("open route with https: %v\nstderr=%s", err, stderr)
	}
	if got := strings.TrimSpace(stdout); got != "https://api.localtest.me:7777/" {
		t.Fatalf("unexpected https open output: %q", got)
	}
}

func TestRunServeRejectsHalfTLSConfig(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())

	_, _, err := captureRunOutput([]string{"serve", "--tls-cert", "./cert.pem"})
	if err == nil || !strings.Contains(err.Error(), "serve requires both --tls-cert and --tls-key together") {
		t.Fatalf("expected TLS pair error, got %v", err)
	}
}

func TestRunCompletionIncludesHostSuffixFlags(t *testing.T) {
	stdout, stderr, err := captureRunOutput([]string{"completion", "bash"})
	if err != nil {
		t.Fatalf("completion bash: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "--addr --host-suffix --tls-cert --tls-key --watch --store") || !strings.Contains(stdout, "--addr --host-suffix --https --store") {
		t.Fatalf("bash completion missing HTTPS/TLS flags: %s", stdout)
	}

	stdout, stderr, err = captureRunOutput([]string{"completion", "zsh"})
	if err != nil {
		t.Fatalf("completion zsh: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "--host-suffix[optional host-based routing suffix]") || !strings.Contains(stdout, "--tls-cert[path to TLS certificate]") || !strings.Contains(stdout, "--https[print an HTTPS URL for TLS-enabled local proxy setups]") {
		t.Fatalf("zsh completion missing HTTPS/TLS flags: %s", stdout)
	}

	stdout, stderr, err = captureRunOutput([]string{"completion", "fish"})
	if err != nil {
		t.Fatalf("completion fish: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "-l host-suffix") || !strings.Contains(stdout, "-l tls-cert") || !strings.Contains(stdout, "-l tls-key") || !strings.Contains(stdout, "-l https") {
		t.Fatalf("fish completion missing HTTPS/TLS flags: %s", stdout)
	}

	stdout, stderr, err = captureRunOutput([]string{"completion", "powershell"})
	if err != nil {
		t.Fatalf("completion powershell: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(stdout, "'--host-suffix'") || !strings.Contains(stdout, "'--tls-cert'") || !strings.Contains(stdout, "'--tls-key'") || !strings.Contains(stdout, "'--https'") {
		t.Fatalf("powershell completion missing HTTPS/TLS flags: %s", stdout)
	}
}
