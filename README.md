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

## Why now

The current OSS wave is full of agent-native tooling, local orchestration, and better terminal UX. We already have great tools for:

- finding local services
- watching ports
- orchestrating agents

What is still oddly manual is the last mile: **giving those local services stable names that humans and agents can reuse in scripts, prompts, demos, and browser tabs**.

`looplane` focuses on that narrow pain point.

## Features in v0

- Add/update named routes with `looplane add`
- Persist routes in `~/.config/looplane/routes.json`
- List routes with `looplane ls`
- Remove routes with `looplane rm`
- Start a local reverse proxy with `looplane serve`
- Path-prefix routing (`/api/...`, `/docs/...`)
- Upstream path preservation (`http://target/base` + `/docs/page` => `/base/page`)
- Helpful plaintext index page at `/`
- No external dependencies

## Install

```bash
go install github.com/agent19710101/looplane/cmd/looplane@latest
```

## Quickstart

```bash
looplane add api http://127.0.0.1:3000
looplane add docs http://127.0.0.1:4321/base
looplane ls
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
$ looplane ls
NAME    TARGET
api     http://127.0.0.1:3000
docs    http://127.0.0.1:4321/base

$ looplane serve --addr 127.0.0.1:7777
looplane listening on http://127.0.0.1:7777
- http://127.0.0.1:7777/api/ -> http://127.0.0.1:3000
- http://127.0.0.1:7777/docs/ -> http://127.0.0.1:4321/base
```

## Roadmap

- health checks and route status in `looplane ls`
- import from local scanners like `devport-radar`
- optional file-watch mode for shared team route config
- shell completions
- TUI dashboard for route health + quick switching
- optional host-based routing (`api.localtest.me` style)

## License

MIT
