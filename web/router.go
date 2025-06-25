package web

import (
	"net"
	"net/http"
	"sort"
	"strings"

	"github.com/Valentin-Kaiser/go-core/apperror"
	"github.com/rs/zerolog/log"
	"golang.org/x/time/rate"
)

// Router is a custom HTTP router that supports middlewares and status callbacks
// It implements the http.Handler interface and allows for flexible request handling
type Router struct {
	mux              *http.ServeMux
	canonicalDomain  string
	sorted           [][]Middleware
	middlewares      map[MiddlewareOrder][]Middleware
	onStatus         map[string]map[int]func(http.ResponseWriter, *http.Request)
	onStatusPatterns map[string]struct{}
	limits           map[string]*rate.Limiter
	limitedPatterns  map[string]struct{}
	whitelist        map[string]*net.IPNet
	blacklist        map[string]*net.IPNet
	honeypotCallback func(map[string]*net.IPNet)
}

// NewRouter creates a new Router instance
// It initializes the ServeMux and the middlewares map
func NewRouter() *Router {
	r := &Router{
		mux:              http.NewServeMux(),
		middlewares:      make(map[MiddlewareOrder][]Middleware),
		onStatus:         make(map[string]map[int]func(http.ResponseWriter, *http.Request)),
		onStatusPatterns: make(map[string]struct{}),
		limits:           make(map[string]*rate.Limiter),
		limitedPatterns:  make(map[string]struct{}),
		whitelist:        make(map[string]*net.IPNet),
		blacklist:        make(map[string]*net.IPNet),
	}

	return r
}

// ServeHTTP implements the http.Handler interface for the Router
// It wraps the request with middlewares and handles the response
func (router *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rw := newResponseWriter(w, r)
	router.block(rw, r)
	router.canonicalRedirect(rw, r)
	router.rateLimit(rw, r)
	router.wrap(router.mux).ServeHTTP(rw, r)
	router.handleStatusHooks(rw, r)
	rw.flush()
}

// Use adds a middleware to the router
// It allows you to specify the order of execution using MiddlewareOrder
// All middlewares of the same order will be executed in the order they were added
func (router *Router) Use(order MiddlewareOrder, middleware func(http.Handler) http.Handler) {
	if _, ok := router.middlewares[order]; !ok {
		router.middlewares[order] = make([]Middleware, 0)
	}
	router.middlewares[order] = append(router.middlewares[order], middleware)
	router.sort()
}

// Handle registers a handler for the given pattern
func (router *Router) Handle(pattern string, handler http.Handler) {
	router.mux.Handle(pattern, handler)
}

// HandleFunc registers a handler function for the given pattern
func (router *Router) HandleFunc(pattern string, handlerFunc http.HandlerFunc) {
	router.Handle(pattern, handlerFunc)
}

// OnStatus registers a callback function for a specific HTTP status code
// This function will be called after the response is written if the status and pattern match
// It allows you to handle specific status codes, such as logging or a custom response
func (router *Router) OnStatus(pattern string, status int, fn func(http.ResponseWriter, *http.Request)) {
	if _, ok := router.onStatus[pattern]; !ok {
		router.onStatus[pattern] = make(map[int]func(http.ResponseWriter, *http.Request))
	}
	router.onStatus[pattern][status] = fn
	router.onStatusPatterns[pattern] = struct{}{}
}

// registerRateLimit applies a rate limit to the given pattern
// It uses the golang.org/x/time/rate package to limit the number of requests
func (router *Router) registerRateLimit(pattern string, limit rate.Limit, burst int) error {
	if limit <= 0 || burst <= 0 {
		return apperror.NewErrorf("invalid rate limit or burst value: limit=%v, burst=%d", limit, burst)
	}

	if _, exists := router.limits[pattern]; exists {
		return apperror.NewErrorf("pattern %s is already rate limited", pattern)
	}

	router.limits[pattern] = rate.NewLimiter(limit, burst)
	router.limitedPatterns[pattern] = struct{}{}
	return nil
}

// wrap applies all registered middlewares to the given handler
// It sorts the middlewares by their order and applies them LIFO (last in, first out)
func (router *Router) wrap(handler http.Handler) http.Handler {
	for _, middlewares := range router.sorted {
		for i := len(middlewares) - 1; i >= 0; i-- {
			handler = middlewares[i](handler)
		}
	}

	return handler
}

func (router *Router) sort() {
	router.sorted = make([][]Middleware, 0, len(router.middlewares))
	orders := make([]MiddlewareOrder, 0, len(router.middlewares))
	for order := range router.middlewares {
		orders = append(orders, order)
	}
	sort.Slice(orders, func(i, j int) bool {
		return orders[i] > orders[j]
	})

	for _, order := range orders {
		router.sorted = append(router.sorted, router.middlewares[order])
	}
}

