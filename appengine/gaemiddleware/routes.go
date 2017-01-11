// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package gaemiddleware

import (
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/settings/admin"

	gaeauth "github.com/luci/luci-go/appengine/gaeauth/server"
	"github.com/luci/luci-go/appengine/tsmon"
	"github.com/luci/luci-go/luci_config/appengine/gaeconfig"
)

// InstallHandlers installs HTTP handlers for various default routes.
//
// These routes are needed for various services provided in BaseProd context to
// work (e.g. authentication related routes, time series monitoring, etc).
//
// 'base' is expected to be BaseProd() or its derivative. It must NOT do any
// interception of requests (e.g. checking and rejecting unauthenticated
// requests). It may inject additional state in the context though, if it is
// needed by various tsmon callbacks or settings pages.
//
// In majority of cases 'base' should just be BaseProd(). If unsure what to use,
// use BaseProd().
func InstallHandlers(r *router.Router, base router.MiddlewareChain) {
	gaeauth.InstallHandlers(r, base)
	tsmon.InstallHandlers(r, base)
	admin.InstallHandlers(r, base, &gaeauth.UsersAPIAuthMethod{})
	gaeconfig.InstallCacheCronHandler(r, base.Extend(RequireCron))
}
