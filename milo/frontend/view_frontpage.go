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

package frontend

import (
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/milo/common"
)

type Frontpage struct {
	Projects []common.Project
}

func frontpageHandler(c *router.Context) {
	projs, err := common.GetAllProjects(c.Context)
	if err != nil {
		ErrorHandler(c, err)
		return
	}
	templates.MustRender(c.Context, c.Writer, "pages/frontpage.html", templates.Args{
		"frontpage": Frontpage{Projects: projs},
	})
}

func frontpageTestData() []common.TestBundle {
	return []common.TestBundle{
		{
			Description: "Basic frontpage",
			Data: templates.Args{
				"frontpage": Frontpage{
					Projects: []common.Project{
						{
							ID:      "fakeproject",
							LogoURL: "https://example.com/logo.png",
						},
					},
				},
			},
		},
	}
}
