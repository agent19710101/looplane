package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var newHTTPClient = func(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout}
}

var (
	osCreateTemp = os.CreateTemp
	osRename     = os.Rename
)

type Route struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type RouteStatus struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	OK         bool   `json:"ok"`
	StatusCode int    `json:"status_code"`
	Message    string `json:"message"`
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
	if err := writeFileAtomic(s.path, append(payload, '\n'), 0o644); err != nil {
		return fmt.Errorf("write routes: %w", err)
	}
	return nil
}

func writeFileAtomic(path string, data []byte, mode os.FileMode) (err error) {
	dir := filepath.Dir(path)
	file, err := osCreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmpPath := file.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpPath)
			_ = file.Close()
		}
	}()

	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Chmod(mode); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return osRename(tmpPath, path)
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

func RouteNames(routes []Route, prefix string) []string {
	names := make([]string, 0, len(routes))
	for _, route := range routes {
		if prefix != "" && !strings.HasPrefix(route.Name, prefix) {
			continue
		}
		names = append(names, route.Name)
	}
	return names
}

func CheckRoutes(routes []Route, timeout time.Duration) []RouteStatus {
	client := newHTTPClient(timeout)
	statuses := make([]RouteStatus, 0, len(routes))
	for _, route := range routes {
		statuses = append(statuses, checkRoute(client, route))
	}
	return statuses
}

func checkRoute(client *http.Client, route Route) RouteStatus {
	status := RouteStatus{Name: route.Name, URL: route.URL}
	statusCode, err := probeRoute(client, http.MethodHead, route.URL)
	if err == nil {
		status.OK = true
		status.StatusCode = statusCode
		status.Message = fmt.Sprintf("ok (%d)", statusCode)
		return status
	}
	if statusCode == http.StatusMethodNotAllowed {
		statusCode, err = probeRoute(client, http.MethodGet, route.URL)
		if err == nil {
			status.OK = true
			status.StatusCode = statusCode
			status.Message = fmt.Sprintf("ok (%d)", statusCode)
			return status
		}
	}
	if statusCode != 0 {
		status.StatusCode = statusCode
		status.Message = fmt.Sprintf("error (%d)", statusCode)
	} else {
		status.Message = fmt.Sprintf("down (%v)", err)
	}
	return status
}

func probeRoute(client *http.Client, method string, rawURL string) (int, error) {
	req, err := http.NewRequest(method, rawURL, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return resp.StatusCode, nil
	}
	if resp.StatusCode == http.StatusMethodNotAllowed {
		return resp.StatusCode, fmt.Errorf("upstream returned %s", resp.Status)
	}
	if resp.StatusCode >= 400 {
		return resp.StatusCode, fmt.Errorf("upstream returned %s", resp.Status)
	}
	return resp.StatusCode, nil
}

type Server struct {
	Addr       string
	HostSuffix string
	TLSCert    string
	TLSKey     string
	Routes     []Route
	LoadRoutes func() ([]Route, error)
	Stdout     io.Writer
	Transport  http.RoundTripper
}

func (s *Server) currentRoutes() ([]Route, error) {
	if s.LoadRoutes != nil {
		return s.LoadRoutes()
	}
	return s.Routes, nil
}

