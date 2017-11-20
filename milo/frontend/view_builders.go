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
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"
)

// BuildersRelativeHandler is responsible for rendering a builders console page according to project.
// Presently only relative time is handled, i.e. last builds without correlation between builders),
// and no filtering by console has been implemented.
//
// The builders console page by relative time is defined in
// ./appengine/templates/pages/builders_relative_time.html.
func BuildersRelativeHandler(c *router.Context, projectID, console string) {
	limit := 30
	if tLimit := GetLimit(c.Request, -1); tLimit >= 0 {
		limit = tLimit
	}

	hists, err := model.GetBuilderHistories(c.Context, projectID, console, limit)
	if err != nil {
		ErrorHandler(c, err)
		return
	}

	templates.MustRender(c.Context, c.Writer, "pages/builders_relative_time.html", templates.Args{
		"Builders": hists,
	})
}
