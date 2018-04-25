// Copyright 2018 The LUCI Authors.
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

package frontend

import (
	"net/http"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/milo/buildsource/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/server/router"
)

// cronHandler is a generic wrapper for running a cron job handler.
// You give it a function that takes a context.Context which returns an error.
// Then the handler method is fed into the frontend router.
type cronHandler func(c context.Context) error

// handler is compatable with the frontend router.  It runs the underlying handler
// and produces the expected status messages.
func (h cronHandler) handler(ctx *router.Context) {
	c, w := ctx.Context, ctx.Writer
	err := h(c)
	if err != nil {
		logging.WithError(err).Errorf(c, "failed to run")
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

var (
	stats        = cronHandler(buildbot.StatsHandler)
	updatePool   = cronHandler(buildbucket.UpdatePools)
	fixDatastore = cronHandler(fixDatastoreHandler)
)
