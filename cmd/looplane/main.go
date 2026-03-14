package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/agent19710101/looplane/internal/app"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage()
		return nil
	}

	command, storePath, commandArgs, err := resolveCommandStore(args)
	if err != nil {
		return err
	}
	store := app.NewStore(storePath)

	switch command {
	case "add":
		if len(commandArgs) != 2 {
			return errors.New("usage: looplane add NAME URL [--store PATH]")
		}
		routes, err := store.Load()
		if err != nil {
			return err
		}
		route, err := app.ValidateRoute(commandArgs[0], commandArgs[1])
		if err != nil {
			return err
		}
		routes = app.UpsertRoute(routes, route)
		if err := store.Save(routes); err != nil {
			return err
		}
		fmt.Printf("saved route %s -> %s\n", route.Name, route.URL)
		return nil
	case "rm":
		if len(commandArgs) != 1 {
			return errors.New("usage: looplane rm NAME [--store PATH]")
		}
		routes, err := store.Load()
		if err != nil {
			return err
		}
		updated, removed := app.DeleteRoute(routes, commandArgs[0])
		if !removed {
			return fmt.Errorf("route %s not found", commandArgs[0])
		}
		if err := store.Save(updated); err != nil {
			return err
		}
		fmt.Printf("removed route %s\n", commandArgs[0])
		return nil
	case "import":
		if len(commandArgs) < 1 {
			return errors.New("usage: looplane import [devport-radar|docker-ps] [--file PATH] [--replace] [--store PATH]")
		}
		fs := flag.NewFlagSet("import", flag.ContinueOnError)
		file := fs.String("file", "", "path to import JSON input (default: stdin)")
		replace := fs.Bool("replace", false, "replace existing routes instead of merging")
		if err := fs.Parse(commandArgs[1:]); err != nil {
			return err
		}
		var input *os.File
		if *file == "" || *file == "-" {
			input = os.Stdin
		} else {
			f, err := os.Open(*file)
			if err != nil {
				return fmt.Errorf("open import file: %w", err)
			}
			defer f.Close()
			input = f
		}
		routes, err := store.Load()
		if err != nil {
			return err
		}
		var result app.ImportResult
		switch commandArgs[0] {
		case "devport-radar":
			result, err = app.ImportDevportRadarJSON(routes, input, app.ImportOptions{Replace: *replace})
		case "docker-ps":
			result, err = app.ImportDockerPSJSON(routes, input, app.ImportOptions{Replace: *replace})
		default:
			return errors.New("usage: looplane import [devport-radar|docker-ps] [--file PATH] [--replace] [--store PATH]")
		}
		if err != nil {
			return err
		}
		if err := store.Save(result.Routes); err != nil {
			return err
		}
		fmt.Printf("imported %s routes: added=%d updated=%d skipped=%d total=%d\n", commandArgs[0], result.Added, result.Updated, result.Skipped, len(result.Routes))
		return nil
	case "ls":
		fs := flag.NewFlagSet("ls", flag.ContinueOnError)
		check := fs.Bool("check", false, "probe upstream health for each route")
		jsonOut := fs.Bool("json", false, "emit routes as JSON for scripts and agents")
		timeout := fs.Duration("timeout", 2*time.Second, "health check timeout (used with --check)")
		if err := fs.Parse(commandArgs); err != nil {
			return err
		}
		routes, err := store.Load()
		if err != nil {
			return err
		}
		if *jsonOut {
			if *check {
				payload, err := json.MarshalIndent(app.CheckRoutes(routes, *timeout), "", "  ")
				if err != nil {
					return err
				}
				fmt.Println(string(payload))
				return nil
			}
			payload, err := json.MarshalIndent(routes, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(payload))
			return nil
		}
		if len(routes) == 0 {
			fmt.Println("no routes configured")
			return nil
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
		if *check {
			fmt.Fprintln(w, "NAME\tTARGET\tSTATUS")
			for _, status := range app.CheckRoutes(routes, *timeout) {
				fmt.Fprintf(w, "%s\t%s\t%s\n", status.Name, status.URL, status.Message)
			}
		} else {
			fmt.Fprintln(w, "NAME\tTARGET")
			for _, route := range routes {
				fmt.Fprintf(w, "%s\t%s\n", route.Name, route.URL)
			}
		}
		return w.Flush()
	case "serve":
		fs := flag.NewFlagSet("serve", flag.ContinueOnError)
		addr := fs.String("addr", "127.0.0.1:7777", "listen address")
		hostSuffix := fs.String("host-suffix", "", "optional host-based routing suffix (for example localtest.me)")
		tlsCert := fs.String("tls-cert", "", "optional TLS certificate path for local HTTPS")
		tlsKey := fs.String("tls-key", "", "optional TLS private key path for local HTTPS")
		watch := fs.Bool("watch", true, "reload routes from the selected store on each request")
		if err := fs.Parse(commandArgs); err != nil {
			return err
		}
		if (strings.TrimSpace(*tlsCert) == "") != (strings.TrimSpace(*tlsKey) == "") {
			return errors.New("serve requires both --tls-cert and --tls-key together")
		}
		routes, err := store.Load()
		if err != nil {
			return err
		}
		srv := &app.Server{
			Addr:       *addr,
			HostSuffix: strings.TrimPrefix(strings.ToLower(strings.TrimSpace(*hostSuffix)), "."),
			TLSCert:    strings.TrimSpace(*tlsCert),
			TLSKey:     strings.TrimSpace(*tlsKey),
			Routes:     routes,
			Stdout:     os.Stdout,
		}
		if *watch {
			srv.LoadRoutes = store.Load
		}
		scheme := routeScheme(*tlsCert, *tlsKey, false)
		fmt.Printf("looplane listening on %s://%s\n", scheme, *addr)
		if len(routes) == 0 {
			fmt.Println("tip: add routes with `looplane add NAME http://127.0.0.1:PORT`")
		} else {
			for _, route := range routes {
				fmt.Printf("- %s://%s/%s/ -> %s\n", scheme, *addr, route.Name, route.URL)
				if srv.HostSuffix != "" {
					if hostURL, err := hostRouteURL(*addr, route.Name, srv.HostSuffix, scheme); err == nil {
						fmt.Printf("- %s -> %s\n", hostURL, route.URL)
					}
				}
			}
		}
		if strings.TrimSpace(*tlsCert) != "" {
			return http.ListenAndServeTLS(*addr, *tlsCert, *tlsKey, srv.Handler())
		}
		return http.ListenAndServe(*addr, srv.Handler())
	case "open":
		openArgs := commandArgs
		routeName := ""
		if len(openArgs) > 0 && !strings.HasPrefix(openArgs[0], "-") {
			routeName = strings.Trim(openArgs[0], "/")
			openArgs = openArgs[1:]
		}
		fs := flag.NewFlagSet("open", flag.ContinueOnError)
		addr := fs.String("addr", "127.0.0.1:7777", "looplane proxy address")
		hostSuffix := fs.String("host-suffix", "", "optional host-based routing suffix (for example localtest.me)")
		https := fs.Bool("https", false, "print an HTTPS URL for TLS-enabled local proxy setups")
		if err := fs.Parse(openArgs); err != nil {
			return err
		}
		if routeName == "" {
			if fs.NArg() != 1 {
				return errors.New("usage: looplane open NAME [--addr 127.0.0.1:7777]")
			}
			routeName = strings.Trim(fs.Arg(0), "/")
		} else if fs.NArg() != 0 {
			return errors.New("usage: looplane open NAME [--addr 127.0.0.1:7777]")
		}
		routes, err := store.Load()
		if err != nil {
			return err
		}
		if _, ok := app.FindRoute(routes, routeName); !ok {
			return fmt.Errorf("route %s not found", routeName)
		}
		scheme := routeScheme("", "", *https)
		normalizedHostSuffix := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(*hostSuffix)), ".")
		if normalizedHostSuffix != "" {
			url, err := hostRouteURL(*addr, routeName, normalizedHostSuffix, scheme)
			if err != nil {
				return err
			}
			fmt.Println(url)
			return nil
		}
		fmt.Printf("%s://%s/%s/\n", scheme, strings.TrimSuffix(*addr, "/"), routeName)
		return nil
	case "completion":
		if len(commandArgs) != 1 {
			return errors.New("usage: looplane completion [bash|zsh|fish|powershell]")
		}
		script, err := completionScript(commandArgs[0])
		if err != nil {
			return err
		}
		fmt.Print(script)
		return nil
	case "__complete":
		return runCompletion(store, commandArgs)
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", command)
	}
}

