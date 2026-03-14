package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type Route struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type Store struct {
	path string
	mu   sync.Mutex
}

type routeFile struct {
	Routes []Route `json:"routes"`
}

func DefaultStorePath() (string, error) {
	cfg, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolve config dir: %w", err)
	}
	return filepath.Join(cfg, "looplane", "routes.json"), nil
}

func NewStore(path string) *Store { return &Store{path: path} }

func (s *Store) Load() ([]Route, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *Store) loadLocked() ([]Route, error) {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []Route{}, nil
		}
		return nil, fmt.Errorf("read routes: %w", err)
	}
	var rf routeFile
	if err := json.Unmarshal(data, &rf); err != nil {
		return nil, fmt.Errorf("parse routes: %w", err)
	}
	sort.Slice(rf.Routes, func(i, j int) bool { return rf.Routes[i].Name < rf.Routes[j].Name })
	return rf.Routes, nil
}

func (s *Store) Save(routes []Route) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}
	sort.Slice(routes, func(i, j int) bool { return routes[i].Name < routes[j].Name })
	payload, err := json.MarshalIndent(routeFile{Routes: routes}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode routes: %w", err)
	}
	if err := os.WriteFile(s.path, append(payload, '\n'), 0o644); err != nil {
		return fmt.Errorf("write routes: %w", err)
	}
	return nil
}

func ValidateRoute(name string, rawURL string) (Route, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Route{}, errors.New("route name is required")
	}
	for _, r := range name {
		ok := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_'
		if !ok {
			return Route{}, fmt.Errorf("invalid route name %q: use lowercase letters, numbers, - or _", name)
		}
	}
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return Route{}, fmt.Errorf("parse target URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return Route{}, fmt.Errorf("unsupported scheme %q: use http or https", u.Scheme)
	}
	if u.Host == "" {
		return Route{}, errors.New("target URL must include a host")
	}
	return Route{Name: name, URL: u.String()}, nil
}

func UpsertRoute(routes []Route, route Route) []Route {
	for i := range routes {
		if routes[i].Name == route.Name {
			routes[i] = route
			return routes
		}
	}
	return append(routes, route)
}

func DeleteRoute(routes []Route, name string) ([]Route, bool) {
	out := make([]Route, 0, len(routes))
	removed := false
	for _, route := range routes {
		if route.Name == name {
			removed = true
			continue
		}
		out = append(out, route)
	}
	return out, removed
}

func FindRoute(routes []Route, name string) (Route, bool) {
	for _, route := range routes {
		if route.Name == name {
			return route, true
		}
	}
	return Route{}, false
}

type Server struct {
	Addr   string
	Routes []Route
	Stdout io.Writer
}

func (s *Server) Handler() http.Handler {
	byName := map[string]Route{}
	for _, route := range s.Routes {
		byName[route.Name] = route
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			writeIndex(w, s.Addr, s.Routes)
			return
		}
		parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		if len(parts) == 0 || parts[0] == "" {
			http.NotFound(w, r)
			return
		}
		route, ok := byName[parts[0]]
		if !ok {
			http.NotFound(w, r)
			return
		}
		target, err := url.Parse(route.URL)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid target for %s: %v", route.Name, err), http.StatusInternalServerError)
			return
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)
			suffix := "/"
			if len(parts) > 1 {
				suffix += strings.Join(parts[1:], "/")
			}
			req.URL.Path = joinURLPath(target.Path, suffix)
			req.URL.RawPath = req.URL.EscapedPath()
			req.Host = target.Host
			if r.URL.RawQuery != "" {
				req.URL.RawQuery = r.URL.RawQuery
			}
			req.Header.Set("X-Forwarded-Prefix", "/"+route.Name)
			req.Header.Set("X-Looplane-Route", route.Name)
		}
		proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
			http.Error(w, fmt.Sprintf("proxy %s failed: %v", route.Name, err), http.StatusBadGateway)
		}
		proxy.ServeHTTP(w, r)
	})
	return mux
}

func writeIndex(w http.ResponseWriter, addr string, routes []Route) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "looplane proxy on %s\n\n", addr)
	if len(routes) == 0 {
		_, _ = io.WriteString(w, "No routes yet. Add one with: looplane add NAME http://127.0.0.1:PORT\n")
		return
	}
	_, _ = io.WriteString(w, "Routes:\n")
	for _, route := range routes {
		_, _ = fmt.Fprintf(w, "- /%s/ -> %s\n", route.Name, route.URL)
	}
}

func joinURLPath(base string, extra string) string {
	base = strings.TrimSuffix(base, "/")
	extra = "/" + strings.TrimPrefix(extra, "/")
	if base == "" {
		return extra
	}
	return base + extra
}