func routesByName(routes []Route) map[string]Route {
	byName := make(map[string]Route, len(routes))
	for _, route := range routes {
		byName[route.Name] = route
	}
	return byName
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		routes, err := s.currentRoutes()
		if err != nil {
			http.Error(w, fmt.Sprintf("load routes: %v", err), http.StatusInternalServerError)
			return
		}

		route, routePath, prefix, ok := s.resolveRoute(routes, r)
		if !ok {
			if r.URL.Path == "/" {
				writeIndex(w, s.Addr, s.HostSuffix, s.TLSCert, s.TLSKey, routes)
				return
			}
			http.NotFound(w, r)
			return
		}

		target, err := url.Parse(route.URL)
		if err != nil {
			http.Error(w, fmt.Sprintf("invalid target for %s: %v", route.Name, err), http.StatusInternalServerError)
			return
		}
		proxy := httputil.NewSingleHostReverseProxy(target)
		if s.Transport != nil {
			proxy.Transport = s.Transport
		}
		originalDirector := proxy.Director
		proxy.Director = func(req *http.Request) {
			originalDirector(req)
			req.URL.Path = joinURLPath(target.Path, routePath)
			req.URL.RawPath = req.URL.EscapedPath()
			req.Host = target.Host
			if r.URL.RawQuery != "" {
				req.URL.RawQuery = r.URL.RawQuery
			}
			if prefix != "" {
				req.Header.Set("X-Forwarded-Prefix", prefix)
			}
			req.Header.Set("X-Looplane-Route", route.Name)
		}
		proxy.ErrorHandler = func(w http.ResponseWriter, req *http.Request, err error) {
			http.Error(w, fmt.Sprintf("proxy %s failed: %v", route.Name, err), http.StatusBadGateway)
		}
		proxy.ServeHTTP(w, r)
	})
	return mux
}

func (s *Server) resolveRoute(routes []Route, r *http.Request) (Route, string, string, bool) {
	if route, ok := s.resolveRouteByHost(routes, r.Host); ok {
		return route, routePathFromRequest(r.URL.Path), "", true
	}

	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return Route{}, "", "", false
	}
	route, ok := routesByName(routes)[parts[0]]
	if !ok {
		return Route{}, "", "", false
	}
	suffix := "/"
	if len(parts) > 1 {
		suffix += strings.Join(parts[1:], "/")
	}
	return route, suffix, "/" + route.Name, true
}

func (s *Server) resolveRouteByHost(routes []Route, host string) (Route, bool) {
	if s.HostSuffix == "" {
		return Route{}, false
	}
	host = stripPort(host)
	suffix := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(s.HostSuffix), "."))
	if host == suffix || !strings.HasSuffix(host, "."+suffix) {
		return Route{}, false
	}
	name := strings.TrimSuffix(host, "."+suffix)
	if strings.Contains(name, ".") || name == "" {
		return Route{}, false
	}
	return FindRoute(routes, name)
}

func writeIndex(w http.ResponseWriter, addr string, hostSuffix string, certPath string, keyPath string, routes []Route) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = fmt.Fprintf(w, "looplane proxy on %s\n\n", addr)
	if len(routes) == 0 {
		_, _ = io.WriteString(w, "No routes yet. Add one with: looplane add NAME http://127.0.0.1:PORT\n")
		return
	}
	scheme := serverScheme(certPath, keyPath)
	_, _ = io.WriteString(w, "Routes:\n")
	for _, route := range routes {
		_, _ = fmt.Fprintf(w, "- /%s/ -> %s\n", route.Name, route.URL)
		if hostSuffix != "" {
			_, _ = fmt.Fprintf(w, "- %s://%s.%s%s/ -> %s\n", scheme, route.Name, hostSuffix, addrPortSuffix(addr), route.URL)
		}
	}
}

func routePathFromRequest(path string) string {
	if path == "" || path == "/" {
		return "/"
	}
	return "/" + strings.TrimPrefix(path, "/")
}

func stripPort(hostport string) string {
	if strings.HasPrefix(hostport, "[") {
		if end := strings.Index(hostport, "]"); end != -1 {
			return strings.ToLower(hostport[1:end])
		}
	}
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return strings.ToLower(h)
	}
	return strings.ToLower(hostport)
}

func serverScheme(certPath string, keyPath string) string {
	if strings.TrimSpace(certPath) != "" && strings.TrimSpace(keyPath) != "" {
		return "https"
	}
	return "http"
}

func addrPortSuffix(addr string) string {
	if _, port, err := net.SplitHostPort(addr); err == nil {
		return ":" + port
	}
	return ""
}

func joinURLPath(base string, extra string) string {
	base = strings.TrimSuffix(base, "/")
	extra = "/" + strings.TrimPrefix(extra, "/")
	if base == "" {
		return extra
	}
	return base + extra
}
