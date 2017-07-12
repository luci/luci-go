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

package gaemiddleware

import (
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/settings/admin"
	"github.com/luci/luci-go/server/warmup"

	gaeauth "github.com/luci/luci-go/appengine/gaeauth/server"
	"github.com/luci/luci-go/appengine/tsmon"
	"github.com/luci/luci-go/luci_config/appengine/gaeconfig"
)

// InstallHandlers installs handlers for framework routes.
//
// These routes are needed for various services provided in BaseProd context to
// work:
//  * Authentication related routes
//  * Settings pages
//  * Various housekeeping crons
//  * Warmup
//
// They must be installed into a default module, but it is also safe to install
// them into a non-default module. This may be handy if you want to move cron
// handlers into a non-default module.
func InstallHandlers(r *router.Router) {
	InstallHandlersWithMiddleware(r, BaseProd())
}

// InstallHandlersWithMiddleware installs handlers for framework routes.
//
// It is same as 'InstallHandlers', but allows caller to customize the
// middleware chain used for the routes. This may be needed if application
// callbacks invoked through the default routes (settings pages, monitoring
// callbacks) need some additional state in the context.
//
// 'base' is expected to be BaseProd() or its derivative. It must NOT do any
// interception of requests (e.g. checking and rejecting unauthenticated
// requests).
func InstallHandlersWithMiddleware(r *router.Router, base router.MiddlewareChain) {
	gaeauth.InstallHandlers(r, base)
	tsmon.InstallHandlers(r, base)
	admin.InstallHandlers(r, base, &gaeauth.UsersAPIAuthMethod{})
	gaeconfig.InstallCacheCronHandler(r, base.Extend(RequireCron))
	warmup.InstallHandlers(r, base)
}
