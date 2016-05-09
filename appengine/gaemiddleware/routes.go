// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gaemiddleware

import (
	"github.com/julienschmidt/httprouter"
	"github.com/luci/luci-go/server/middleware"

	gaeauth "github.com/luci/luci-go/appengine/gaeauth/server"
	"github.com/luci/luci-go/appengine/tsmon"
)

// InstallHandlers installs HTTP handlers for various default routes.
//
// These routes are needed for various services provided in BaseProd context to
// work (e.g. authentication related routes, time series monitoring, etc).
//
// 'base' is expected to be BaseProd or its derivative.
func InstallHandlers(r *httprouter.Router, base middleware.Base) {
	gaeauth.InstallHandlers(r, base)
	tsmon.InstallHandlers(r, base)
}
