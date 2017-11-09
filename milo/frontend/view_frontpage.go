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
	"context"
	"sort"

	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/milo/common"
)

type Frontpage struct {
	Projects []Project
}

type Project struct {
	common.Project
	Consoles []*common.Console
}

func frontpageHandler(c *router.Context) {
	projs, err := GetAllProjects(c.Context)
	if err != nil {
		panic(err)
	}
	templates.MustRender(c.Context, c.Writer, "pages/frontpage.html", templates.Args{
		"frontpage": Frontpage{Projects: projs},
	})
}

// GetAllUserConsoles returns all consoles the current user has access to.
func GetAllProjects(c context.Context) ([]Project, error) {
	projects, err := common.GetAllProjects(c)
	if err != nil {
		return nil, err
	}
	projMap := make(map[string]*Project, len(projects))
	for _, project := range projects {
		projMap[project.ID] = &Project{Project: project}
	}
	// We get all consoles and filter it out after because GetAllConsoles() has
	// a fast path.
	consoles, err := common.GetAllConsoles(c, "")
	if err != nil {
		return nil, err
	}
	for _, con := range consoles {
		if proj, ok := projMap[con.Project()]; ok {
			proj.Consoles = append(proj.Consoles, con)
		}
	}

	projNames := make([]string, 0, len(projMap))
	for _, proj := range projMap {
		projNames = append(projNames, proj.ID)
	}
	sort.Strings(projNames)
	result := make([]Project, len(projMap))
	for i, name := range projNames {
		result[i] = *projMap[name]
	}
	return result, nil
}
