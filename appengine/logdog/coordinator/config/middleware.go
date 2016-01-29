// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/luci-go/server/middleware"
	"golang.org/x/net/context"
)

// WithConfig is a middleware.Handler that installs the LogDog Coordinator
// configuration into the Context.
func WithConfig(h middleware.Handler) middleware.Handler {
	return func(c context.Context, rw http.ResponseWriter, r *http.Request, params httprouter.Params) {
		c = UseConfig(c)
		h(c, rw, r, params)
	}
}
