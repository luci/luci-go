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
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/settings/admin"
	"go.chromium.org/luci/server/warmup"

	gaeauth "go.chromium.org/luci/appengine/gaeauth/server"
	"go.chromium.org/luci/appengine/tsmon"
	"go.chromium.org/luci/luci_config/appengine/gaeconfig"
)

// InstallHandlersWithMiddleware installs handlers for framework routes.
//
// These routes are needed for various services provided in Base context to
// work:
//  * Authentication related routes
//  * Settings pages
//  * Various housekeeping crons
//  * Warmup
//
// They must be installed into a default module, but it is also safe to install
// them into a non-default module. This may be handy if you want to move cron
// handlers into a non-default module.
//
// 'base' is expected to be an Environment's Base() or its derivative. It must
// NOT do any interception of requests (e.g. checking and rejecting
// unauthenticated requests).
func InstallHandlersWithMiddleware(r *router.Router, base router.MiddlewareChain) {
	gaeauth.InstallHandlers(r, base)
	tsmon.InstallHandlers(r, base)
	admin.InstallHandlers(r, base, &gaeauth.UsersAPIAuthMethod{})
	gaeconfig.InstallCacheCronHandler(r, base.Extend(RequireCron))
	warmup.InstallHandlers(r, base)
}
