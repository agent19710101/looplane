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
- Import routes from `devport-radar --json`
- Optional health checks with `looplane ls --check`
- Remove routes with `looplane rm`
- Print stable route URLs with `looplane open NAME`
- Generate shell completions with `looplane completion [bash|zsh|fish|powershell]`
- Store-backed route-name completion for `looplane open` and `looplane rm`
- Start a local reverse proxy with `looplane serve`
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

Generated completions read route names directly from `~/.config/looplane/routes.json`, so `open` and `rm` stay in sync with the current store without scraping `ls --json`.

## Quickstart

```bash
looplane add api http://127.0.0.1:3000
looplane add docs http://127.0.0.1:4321/base
devport-radar --json > radar.json
looplane import devport-radar --file radar.json
looplane ls --check
looplane ls --json
looplane open api
looplane serve --addr 127.0.0.1:7777
```

Then open:

```bash
curl http://127.0.0.1:7777/
curl http://127.0.0.1:7777/api/healthz
curl http://127.0.0.1:7777/docs/
```

## Example output

```text
$ looplane ls --check
NAME  TARGET                         STATUS
api   http://127.0.0.1:3000          ok (200)
docs  http://127.0.0.1:4321/base     ok (200)

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

$ looplane open api
http://127.0.0.1:7777/api/
```

## Status

Early, usable v0.x project. Core route persistence and stable local proxying work today. Health checks, JSON route listing, stable URL printing, `devport-radar` snapshot import, and generated shell completions are already in place. Route-name completion for `open` and `rm` is now store-backed, so the interactive UX follows the saved config directly.

## Roadmap

- optional file-watch mode for shared team route config
- import from additional local scanners beyond `devport-radar`
- TUI dashboard for route health + quick switching
- optional host-based routing (`api.localtest.me` style)

## Development

```bash
go test ./...
```

## License

MIT