func defaultStorePath() (string, error) {
	return app.DefaultStorePath()
}

func resolveCommandStore(args []string) (string, string, []string, error) {
	if len(args) == 0 {
		return "", "", nil, errors.New("missing command")
	}

	command := args[0]
	storePath, err := defaultStorePath()
	if err != nil {
		return "", "", nil, err
	}

	commandArgs := make([]string, 0, len(args)-1)
	for i := 1; i < len(args); i++ {
		if args[i] == "--store" {
			if i+1 >= len(args) {
				return "", "", nil, errors.New("--store requires a path")
			}
			storePath = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(args[i], "--store=") {
			storePath = strings.TrimPrefix(args[i], "--store=")
			continue
		}
		commandArgs = append(commandArgs, args[i])
	}

	if strings.TrimSpace(storePath) == "" {
		return "", "", nil, errors.New("--store requires a non-empty path")
	}
	return command, storePath, commandArgs, nil
}

func routeScheme(certPath string, keyPath string, https bool) string {
	if https || (strings.TrimSpace(certPath) != "" && strings.TrimSpace(keyPath) != "") {
		return "https"
	}
	return "http"
}

func hostRouteURL(addr string, routeName string, hostSuffix string, scheme string) (string, error) {
	hostSuffix = strings.TrimPrefix(strings.ToLower(strings.TrimSpace(hostSuffix)), ".")
	if hostSuffix == "" {
		return "", errors.New("host suffix is required")
	}
	host := strings.TrimSpace(addr)
	if strings.Contains(host, "://") {
		return "", errors.New("addr should be host:port, not a full URL")
	}
	if h, p, err := net.SplitHostPort(host); err == nil {
		if h == "" || h == "0.0.0.0" || h == "::" || h == "[::]" {
			h = "127.0.0.1"
		}
		if h == "127.0.0.1" || h == "localhost" {
			return fmt.Sprintf("%s://%s.%s:%s/", scheme, routeName, hostSuffix, p), nil
		}
		return fmt.Sprintf("%s://%s.%s:%s/", scheme, routeName, hostSuffix, p), nil
	}
	return fmt.Sprintf("%s://%s.%s/", scheme, routeName, hostSuffix), nil
}

func printUsage() {
	fmt.Print(`looplane keeps stable names for flaky local dev ports.

Usage:
  looplane add NAME URL [--store PATH]         Add or update a named upstream route
  looplane rm NAME [--store PATH]              Remove a route
  looplane import SOURCE [--file PATH] [--replace] [--store PATH]
                                              Import routes from devport-radar or docker ps JSON
  looplane ls [--check] [--json] [--timeout D] [--store PATH]
                                              List routes (optionally probe health)
  looplane serve [--addr A] [--host-suffix SUFFIX] [--tls-cert FILE --tls-key FILE] [--watch] [--store PATH]
                                              Start reverse proxy (default 127.0.0.1:7777)
  looplane open NAME [--addr A] [--host-suffix SUFFIX] [--https] [--store PATH]
                                              Print the stable URL for a configured route
  looplane completion SHELL                    Print a shell completion script

Examples:
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
  looplane open api --host-suffix localtest.me --https
  looplane serve --addr 127.0.0.1:7777
  looplane serve --addr 127.0.0.1:7777 --host-suffix localtest.me
  looplane serve --addr 127.0.0.1:7777 --host-suffix localtest.me --tls-cert ./certs/local.pem --tls-key ./certs/local-key.pem
  looplane ls --store ./looplane.routes.json
  looplane serve --store ./looplane.routes.json --watch
  looplane completion bash > ~/.local/share/bash-completion/completions/looplane
  curl http://127.0.0.1:7777/api/healthz
`)
}

func runCompletion(store *app.Store, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: looplane __complete routes [PREFIX] [--store PATH]")
	}
	switch args[0] {
	case "routes":
		completionArgs := args[1:]
		prefix := ""
		if len(completionArgs) > 0 && !strings.HasPrefix(completionArgs[0], "-") {
			prefix = completionArgs[0]
			completionArgs = completionArgs[1:]
		}
		if len(completionArgs) > 0 {
			storePath, err := completionStorePath(completionArgs)
			if err != nil {
				return err
			}
			if storePath != "" {
				store = app.NewStore(storePath)
			}
		}
		routes, err := store.Load()
		if err != nil {
			return err
		}
		for _, name := range app.RouteNames(routes, prefix) {
			fmt.Println(name)
		}
		return nil
	default:
		return fmt.Errorf("unknown completion target %q", args[0])
	}
}

