# looplane

Stable names for unstable local dev ports.

`looplane` is a tiny Go CLI + reverse proxy for the annoying part of modern local development: your app moves from `:3000` to `:4321` to `:5173`, but your bookmarks, scripts, demos, and agents still want one memorable URL.

Instead of remembering ports, you give local services names:

- `api` → `http://127.0.0.1:3000`
- `docs` → `http://127.0.0.1:4321/base`
- `grafana` → `http://127.0.0.1:3001`

Then `looplane serve` exposes stable local URLs like:

- `http://127.0.0.1:7777/api/`
- `http://127.0.0.1:7777/docs/`
- `http://127.0.0.1:7777/grafana/`

## Problem

Modern local dev stacks are messy:

- frontend dev servers jump between ports
- docs and dashboards live on different base paths
- scripts and prompts hardcode yesterday's URL
- humans remember names faster than ports

`looplane` gives those moving local services a tiny, stable naming layer.

## Why now

The current OSS wave is full of agent-native tooling, local orchestration, and better terminal UX. We already have great tools for:

- finding local services
- watching ports
- orchestrating agents

What is still oddly manual is the last mile: **giving those local services stable names that humans and agents can reuse in scripts, prompts, demos, and browser tabs**.

`looplane` focuses on that narrow pain point.

## Features

- Add/update named routes with `looplane add`
- Persist routes in `~/.config/looplane/routes.json`
- List routes with `looplane ls`
- JSON output for scripts and agents with `looplane ls --json`
- Stable flat health-check JSON with `looplane ls --json --check`
- Import routes from `devport-radar --json`, `docker ps --format json`, or `docker compose ps --format json`
- Optional health checks with `looplane ls --check` (2xx/3xx healthy, 4xx/5xx surfaced as errors)
- Remove routes with `looplane rm`
- Print stable route URLs with `looplane open NAME`
- Generate shell completions with `looplane completion [bash|zsh|fish|powershell]`
- Store-backed route-name completion for `looplane open` and `looplane rm`
- Optional shared route config via `--store PATH` across route and serve commands
- Start a local reverse proxy with `looplane serve`
- Optional host-based routing with `looplane serve --host-suffix localtest.me` for DNS-safe route names
- Forwarded host/proto/prefix headers so upstream apps see stable canonical URLs behind looplane
- Optional local HTTPS termination with `looplane serve --tls-cert ... --tls-key ...`
- Live-reload served routes when the selected store changes
- Path-prefix routing (`/api/...`, `/docs/...`)
- Upstream path preservation (`http://target/base` + `/docs/page` => `/base/page`)
- Helpful plaintext index page at `/`
- No external dependencies

## Install

```bash
go install github.com/agent19710101/looplane/cmd/looplane@latest
```

### Shell completions

```bash
looplane completion bash > ~/.local/share/bash-completion/completions/looplane
looplane completion zsh > "${fpath[1]}/_looplane"
looplane completion fish > ~/.config/fish/completions/looplane.fish
```

PowerShell:

```powershell
looplane completion powershell | Out-String | Invoke-Expression
```

Generated completions read route names directly from the active route store, so `open` and `rm` stay in sync with the current config without scraping `ls --json`. If you work with a shared file via `--store PATH`, completions follow that store too.

## Quickstart

```bash
looplane add api http://127.0.0.1:3000
looplane add docs http://127.0.0.1:4321/base
devport-radar --json > radar.json
looplane import devport-radar --file radar.json
docker ps --format json > docker.jsonl
looplane import docker-ps --file docker.jsonl
docker compose ps --format json > compose.json
looplane import docker-compose-ps --file compose.json
looplane ls --check
looplane ls --json
looplane open api
looplane open api --host-suffix localtest.me
looplane serve --addr 127.0.0.1:7777
looplane serve --addr 127.0.0.1:7777 --host-suffix localtest.me
```

While `looplane serve --watch` is running, later `add`, `rm`, and `import` changes are picked up on the next request, so you do not need to restart the proxy to refresh the route map.

### Docker and Compose import

If part of your local stack already runs in containers, `looplane` can import the published host ports from `docker ps --format json` output (JSON lines or a JSON array):

```bash
docker ps --format json > docker.jsonl
looplane import docker-ps --file docker.jsonl
looplane ls
```

Imported Docker routes use the container name when available, fall back to the image name if needed, and map published ports to `http://127.0.0.1:PORT`. Containers without published host ports are skipped.

For Compose-backed stacks, point `looplane` at `docker compose ps --format json` output:

```bash
docker compose ps --format json > compose.json
looplane import docker-compose-ps --file compose.json
looplane ls
```

Compose imports use the Compose service name when available, fall back to the container name if needed, and keep the same deterministic conflict handling as the other import sources.

### Shared route config

Use `--store PATH` when a repo, devcontainer, or team workflow needs a shared route file instead of the default per-user store:

```bash
looplane add api http://127.0.0.1:3000 --store ./.looplane/routes.json
looplane import devport-radar --file radar.json --store ./.looplane/routes.json
looplane ls --store ./.looplane/routes.json
looplane open api --store ./.looplane/routes.json
looplane serve --store ./.looplane/routes.json --watch
```

This keeps the single-user default simple while making shared route maps explicit and portable. Shell completions for `looplane open` and `looplane rm` use the same shared store when `--store PATH` is present on the command line, and route-store updates are written atomically so an interrupted save does not clobber the last valid JSON file.

Then open:

```bash
curl http://127.0.0.1:7777/
curl http://127.0.0.1:7777/api/healthz
curl http://127.0.0.1:7777/docs/
```

### Host-based routing

If you prefer memorable per-service hostnames, start the proxy with a wildcard local domain such as `localtest.me`:

