package main

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/agent19710101/looplane/internal/app"
)

type dashboardOptions struct {
	Addr       string
	HostSuffix string
	HTTPS      bool
}

func printDashboard(w io.Writer, routes []app.Route, statuses []app.RouteStatus, opts dashboardOptions) {
	scheme := routeScheme("", "", opts.HTTPS)
	normalizedHostSuffix := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(opts.HostSuffix)), ".")
	fmt.Fprintln(w, "looplane dashboard")
	if len(routes) == 0 {
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "No routes configured yet.")
		fmt.Fprintln(w, "Quick start: looplane add api http://127.0.0.1:3000")
		return
	}

	byName := make(map[string]app.RouteStatus, len(statuses))
	for _, status := range statuses {
		byName[status.Name] = status
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "HEALTH\tNAME\tTARGET\tSTABLE URL")
	for _, route := range routes {
		status := byName[route.Name]
		marker := "OK"
		if !status.OK {
			marker = "!!"
		}
		stableURL := fmt.Sprintf("%s://%s/%s/", scheme, strings.TrimSuffix(opts.Addr, "/"), route.Name)
		if normalizedHostSuffix != "" {
			if hostURL, err := hostRouteURL(opts.Addr, route.Name, normalizedHostSuffix, scheme); err == nil {
				stableURL = hostURL
			} else {
				stableURL = fmt.Sprintf("%s (path fallback: %s://%s/%s/)", err.Error(), scheme, strings.TrimSuffix(opts.Addr, "/"), route.Name)
			}
		}
		fmt.Fprintf(tw, "%s %s\t%s\t%s\t%s\n", marker, status.Message, route.Name, route.URL, stableURL)
	}
	_ = tw.Flush()
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Quick actions:")
	fmt.Fprintf(w, "  Recheck:  looplane ls --check --timeout 2s%s\n", dashboardStoreSuffix(""))
	fmt.Fprintf(w, "  Open:     looplane open NAME --addr %s%s\n", opts.Addr, dashboardOpenSuffix(normalizedHostSuffix, opts.HTTPS))
	fmt.Fprintf(w, "  Import:   looplane import devport-radar --file radar.json%s\n", dashboardStoreSuffix(""))
	fmt.Fprintf(w, "  Import:   looplane import docker-ps --file docker.jsonl%s\n", dashboardStoreSuffix(""))
	fmt.Fprintf(w, "  Proxy:    looplane serve --addr %s%s\n", opts.Addr, dashboardServeSuffix(normalizedHostSuffix, opts.HTTPS))
}

func dashboardOpenSuffix(hostSuffix string, https bool) string {
	parts := []string{}
	if hostSuffix != "" {
		parts = append(parts, "--host-suffix "+hostSuffix)
	}
	if https {
		parts = append(parts, "--https")
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func dashboardServeSuffix(hostSuffix string, https bool) string {
	parts := []string{}
	if hostSuffix != "" {
		parts = append(parts, "--host-suffix "+hostSuffix)
	}
	if https {
		parts = append(parts, "--tls-cert ./cert.pem --tls-key ./key.pem")
	}
	if len(parts) == 0 {
		return ""
	}
	return " " + strings.Join(parts, " ")
}

func dashboardStoreSuffix(storePath string) string {
	if strings.TrimSpace(storePath) == "" {
		return ""
	}
	return " --store " + storePath
}
