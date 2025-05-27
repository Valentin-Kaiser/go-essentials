package web

import (
	"net/http"
	"sort"
)

// Router is a custom HTTP router that supports middlewares and status callbacks
// It implements the http.Handler interface and allows for flexible request handling
type Router struct {
	mux         *http.ServeMux
	middlewares map[int8][]Middleware
	// onStatus is a map of status codes to callback functions
	onStatus map[int]func(http.ResponseWriter, *http.Request)
}

// NewRouter creates a new Router instance
// It initializes the ServeMux and the middlewares map
func NewRouter() *Router {
	r := &Router{
		mux:         http.NewServeMux(),
		middlewares: make(map[int8][]Middleware),
	}

	return r
}

// ServeHTTP implements the http.Handler interface for the Router
// It wraps the request with middlewares and handles the response
func (router *Router) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	handler := router.wrap(router.mux)

	rw := newResponseWriter(w, r)
	handler.ServeHTTP(rw, r)

	if fn, ok := router.onStatus[rw.status]; ok {
		rw.Clear()
		fn(rw, rw.r)
	}
	rw.Flush()
}

// Use adds a middleware to the router
func (router *Router) Use(priority int8, middleware func(http.Handler) http.Handler) {
	if _, ok := router.middlewares[priority]; !ok {
		router.middlewares[priority] = make([]Middleware, 0)
	}
	router.middlewares[priority] = append(router.middlewares[priority], middleware)
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
func (router *Router) OnStatus(status int, fn func(http.ResponseWriter, *http.Request)) {
	if router.onStatus == nil {
		router.onStatus = make(map[int]func(http.ResponseWriter, *http.Request))
	}
	router.onStatus[status] = fn
}

func (router *Router) wrap(handler http.Handler) http.Handler {
	priorities := make([]int8, 0, len(router.middlewares))
	for priority := range router.middlewares {
		priorities = append(priorities, priority)
	}
	sort.Slice(priorities, func(i, j int) bool {
		return priorities[i] > priorities[j]
	})

	for _, priority := range priorities {
		mws := router.middlewares[priority]
		if priority == 0 {
			// Apply priority 0 middlewares in reverse order
			for i := len(mws) - 1; i >= 0; i-- {
				handler = mws[i](handler)
			}
			continue
		}
		for _, mw := range mws {
			handler = mw(handler)
		}
	}

	return handler
}
