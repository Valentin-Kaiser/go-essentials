package web

import (
	"net/http"
	"sort"
	"strings"
)

// Router is a custom HTTP router that supports middlewares and status callbacks
// It implements the http.Handler interface and allows for flexible request handling
type Router struct {
	mux         *http.ServeMux
	middlewares map[MiddlewareOrder][]Middleware
	sorted      [][]Middleware
	onStatus    map[string]map[int]func(http.ResponseWriter, *http.Request)
}

// NewRouter creates a new Router instance
// It initializes the ServeMux and the middlewares map
func NewRouter() *Router {
	r := &Router{
		mux:         http.NewServeMux(),
		middlewares: make(map[MiddlewareOrder][]Middleware),
	}

	return r
}

// ServeHTTP implements the http.Handler interface for the Router
// It wraps the request with middlewares and handles the response
func (router *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rw := newResponseWriter(w, r)
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
// This function will be called after the response is written if the status matches
// It allows you to handle specific status codes, such as logging or a custom response
func (router *Router) OnStatus(pattern string, status int, fn func(http.ResponseWriter, *http.Request)) {
	if router.onStatus == nil {
		router.onStatus = make(map[string]map[int]func(http.ResponseWriter, *http.Request))
	}
	if _, ok := router.onStatus[pattern]; !ok {
		router.onStatus[pattern] = make(map[int]func(http.ResponseWriter, *http.Request))
	}
	router.onStatus[pattern][status] = fn
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
	if matched := router.matchPattern(r.URL.Path); matched != "" {
		if fn, ok := router.onStatus[matched][rw.status]; ok {
			rw.clear()
			fn(rw, r)
		}
	}
}

func (router *Router) matchPattern(path string) (matched string) {
	for pattern := range router.onStatus {
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
