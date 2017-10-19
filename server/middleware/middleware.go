// Copyright 2015 The LUCI Authors.
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

// Package middleware defines base type for context-aware HTTP request handler.
// See appengine/middleware for examples of how to use it in GAE environment.
package middleware

import (
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"go.chromium.org/luci/server/router"
)

// Handler is the type for all request handlers. Of particular note, it's the
// same as httprouter.Handle, except that it also has a context parameter.
type Handler func(context.Context, http.ResponseWriter, *http.Request, httprouter.Params)

// Middleware takes a handler, wraps it with some additional logic, and returns
// resulting handler.
type Middleware func(Handler) Handler

// Base is a start of the middlware chain. It sets up initial context with all
// base services and passes it to the given handler. Return value of Base can
// be plugged in into httprouter directly.
type Base func(Handler) httprouter.Handle

// TestingBase is Base that passes given context to the handler. Useful in
// tests.
func TestingBase(c context.Context) Base {
	return func(h Handler) httprouter.Handle {
		return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
			h(c, rw, r, p)
		}
	}
}

// WithContextTimeout returns a middleware that adds a timeout to the context.
func WithContextTimeout(timeout time.Duration) router.Middleware {
	return func(c *router.Context, next router.Handler) {
		c.Context, _ = context.WithTimeout(c.Context, timeout)
		next(c)
	}
}
