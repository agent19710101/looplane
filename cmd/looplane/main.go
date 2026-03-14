package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
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

	storePath, err := defaultStorePath()
	if err != nil {
		return err
	}
	store := app.NewStore(storePath)

	switch args[0] {
	case "add":
		if len(args) != 3 {
			return errors.New("usage: looplane add NAME URL")
		}
		routes, err := store.Load()
		if err != nil {
			return err
		}
		route, err := app.ValidateRoute(args[1], args[2])
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
		if len(args) != 2 {
			return errors.New("usage: looplane rm NAME")
		}
		routes, err := store.Load()
		if err != nil {
			return err
		}
		updated, removed := app.DeleteRoute(routes, args[1])
		if !removed {
			return fmt.Errorf("route %s not found", args[1])
		}
		if err := store.Save(updated); err != nil {
			return err
		}
		fmt.Printf("removed route %s\n", args[1])
		return nil
	case "ls":
		fs := flag.NewFlagSet("ls", flag.ContinueOnError)
		check := fs.Bool("check", false, "probe upstream health for each route")
		jsonOut := fs.Bool("json", false, "emit routes as JSON for scripts and agents")
		timeout := fs.Duration("timeout", 2*time.Second, "health check timeout (used with --check)")
		if err := fs.Parse(args[1:]); err != nil {
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
				fmt.Fprintf(w, "%s\t%s\t%s\n", status.Route.Name, status.Route.URL, status.Message)
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
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		routes, err := store.Load()
		if err != nil {
			return err
		}
		srv := &app.Server{Addr: *addr, Routes: routes, Stdout: os.Stdout}
		fmt.Printf("looplane listening on http://%s\n", *addr)
		if len(routes) == 0 {
			fmt.Println("tip: add routes with `looplane add NAME http://127.0.0.1:PORT`")
		} else {
			for _, route := range routes {
				fmt.Printf("- http://%s/%s/ -> %s\n", *addr, route.Name, route.URL)
			}
		}
		return http.ListenAndServe(*addr, srv.Handler())
	case "open":
		openArgs := args[1:]
		routeName := ""
		if len(openArgs) > 0 && !strings.HasPrefix(openArgs[0], "-") {
			routeName = strings.Trim(openArgs[0], "/")
			openArgs = openArgs[1:]
		}
		fs := flag.NewFlagSet("open", flag.ContinueOnError)
		addr := fs.String("addr", "127.0.0.1:7777", "looplane proxy address")
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
		fmt.Printf("http://%s/%s/\n", strings.TrimSuffix(*addr, "/"), routeName)
		return nil
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func defaultStorePath() (string, error) {
	return app.DefaultStorePath()
}

func printUsage() {
	fmt.Print(`looplane keeps stable names for flaky local dev ports.

Usage:
  looplane add NAME URL                        Add or update a named upstream route
  looplane rm NAME                             Remove a route
  looplane ls [--check] [--json] [--timeout D] List routes (optionally probe health)
  looplane serve [--addr A]                    Start reverse proxy (default 127.0.0.1:7777)
  looplane open NAME [--addr A]                Print the stable URL for a configured route

Examples:
  looplane add api http://127.0.0.1:3000
  looplane add docs http://127.0.0.1:4321/base
  looplane ls --check
  looplane ls --json
  looplane open api
  looplane serve --addr 127.0.0.1:7777
  curl http://127.0.0.1:7777/api/healthz
`)
}
