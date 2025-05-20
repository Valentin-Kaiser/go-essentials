package web

import (
	"net/http"
)

type Router struct {
	*http.ServeMux
	middlewares []func(http.Handler) http.Handler
}

func NewRouter() *Router {
	return &Router{
		ServeMux:    http.NewServeMux(),
		middlewares: []func(http.Handler) http.Handler{},
	}
}

func (router *Router) Use(middleware func(http.Handler) http.Handler) {
	router.middlewares = append(router.middlewares, middleware)
}

func (router *Router) Handle(pattern string, handler http.Handler) {
	for i := len(router.middlewares) - 1; i >= 0; i-- {
		handler = router.middlewares[i](handler)
	}
	router.ServeMux.Handle(pattern, handler)
}

func (router *Router) HandleFunc(pattern string, handlerFunc http.HandlerFunc) {
	router.Handle(pattern, logRequestFunc(handlerFunc))
}
