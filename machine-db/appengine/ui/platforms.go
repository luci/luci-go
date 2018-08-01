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

package ui

import (
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/machine-db/api/crimson/v1"
)

func platformsPage(c *router.Context) {
	resp, err := server(c.Context).ListPlatforms(c.Context, &crimson.ListPlatformsRequest{})
	if err != nil {
		renderErr(c, err)
		return
	}
	templates.MustRender(c.Context, c.Writer, "pages/platforms.html", map[string]interface{}{
		"Platforms": resp.Platforms,
	})
}
