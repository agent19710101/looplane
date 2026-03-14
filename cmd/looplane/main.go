package main

import (
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

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

	storePath, err := app.DefaultStorePath()
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
		routes, err := store.Load()
		if err != nil {
			return err
		}
		if len(routes) == 0 {
			fmt.Println("no routes configured")
			return nil
		}
		fmt.Println("NAME\tTARGET")
		for _, route := range routes {
			fmt.Printf("%s\t%s\n", route.Name, route.URL)
		}
		return nil
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
		if len(args) != 3 {
			return errors.New("usage: looplane open ADDR NAME")
		}
		fmt.Printf("http://%s/%s/\n", strings.TrimSuffix(args[1], "/"), strings.Trim(args[2], "/"))
		return nil
	case "help", "-h", "--help":
		printUsage()
		return nil
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

func printUsage() {
	fmt.Print(`looplane keeps stable names for flaky local dev ports.

Usage:
  looplane add NAME URL      Add or update a named upstream route
  looplane rm NAME           Remove a route
  looplane ls                List routes
  looplane serve [--addr A]  Start reverse proxy (default 127.0.0.1:7777)
  looplane open ADDR NAME    Print the stable URL for a route

Examples:
  looplane add api http://127.0.0.1:3000
  looplane add docs http://127.0.0.1:4321/base
  looplane serve --addr 127.0.0.1:7777
  curl http://127.0.0.1:7777/api/healthz
`)
}
