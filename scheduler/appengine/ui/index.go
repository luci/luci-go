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
	"fmt"

	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/scheduler/appengine/presentation"
)

func indexPage(c *router.Context) {
	jobs, err := config(c.Context).Engine.GetVisibleJobs(c.Context)
	if err != nil {
		panic(err)
	}

	// Group jobs by project they belong to and their state.
	type projectAndJobs struct {
		ProjectID string
		Paused    sortedJobs
		Running   sortedJobs
		Scheduled sortedJobs
		Waiting   sortedJobs
	}
	var byProj []*projectAndJobs
	for _, job := range sortJobs(c.Context, jobs) {
		if len(byProj) == 0 || byProj[len(byProj)-1].ProjectID != job.ProjectID {
			byProj = append(byProj, &projectAndJobs{ProjectID: job.ProjectID})
		}
		group := byProj[len(byProj)-1]
		switch job.State {
		case presentation.PublicStatePaused:
			group.Paused = append(group.Paused, job)
		case presentation.PublicStateRunning:
			group.Running = append(group.Running, job)
		case presentation.PublicStateScheduled:
			group.Scheduled = append(group.Scheduled, job)
		case presentation.PublicStateWaiting, presentation.PublicStateDisabled:
			group.Waiting = append(group.Waiting, job)
		default:
			panic(fmt.Sprintf("unexpected job state %q in %v", job.State, job))
		}
	}

	templates.MustRender(c.Context, c.Writer, "pages/index.html", map[string]any{
		"Projects": byProj,
	})
}
