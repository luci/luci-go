// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package templates

import (
	"fmt"
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/middleware"
)

// WithTemplates is middleware that lazily loads template bundle and injects it
// into the context.
//
// Wrapper reply with HTTP 500 if templates can not be loaded. Inner handler
// receives context with all templates successfully loaded.
func WithTemplates(h middleware.Handler, b *Bundle) middleware.Handler {
	return func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
		c = Use(c, b) // calls EnsureLoaded and initializes b.err inside
		if b.err != nil {
			http.Error(rw, fmt.Sprintf("Can't load HTML templates.\n%s", b.err), http.StatusInternalServerError)
			return
		}
		h(c, rw, r, p)
	}
}
