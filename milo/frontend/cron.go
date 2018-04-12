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

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/milo/buildsource/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/server/router"
)

func StatsHandler(ctx *router.Context) {
	c, h := ctx.Context, ctx.Writer
	err := buildbot.StatsHandler(c)
	if err != nil {
		logging.Errorf(c, "failed to send stats: %s", err)
		h.WriteHeader(http.StatusInternalServerError)
		return
	}
	h.WriteHeader(http.StatusOK)
}

func UpdatePoolHandler(ctx *router.Context) {
	c, h := ctx.Context, ctx.Writer
	err := buildbucket.UpdatePools(c)
	if err != nil {
		logging.Errorf(c, "failed to update pools: %s", err)
		h.WriteHeader(http.StatusInternalServerError)
		return
	}
	h.WriteHeader(http.StatusOK)
}