```bash
looplane serve --addr 127.0.0.1:7777 --host-suffix localtest.me
looplane open api --host-suffix localtest.me
# -> http://api.localtest.me:7777/
```

With host-based routing enabled, requests for `api.localtest.me:7777` go straight to the `api` route root, so you can use hostnames instead of `/api/...` path prefixes when that fits your workflow better.

Host-based routing intentionally uses a stricter route-name contract than path-based routing: the route name must already be DNS-safe (`a-z`, `0-9`, and `-`, with no leading/trailing `-`). Names with underscores still work for path-based URLs like `/api_v2/`, but `looplane open --host-suffix ...` and `looplane serve --host-suffix ...` now reject them instead of printing or serving misleading hostnames.

### Forwarded headers for canonical URLs

When `looplane` proxies a request, it now forwards the original routing context that many upstream web apps use for redirects and absolute URL generation:

- `X-Forwarded-Host`: the original `Host` header seen by `looplane`
- `X-Forwarded-Proto`: `http` or `https`, depending on how the client reached `looplane`
- `X-Forwarded-Prefix`: the stable path prefix (for example `/api`) in path-based mode

That means apps behind `/api/`, host-based routing, or local TLS termination can generate canonical links, callback URLs, and redirects that match the URL the user actually visited instead of the raw upstream target.

### Local HTTPS for host-based routing

For browser flows that behave differently without TLS, `looplane` can terminate HTTPS locally when you already have a development certificate and key:

```bash
looplane serve \
  --addr 127.0.0.1:7777 \
  --host-suffix localtest.me \
  --tls-cert ./certs/localtest-me.pem \
  --tls-key ./certs/localtest-me-key.pem

looplane open api --host-suffix localtest.me --https
# -> https://api.localtest.me:7777/
```

A simple path is to generate a wildcard local certificate with `mkcert`, for example `*.localtest.me`, then point `looplane serve` at the resulting cert and key files. `looplane` intentionally stays small here: it uses the certs you provide instead of becoming a full certificate manager.

## Example output

```text
$ looplane ls --check
NAME  TARGET                         STATUS
api   http://127.0.0.1:3000          ok (200)
docs  http://127.0.0.1:4321/base     error (404)

$ looplane ls --json
[
  {
    "name": "api",
    "url": "http://127.0.0.1:3000"
  },
  {
    "name": "docs",
    "url": "http://127.0.0.1:4321/base"
  }
]

$ looplane ls --json --check
[
  {
    "name": "api",
    "url": "http://127.0.0.1:3000",
    "ok": true,
    "status_code": 200,
    "message": "ok (200)"
  }
]

$ looplane open api
http://127.0.0.1:7777/api/
```

## Status

Early, usable v0.x project. Core route persistence and stable local proxying work today. Health checks, JSON route listing, stable URL printing, `devport-radar`, Docker `docker ps --format json`, and Docker Compose `docker compose ps --format json` snapshot import, generated shell completions, optional shared stores, host-based routing via `--host-suffix` (for DNS-safe route names), forwarded `X-Forwarded-Host`/`X-Forwarded-Proto`/`X-Forwarded-Prefix` headers for upstream canonical URL correctness, optional local TLS termination via `--tls-cert`/`--tls-key`, watch-mode route reloads for a running proxy, and atomic route-store writes are already in place. Route-name completion for `open` and `rm` is store-backed, including shared `--store PATH` workflows, so the interactive UX follows the selected config directly. `looplane ls --json --check` emits a flat lowercase schema for automation consumers. GitHub Actions runs formatting checks, `go vet`, and `go test ./...` on pushes, pull requests, tags, and published releases.

## Roadmap

- #13: run CI tests in a cross-platform matrix before release
- #10: add a minimal terminal dashboard for route health and quick actions
- consider a Kubernetes-friendly import path after the current proxy/correctness issues are closed
- consider lightweight dev-cert generation helpers on top of the existing `--tls-cert` / `--tls-key` flow

## Minimal release plan

### v0.10.0 — local HTTPS helpers

- `looplane serve` accepts `--tls-cert` and `--tls-key` for optional local HTTPS termination
- `looplane open --https --host-suffix ...` prints stable HTTPS route URLs for TLS-enabled local proxy setups
- documented an `mkcert`-friendly workflow instead of adding heavy built-in cert management
- added regression coverage for HTTPS URL printing, TLS index output, completion flags, and invalid half-config rejection

### v0.12.0 — forwarded canonical URL headers

- `looplane serve` now forwards `X-Forwarded-Host` and `X-Forwarded-Proto` so upstream apps can generate correct absolute URLs, redirects, and callback targets
- path-based proxying keeps forwarding `X-Forwarded-Prefix`, while host-based and TLS-backed flows now get the matching host/proto context too
- added regression coverage for path-based HTTP and host-based HTTPS forwarded-header behavior

### v0.11.1 — host-routing contract cleanup

- reject non-DNS-safe route names when `--host-suffix` is used, instead of printing hostnames that browsers and certs cannot rely on
- keep underscore-containing names available for path-based routing and imported route sets
- document the host-routing constraint and cover it with regression tests

### v0.12.0 — forwarded-header correctness

- land issue #12 so upstream apps receive `X-Forwarded-Host` and `X-Forwarded-Proto` alongside the existing prefix behavior
- add regression coverage for path-based, host-based, HTTP, and TLS-enabled proxy modes

### v0.13.x — release confidence + operator UX

- land issue #13 with a small cross-platform CI matrix before release
- follow with issue #10’s additive terminal dashboard for route health, stable URLs, and quick follow-up actions

## Development

Run these before pushing or cutting a release:

```bash
gofmt -w .
go vet ./...
go test ./...
```

## License

MIT
