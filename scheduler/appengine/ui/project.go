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

package ui

import (
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"
)

func projectPage(c *router.Context) {
	projectID := c.Params.ByName("ProjectID")
	jobs, err := config(c.Context).Engine.GetVisibleProjectJobs(c.Context, projectID)
	if err != nil {
		panic(err)
	}
	templates.MustRender(c.Context, c.Writer, "pages/project.html", map[string]interface{}{
		"ProjectID": projectID,
		"Jobs":      sortJobs(c.Context, jobs),
	})
}