func completionStorePath(args []string) (string, error) {
	storePath := ""
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--store":
			if i+1 >= len(args) {
				return "", errors.New("--store requires a path")
			}
			storePath = args[i+1]
			i++
		case strings.HasPrefix(args[i], "--store="):
			storePath = strings.TrimPrefix(args[i], "--store=")
		}
	}
	if strings.TrimSpace(storePath) == "" {
		return storePath, nil
	}
	return storePath, nil
}

func completionScript(shell string) (string, error) {
	switch shell {
	case "bash":
		return `# bash completion for looplane
_looplane_store_args() {
    local i
    for ((i=1; i<${#words[@]}; i++)); do
        case "${words[i]}" in
            --store)
                if (( i + 1 < ${#words[@]} )); then
                    printf '%s\n' "--store" "${words[i+1]}"
                fi
                return
                ;;
            --store=*)
                printf '%s\n' "${words[i]}"
                return
                ;;
        esac
    done
}

_looplane() {
    local cur prev words cword
    _init_completion || return

    local commands="add rm import ls serve open completion help"
    local -a store_args
    mapfile -t store_args < <(_looplane_store_args)

    case "${prev}" in
        import)
            COMPREPLY=( $(compgen -W "devport-radar docker-ps" -- "$cur") )
            return
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish powershell" -- "$cur") )
            return
            ;;
        open|rm)
            local routes
            routes=$(looplane __complete routes "$cur" "${store_args[@]}" 2>/dev/null)
            COMPREPLY=( $(compgen -W "$routes" -- "$cur") )
            return
            ;;
    esac

    if [[ "$cword" -eq 1 ]]; then
        COMPREPLY=( $(compgen -W "$commands" -- "$cur") )
        return
    fi

    case "${words[1]}" in
        ls)
            COMPREPLY=( $(compgen -W "--check --json --timeout --store" -- "$cur") )
            ;;
        serve)
            COMPREPLY=( $(compgen -W "--addr --host-suffix --tls-cert --tls-key --watch --store" -- "$cur") )
            ;;
        open)
            if [[ "$cur" == -* ]]; then
                COMPREPLY=( $(compgen -W "--addr --host-suffix --https --store" -- "$cur") )
                return
            fi
            local routes
            routes=$(looplane __complete routes "$cur" "${store_args[@]}" 2>/dev/null)
            COMPREPLY=( $(compgen -W "$routes" -- "$cur") )
            ;;
        import)
            COMPREPLY=( $(compgen -W "devport-radar docker-ps --file --replace --store" -- "$cur") )
            ;;
        completion)
            COMPREPLY=( $(compgen -W "bash zsh fish powershell" -- "$cur") )
            ;;
    esac
}

complete -F _looplane looplane
`, nil
	case "zsh":
		return `#compdef looplane

_looplane_store_args() {
  local -a out
  local i
  for ((i = 1; i <= CURRENT; i++)); do
    case ${words[i]} in
      --store)
        if (( i + 1 <= CURRENT )); then
          out+=(--store "${words[i+1]}")
        fi
        break
        ;;
      --store=*)
        out+=("${words[i]}")
        break
        ;;
    esac
  done
  reply=(${out[@]})
}

_looplane_routes() {
  local -a routes store_args
  _looplane_store_args
  store_args=(${reply[@]})
  routes=(${(f)"$(looplane __complete routes "${PREFIX:-}" ${store_args:+${store_args[@]}} 2>/dev/null)"})
  _describe 'route' routes
}

_looplane() {
  local context state line
  typeset -A opt_args

  _arguments -C \
    '1:command:((add:"Add or update a route" rm:"Remove a route" import:"Import routes" ls:"List routes" serve:"Start proxy" open:"Print stable URL" completion:"Print completions" help:"Show help"))' \
    '*::arg:->args'

  case $state in
    args)
      case $words[2] in
        rm)
          _looplane_routes
          ;;
        open)
          _looplane_routes
          _arguments '--addr[listen address]:address:' '--host-suffix[optional host-based routing suffix]:suffix:' '--https[print an HTTPS URL for TLS-enabled local proxy setups]' '--store[path to routes store]:file:_files'
          ;;
        import)
          _arguments '1:source:(devport-radar docker-ps)' '--file[path to import JSON]:file:_files' '--replace[replace existing routes instead of merging]' '--store[path to routes store]:file:_files'
          ;;
        ls)
          _arguments '--check[probe upstream health for each route]' '--json[emit routes as JSON]' '--timeout[health check timeout]:duration:' '--store[path to routes store]:file:_files'
          ;;
        serve)
          _arguments '--addr[listen address]:address:' '--host-suffix[optional host-based routing suffix]:suffix:' '--tls-cert[path to TLS certificate]:file:_files' '--tls-key[path to TLS private key]:file:_files' '--watch[reload routes from the selected store on each request]' '--store[path to routes store]:file:_files'
          ;;
        completion)
          _arguments '1:shell:(bash zsh fish powershell)'
          ;;
      esac
      ;;
  esac
}

_looplane "$@"
`, nil
	case "fish":
		return `function __looplane_store_args
    set -l tokens (commandline -opc)
    for i in (seq 1 (count $tokens))
        set -l token $tokens[$i]
        if test "$token" = "--store"
            set -l next_index (math $i + 1)
            if test $next_index -le (count $tokens)
                printf '%s\n' --store $tokens[$next_index]
            end
            return
        end
        if string match -q -- '--store=*' "$token"
            printf '%s\n' "$token"
            return
        end
    end
end

complete -c looplane -n '__fish_use_subcommand' -f -a 'add' -d 'Add or update a named upstream route'
complete -c looplane -n '__fish_use_subcommand' -f -a 'rm' -d 'Remove a route'
complete -c looplane -n '__fish_use_subcommand' -f -a 'import' -d 'Import routes from devport-radar JSON'
complete -c looplane -n '__fish_use_subcommand' -f -a 'ls' -d 'List routes'
complete -c looplane -n '__fish_use_subcommand' -f -a 'serve' -d 'Start reverse proxy'
complete -c looplane -n '__fish_use_subcommand' -f -a 'open' -d 'Print stable route URL'
complete -c looplane -n '__fish_use_subcommand' -f -a 'completion' -d 'Print shell completion script'
complete -c looplane -n '__fish_use_subcommand' -f -a 'help' -d 'Show help'

complete -c looplane -n '__fish_seen_subcommand_from import' -f -a 'devport-radar docker-ps'
complete -c looplane -n '__fish_seen_subcommand_from ls' -l check -d 'Probe upstream health for each route'
complete -c looplane -n '__fish_seen_subcommand_from ls' -l json -d 'Emit routes as JSON'
complete -c looplane -n '__fish_seen_subcommand_from ls' -l timeout -d 'Health check timeout' -r
complete -c looplane -n '__fish_seen_subcommand_from ls import serve open rm' -l store -d 'Path to routes store' -r
complete -c looplane -n '__fish_seen_subcommand_from serve open' -l addr -d 'Listen/proxy address' -r
complete -c looplane -n '__fish_seen_subcommand_from serve open' -l host-suffix -d 'Optional host-based routing suffix'
complete -c looplane -n '__fish_seen_subcommand_from serve' -l tls-cert -d 'Path to TLS certificate' -r
complete -c looplane -n '__fish_seen_subcommand_from serve' -l tls-key -d 'Path to TLS private key' -r
complete -c looplane -n '__fish_seen_subcommand_from open' -l https -d 'Print an HTTPS URL'
complete -c looplane -n '__fish_seen_subcommand_from serve' -l watch -d 'Reload routes from the selected store on each request'
complete -c looplane -n '__fish_seen_subcommand_from import' -l file -d 'Path to import JSON' -r
complete -c looplane -n '__fish_seen_subcommand_from import' -l replace -d 'Replace existing routes instead of merging'
complete -c looplane -n '__fish_seen_subcommand_from completion' -f -a 'bash zsh fish powershell'
complete -c looplane -n '__fish_seen_subcommand_from rm open' -f -a '(looplane __complete routes (commandline -ct) (__looplane_store_args) 2>/dev/null)'
`, nil
	case "powershell":
		return `Register-ArgumentCompleter -Native -CommandName looplane -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)

    $commands = 'add', 'rm', 'import', 'ls', 'serve', 'open', 'completion', 'help'
    $shells = 'bash', 'zsh', 'fish', 'powershell'
    $routeNames = @()

    $tokens = $commandAst.CommandElements | ForEach-Object { $_.Extent.Text }
    $storeArgs = @()
    for ($i = 0; $i -lt $tokens.Count; $i++) {
        if ($tokens[$i] -eq '--store' -and $i + 1 -lt $tokens.Count) {
            $storeArgs = @('--store', $tokens[$i + 1])
            break
        }
        if ($tokens[$i] -like '--store=*') {
            $storeArgs = @($tokens[$i])
            break
        }
    }

    try {
        $routes = looplane __complete routes $wordToComplete @storeArgs 2>$null
        if ($routes) {
            $routeNames = @($routes)
        }
    } catch {}

    if ($tokens.Count -le 1) {
        $commands | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
            [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
        }
        return
    }

    switch ($tokens[1]) {
        'import' {
            @('devport-radar', 'docker-ps', '--file', '--replace', '--store') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
            }
        }
        'ls' {
            @('--check', '--json', '--timeout', '--store') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
            }
        }
        'serve' {
            @('--addr', '--host-suffix', '--tls-cert', '--tls-key', '--watch', '--store') | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
            }
        }
        'open' {
            @('--addr', '--host-suffix', '--https', '--store') + $routeNames | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
            }
        }
        'rm' {
            @('--store') + $routeNames | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
            }
        }
        'completion' {
            $shells | Where-Object { $_ -like "$wordToComplete*" } | ForEach-Object {
                [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
            }
        }
    }
}
`, nil
	default:
		return "", fmt.Errorf("unsupported shell %q (use bash, zsh, fish, or powershell)", shell)
	}
}
