// Copyright 2017 The LUCI Authors.
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
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/milo/internal/buildsource/swarming"
)

// HandleSwarmingLog renders a step log from a swarming build.
func HandleSwarmingLog(c *router.Context) error {
	log, closed, err := swarming.GetLog(
		c.Request.Context(),
		c.Request.FormValue("server"),
		c.Params.ByName("id"),
		c.Params.ByName("logname"))
	if err != nil {
		return err
	}

	templates.MustRender(c.Request.Context(), c.Writer, "pages/log.html", templates.Args{
		"Log":    log,
		"Closed": closed,
	})
	return nil
}
