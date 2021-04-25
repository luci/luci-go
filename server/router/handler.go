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

package router

// Handler is the type for all request handlers.
type Handler func(*Context)

// Middleware does some pre/post processing of a request.
//
// It is a function that accepts a context carrying an http.Request and
// an http.ResponseWriter and the `next` function. `next` is either the final
// request handler or a next link in the middleware chain.
//
// A middleware implementation must obey the following rules:
//   - Middleware *must* call `next` if it has not written the response itself.
//   - Middleware *must not* call `next` if it has written the response.
//   - Middleware *may* modify the context in-place before calling `next`.
//
// Note that writing the response after `next` is called is undefined behavior.
// It may or may not work depending on what exact happened down the chain.
type Middleware func(c *Context, next Handler)

// MiddlewareChain is an ordered collection of Middleware.
//
// The first middleware is the outermost, i.e. it will be called first when
// processing a request.
type MiddlewareChain []Middleware

// RunMiddleware executes the middleware chain and the handler with the given
// initial context. Useful to execute a chain of functions in tests.
func RunMiddleware(c *Context, mc MiddlewareChain, h Handler) {
	run(c, mc, nil, h)
}

// NewMiddlewareChain creates a new MiddlewareChain with the supplied Middleware
// entries.
func NewMiddlewareChain(mw ...Middleware) MiddlewareChain {
	if len(mw) == 0 {
		return nil
	}
	return append(MiddlewareChain(nil), mw...)
}

// Extend returns a new MiddlewareChain with the supplied Middleware appended to
// the end.
func (mc MiddlewareChain) Extend(mw ...Middleware) MiddlewareChain {
	ext := make(MiddlewareChain, 0, len(mc)+len(mw))
	return append(append(ext, mc...), mw...)
}

// run executes the middleware chains m and n and the handler h using
// c as the initial context. If a middleware or handler is nil, run
// simply advances to the next middleware or handler.
func run(c *Context, m, n MiddlewareChain, h Handler) {
	switch {
	case len(m) > 0:
		if m[0] != nil {
			m[0](c, func(ctx *Context) { run(ctx, m[1:], n, h) })
		} else {
			run(c, m[1:], n, h)
		}
	case len(n) > 0:
		if n[0] != nil {
			n[0](c, func(ctx *Context) { run(ctx, nil, n[1:], h) })
		} else {
			run(c, nil, n[1:], h)
		}
	case h != nil:
		h(c)
	}
}
