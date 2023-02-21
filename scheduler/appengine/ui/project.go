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
	"strings"

	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/scheduler/appengine/presentation"
)

func projectPage(c *router.Context) {
	projectID := c.Params.ByName("ProjectID")
	jobs, err := config(c.Context).Engine.GetVisibleProjectJobs(c.Context, projectID)
	if err != nil {
		panic(err)
	}

	check := func(j *schedulerJob) bool { return true }

	// See index.html and index.go for these values.
	filter := strings.ToLower(c.Request.URL.Query().Get("filter"))
	switch filter {
	case "running":
		check = func(j *schedulerJob) bool { return j.State == presentation.PublicStateRunning }
	case "scheduled":
		check = func(j *schedulerJob) bool { return j.State == presentation.PublicStateScheduled }
	case "waiting":
		check = func(j *schedulerJob) bool {
			return j.State == presentation.PublicStateWaiting ||
				j.State == presentation.PublicStateDisabled
		}
	case "paused":
		check = func(j *schedulerJob) bool { return j.State == presentation.PublicStatePaused }
	default:
		filter = "" // unknown filter, ignore it in UI
	}

	// Filter jobs used `check`.
	sorted := sortJobs(c.Context, jobs)
	filtered := sorted[:0]
	for _, job := range sorted {
		if check(job) {
			filtered = append(filtered, job)
		}
	}

	templates.MustRender(c.Context, c.Writer, "pages/project.html", map[string]any{
		"ProjectID":    projectID,
		"ProjectEmpty": len(sorted) == 0,
		"Filter":       filter,
		"Jobs":         filtered,
	})
}
