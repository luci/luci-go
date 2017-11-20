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

package tsmon

import (
	"net/http"

	"go.chromium.org/luci/appengine/gaemiddleware"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/server/router"
)

// InstallHandlers installs HTTP handlers for tsmon routes.
func InstallHandlers(r *router.Router, base router.MiddlewareChain) {
	r.GET(
		"/internal/cron/ts_mon/housekeeping",
		base.Extend(gaemiddleware.RequireCron),
		housekeepingHandler)
}

// housekeepingHandler performs task number assignments and runs global metric
// callbacks.
//
// See DatastoreTaskNumAllocator and AssignTaskNumbers.
func housekeepingHandler(c *router.Context) {
	tsmon.GetState(c.Context).RunGlobalCallbacks(c.Context)
	if err := AssignTaskNumbers(c.Context); err != nil {
		logging.WithError(err).Errorf(c.Context, "Task number assigner failed")
		c.Writer.WriteHeader(http.StatusInternalServerError)
	}
}
