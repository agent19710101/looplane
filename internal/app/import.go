package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

type ImportOptions struct {
	Replace bool
}

type ImportResult struct {
	Added   int
	Updated int
	Skipped int
	Routes  []Route
}

type DevportRadarService struct {
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
	Process  string `json:"process"`
	Alias    string `json:"alias"`
}

func ImportDevportRadarJSON(existing []Route, r io.Reader, opts ImportOptions) (ImportResult, error) {
	var services []DevportRadarService
	if err := json.NewDecoder(r).Decode(&services); err != nil {
		return ImportResult{}, fmt.Errorf("decode devport-radar json: %w", err)
	}

	routesByName := make(map[string]Route, len(existing))
	for _, route := range existing {
		routesByName[route.Name] = route
	}
	if opts.Replace {
		routesByName = map[string]Route{}
	}

	result := ImportResult{}
	usedNames := make(map[string]struct{}, len(routesByName))
	for name := range routesByName {
		usedNames[name] = struct{}{}
	}

	for _, svc := range services {
		if svc.Port <= 0 {
			result.Skipped++
			continue
		}
		scheme := svc.Protocol
		if scheme == "" {
			scheme = "http"
		}
		if scheme != "http" && scheme != "https" {
			result.Skipped++
			continue
		}
		base := importRouteBaseName(svc)
		if base == "" {
			base = "port-" + strconv.Itoa(svc.Port)
		}
		name := uniqueImportRouteName(base, svc.Port, usedNames)
		route := Route{Name: name, URL: (&url.URL{Scheme: scheme, Host: "127.0.0.1:" + strconv.Itoa(svc.Port)}).String()}
		if prev, ok := routesByName[name]; ok {
			if prev.URL == route.URL {
				continue
			}
			result.Updated++
		} else {
			result.Added++
		}
		routesByName[name] = route
		usedNames[name] = struct{}{}
	}

	result.Routes = make([]Route, 0, len(routesByName))
	for _, route := range routesByName {
		result.Routes = append(result.Routes, route)
	}
	sort.Slice(result.Routes, func(i, j int) bool { return result.Routes[i].Name < result.Routes[j].Name })
	return result, nil
}

func importRouteBaseName(svc DevportRadarService) string {
	for _, candidate := range []string{svc.Alias, svc.Process} {
		name := sanitizeImportName(candidate)
		if name != "" {
			return name
		}
	}
	return ""
}

func sanitizeImportName(raw string) string {
	raw = strings.ToLower(strings.TrimSpace(raw))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	lastDash := false
	for _, r := range raw {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
		if ok {
			b.WriteRune(r)
			lastDash = false
			continue
		}
		if !lastDash {
			b.WriteByte('-')
			lastDash = true
		}
	}
	out := strings.Trim(b.String(), "-_")
	return out
}

func uniqueImportRouteName(base string, port int, used map[string]struct{}) string {
	if _, ok := used[base]; !ok {
		return base
	}
	withPort := fmt.Sprintf("%s-%d", base, port)
	if _, ok := used[withPort]; !ok {
		return withPort
	}
	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s-%d-%d", base, port, i)
		if _, ok := used[candidate]; !ok {
			return candidate
		}
	}
}
