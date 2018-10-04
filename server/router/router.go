// Copyright 2016 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package router provides an HTTP router with support for middleware and
// subrouters. It wraps around julienschmidt/httprouter.
package router

import (
	"context"
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"
)

// Router is the main type for the package. To create a Router, use New.
type Router struct {
	hrouter    *httprouter.Router
	middleware MiddlewareChain
	rootCtx    context.Context

	BasePath string
}

// Context contains the context, response writer, request, and params shared
// across Middleware and Handler functions.
type Context struct {
	Context     context.Context
	Writer      http.ResponseWriter
	Request     *http.Request
	Params      httprouter.Params
	HandlerPath string // the path with which the handler was registered
}

var _ http.Handler = (*Router)(nil)

// New creates a Router.
func New() *Router {
	return NewWithRootContext(context.Background())
}

// NewWithRootContext creates a router whose request contexts all inherit from
// the given context.
func NewWithRootContext(root context.Context) *Router {
	return &Router{
		hrouter:    httprouter.New(),
		middleware: NewMiddlewareChain(),
		rootCtx:    root,
		BasePath:   "/",
	}
}

// Use adds middleware chains to the group. The added middleware applies to
// all handlers registered on the router and to all handlers registered on
// routers that may be derived from the router (using Subrouter).
func (r *Router) Use(mc MiddlewareChain) {
	r.middleware = r.middleware.ExtendFrom(mc)
}

// Subrouter creates a new router with an updated base path.
// The new router copies middleware and configuration from the
// router it derives from.
func (r *Router) Subrouter(relativePath string) *Router {
	newRouter := &Router{
		hrouter:    r.hrouter,
		middleware: r.middleware,
		rootCtx:    r.rootCtx,
		BasePath:   makeBasePath(r.BasePath, relativePath),
	}
	return newRouter
}

// GET is a shortcut for router.Handle("GET", path, mc, h)
func (r *Router) GET(path string, mc MiddlewareChain, h Handler) {
	r.Handle("GET", path, mc, h)
}

// HEAD is a shortcut for router.Handle("HEAD", path, mc, h)
func (r *Router) HEAD(path string, mc MiddlewareChain, h Handler) {
	r.Handle("HEAD", path, mc, h)
}

// OPTIONS is a shortcut for router.Handle("OPTIONS", path, mc, h)
func (r *Router) OPTIONS(path string, mc MiddlewareChain, h Handler) {
	r.Handle("OPTIONS", path, mc, h)
}

// POST is a shortcut for router.Handle("POST", path, mc, h)
func (r *Router) POST(path string, mc MiddlewareChain, h Handler) {
	r.Handle("POST", path, mc, h)
}

// PUT is a shortcut for router.Handle("PUT", path, mc, h)
func (r *Router) PUT(path string, mc MiddlewareChain, h Handler) {
	r.Handle("PUT", path, mc, h)
}

// PATCH is a shortcut for router.Handle("PATCH", path, mc, h)
func (r *Router) PATCH(path string, mc MiddlewareChain, h Handler) {
	r.Handle("PATCH", path, mc, h)
}

// DELETE is a shortcut for router.Handle("DELETE", path, mc, h)
func (r *Router) DELETE(path string, mc MiddlewareChain, h Handler) {
	r.Handle("DELETE", path, mc, h)
}

// Handle registers a middleware chain and a handler for the given method and
// path. len(mc)==0 is allowed. See https://godoc.org/github.com/julienschmidt/httprouter
// for documentation on how the path may be formatted.
func (r *Router) Handle(method, path string, mc MiddlewareChain, h Handler) {
	p := makeBasePath(r.BasePath, path)
	handle := r.adapt(mc, h, p)
	r.hrouter.Handle(method, p, handle)
}

// ServeHTTP makes Router implement the http.Handler interface.
func (r *Router) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	r.hrouter.ServeHTTP(rw, req)
}

// GetParams parases the httprouter.Params from the supplied method and path.
//
// If nothing is registered for method/path, GetParams will return false.
func (r *Router) GetParams(method, path string) (httprouter.Params, bool) {
	if h, p, _ := r.hrouter.Lookup(method, path); h != nil {
		return p, true
	}
	return nil, false
}

// NotFound sets the handler to be called when no matching route is found.
func (r *Router) NotFound(mc MiddlewareChain, h Handler) {
	handle := r.adapt(mc, h, "")
	r.hrouter.NotFound = http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		handle(rw, req, nil)
	})
}

// adapt adapts given middleware chain and handler into a httprouter-style handle.
func (r *Router) adapt(mc MiddlewareChain, h Handler, path string) httprouter.Handle {
	return httprouter.Handle(func(rw http.ResponseWriter, req *http.Request, p httprouter.Params) {
		runChains(&Context{
			Context:     r.rootCtx,
			Writer:      rw,
			Request:     req,
			Params:      p,
			HandlerPath: path,
		}, r.middleware, mc, h)
	})
}

// makeBasePath combines the given base and relative path using "/".
// The result is: "/"+base+"/"+relative. Consecutive "/" are collapsed
// into a single "/". In addition, the following rules apply:
//         - The "/" between base and relative exists only if either base has a
//           trailing "/" or relative is not the empty string.
//         - A trailing "/" is added to the result if relative has a trailing
//           "/".
func makeBasePath(base, relative string) string {
	if !strings.HasSuffix(base, "/") && relative != "" {
		base += "/"
	}
	return httprouter.CleanPath(base + relative)
}
