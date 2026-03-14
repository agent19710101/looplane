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
- Import routes from `devport-radar --json` or `docker ps --format json`
- Optional health checks with `looplane ls --check` (2xx/3xx healthy, 4xx/5xx surfaced as errors)
- Remove routes with `looplane rm`
- Print stable route URLs with `looplane open NAME`
- Generate shell completions with `looplane completion [bash|zsh|fish|powershell]`
- Store-backed route-name completion for `looplane open` and `looplane rm`
- Optional shared route config via `--store PATH` across route and serve commands
- Start a local reverse proxy with `looplane serve`
- Optional host-based routing with `looplane serve --host-suffix localtest.me`
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

Generated completions read route names directly from the active route store, so `open` and `rm` stay in sync with the current config without scraping `ls --json`. If you work with a shared file via `--store PATH`, completions now follow that store too.

## Quickstart

```bash
looplane add api http://127.0.0.1:3000
looplane add docs http://127.0.0.1:4321/base
devport-radar --json > radar.json
looplane import devport-radar --file radar.json
docker ps --format json > docker.jsonl
looplane import docker-ps --file docker.jsonl
looplane ls --check
looplane ls --json
looplane open api
looplane open api --host-suffix localtest.me
looplane serve --addr 127.0.0.1:7777
looplane serve --addr 127.0.0.1:7777 --host-suffix localtest.me
```

While `looplane serve --watch` is running, later `add`, `rm`, and `import` changes are picked up on the next request, so you do not need to restart the proxy to refresh the route map.

### Docker import

If part of your local stack already runs in containers, `looplane` can import the published host ports from `docker ps --format json` output (JSON lines or a JSON array):

```bash
docker ps --format json > docker.jsonl
looplane import docker-ps --file docker.jsonl
looplane ls
```

Imported Docker routes use the container name when available, fall back to the image name if needed, and map published ports to `http://127.0.0.1:PORT`. Containers without published host ports are skipped.

### Shared route config

Use `--store PATH` when a repo, devcontainer, or team workflow needs a shared route file instead of the default per-user store:

```bash
looplane add api http://127.0.0.1:3000 --store ./.looplane/routes.json
looplane import devport-radar --file radar.json --store ./.looplane/routes.json
looplane ls --store ./.looplane/routes.json
looplane open api --store ./.looplane/routes.json
looplane serve --store ./.looplane/routes.json --watch
```

This keeps the single-user default simple while making shared route maps explicit and portable. Shell completions for `looplane open` and `looplane rm` now use the same shared store when `--store PATH` is present on the command line, and route-store updates are written atomically so an interrupted save does not clobber the last valid JSON file.

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

Early, usable v0.x project. Core route persistence and stable local proxying work today. Health checks, JSON route listing, stable URL printing, `devport-radar` and Docker `docker ps --format json` snapshot import, generated shell completions, optional shared stores, host-based routing via `--host-suffix`, watch-mode route reloads for a running proxy, and atomic route-store writes are already in place. Route-name completion for `open` and `rm` is store-backed, including shared `--store PATH` workflows, so the interactive UX follows the selected config directly. `looplane ls --json --check` now emits a flat lowercase schema for automation consumers. GitHub Actions runs formatting checks, `go vet`, and `go test ./...` on pushes, pull requests, tags, and published releases.

## Roadmap

- #9: import from additional local stack scanners, starting with Compose/Kubernetes-friendly sources
- #10: add a minimal terminal dashboard for route health and quick actions
- #11: evaluate optional HTTPS/dev-cert helpers for host-based local routing

## Minimal release plan

### v0.9.0 — Docker import

- `looplane import docker-ps` now ingests `docker ps --format json` output in either JSON-lines or JSON-array form
- imported Docker routes use container names when available, fall back to image names, and map published host ports to `http://127.0.0.1:PORT`
- containers without published host ports are skipped instead of creating broken routes
- added regression coverage for Docker import parsing, multiple published ports, CLI wiring, and completion entries

### v0.10.x — broader local-stack import coverage

- land issue #9 with at least one more high-value import source beyond `devport-radar` and Docker
- keep stdin/file ergonomics, deterministic naming, and conflict handling consistent across import paths
- update docs around "discover first, then pin stable names"

### v0.11.x — operator UX

- land issue #10 with a small TUI/dashboard for route health, stable URLs, and quick follow-up actions
- keep the dashboard additive rather than replacing the core CLI/scriptable flow

### v0.12.x — host-based routing polish

- resolve issue #11 with either a clear "not worth it" decision or a minimal optional HTTPS/dev-cert path
- document the production-like browser/auth workflows that benefit from host-based local HTTPS

## Development

Run these before pushing or cutting a release:

```bash
gofmt -w .
go vet ./...
go test ./...
```

## License

MIT