func (router *Router) handleStatusHooks(rw *ResponseWriter, r *http.Request) {
	if matched := router.matchPattern(r.URL.Path, router.onStatusPatterns); matched != "" {
		if fn, ok := router.onStatus[matched][rw.status]; ok {
			rw.clear()
			fn(rw, r)
		}
	}
}

func (router *Router) rateLimit(w http.ResponseWriter, r *http.Request) {
	matched := router.matchPattern(r.URL.Path, router.limitedPatterns)
	if matched != "" {
		if !router.limits[matched].Allow() {
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}
	}
}

func (router *Router) canonicalRedirect(w http.ResponseWriter, r *http.Request) {
	if router.canonicalDomain == "" {
		return
	}

	addr := strings.Split(r.Host, ":")

	port := ""
	domain := addr[0]
	if len(addr) > 1 {
		port = ":" + addr[1]
	}

	if domain != router.canonicalDomain {
		protocol := "http"
		if r.TLS != nil {
			protocol = "https"
		}

		log.Trace().Str("host", r.Host).Str("domain", router.canonicalDomain).Str("port", port).Str("protocol", protocol).Msg("redirecting to canonical domain")
		http.Redirect(w, r, protocol+"://"+router.canonicalDomain+port+r.RequestURI, http.StatusMovedPermanently)
		return
	}
}

func (router *Router) honeypot(w http.ResponseWriter, r *http.Request) {
	ipStr := router.clientIP(r)

	ip := net.ParseIP(ipStr)
	if ip == nil {
		log.Warn().Str("ip", ipStr).Msg("honeypot accessed with invalid IP address")
		http.Error(w, "Invalid IP address", http.StatusBadRequest)
		return
	}

	if !router.ipInList(ip, router.whitelist) {
		cidr := ip.String() + "/32"
		if ip.To4() == nil {
			cidr = ip.String() + "/128" // Use /128 for IPv6 addresses
		}

		_, network, err := net.ParseCIDR(cidr)
		if err != nil {
			log.Error().Err(err).Str("ip", cidr).Msg("failed to parse IP address for honeypot")
			return
		}

		log.Debug().Str("ip", network.String()).Msg("honeypot triggered, blocking IP address")
		router.blacklist[network.String()] = network
		if router.honeypotCallback != nil {
			router.honeypotCallback(router.blacklist)
		}
	}
}

// block blocks all requests to the router coming from a IP address defined in the blacklist
func (router *Router) block(w http.ResponseWriter, r *http.Request) {
	ipStr := router.clientIP(r)
	ip := net.ParseIP(ipStr)
	if ip == nil {
		log.Warn().Str("ip", ipStr).Msg("blocked request with invalid IP address")
		http.Error(w, "Invalid IP address", http.StatusBadRequest)
		return
	}

	if router.ipInList(ip, router.whitelist) {
		return // If the IP is whitelisted, do not block it
	}
	if router.ipInList(ip, router.blacklist) {
		log.Warn().Str("ip", ip.String()).Msg("blocked request from IP address")
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
}

func (router *Router) setWhitelist(entries []string) error {
	networks, err := router.parseIPList(entries)
	if err != nil {
		return apperror.Wrap(err)
	}
	router.whitelist = make(map[string]*net.IPNet, len(networks))
	for _, network := range networks {
		router.whitelist[network.String()] = network
	}
	return nil
}

func (router *Router) setBlacklist(entries []string) error {
	networks, err := router.parseIPList(entries)
	if err != nil {
		return apperror.Wrap(err)
	}
	router.blacklist = make(map[string]*net.IPNet, len(networks))
	for _, network := range networks {
		router.blacklist[network.String()] = network
	}
	return nil
}

func (router *Router) matchPattern(path string, patterns map[string]struct{}) (matched string) {
	for pattern := range patterns {
		if pattern == path ||
			(strings.HasSuffix(pattern, "/") && strings.HasPrefix(path, pattern)) ||
			pattern == "/" {
			if len(pattern) > len(matched) {
				matched = pattern
			}
		}
	}
	return
}

func (router *Router) clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		return strings.TrimSpace(parts[0])
	}

	if xRealIP := r.Header.Get("X-Real-IP"); xRealIP != "" {
		return strings.TrimSpace(xRealIP)
	}

	if r.RemoteAddr != "" {
		host, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			return r.RemoteAddr
		}
		return host
	}

	return ""
}

func (router *Router) ipInList(ip net.IP, list map[string]*net.IPNet) bool {
	for _, network := range list {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

func (router *Router) parseIPList(entries []string) ([]*net.IPNet, error) {
	var list []*net.IPNet
	for _, entry := range entries {
		if !strings.Contains(entry, "/") {
			if ip := net.ParseIP(entry); ip != nil {
				if ip.To4() != nil {
					entry += "/32" // Use /32 for IPv4 addresses
				} else {
					entry += "/128" // Use /128 for IPv6 addresses
				}
			}
		}
		_, network, err := net.ParseCIDR(entry)
		if err != nil {
			return nil, err
		}
		list = append(list, network)
	}
	return list, nil
}
