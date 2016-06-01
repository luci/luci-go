// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package gaetesting

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/luci-go/server/middleware"
	"github.com/luci/luci-go/server/proccache"
	"github.com/luci/luci-go/server/secrets/testsecrets"
)

// BaseTest adapts a middleware-style handler to a httprouter.Handle. It passes
// a new context to `h` with the following services installed:
//   * github.com/luci/gae/impl/memory (in-memory appengine services)
//   * github.com/luci/luci-go/server/secrets/testsecrets (access to fake secret keys)
func BaseTest(h middleware.Handler) httprouter.Handle {
	return func(rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		h(TestingContext(), rw, r, p)
	}
}

// TestingContext returns context with base services installed.
func TestingContext() context.Context {
	c := context.Background()
	c = memory.Use(c)
	c = testsecrets.Use(c)
	c = proccache.Use(c, &proccache.Cache{})
	return c
}
