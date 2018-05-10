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
	"net/http"
	"strconv"

	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/scheduler/appengine/engine"
)

func invocationPage(c *router.Context) {
	projectID := c.Params.ByName("ProjectID")
	jobName := c.Params.ByName("JobName")
	invID, err := strconv.ParseInt(c.Params.ByName("InvID"), 10, 64)
	if err != nil {
		uiErrBadInvocationID.render(c)
		return
	}

	job := jobFromEngine(c, projectID, jobName)
	if job == nil {
		return
	}

	inv, err := config(c.Context).Engine.GetInvocation(c.Context, job, invID)
	switch {
	case err == engine.ErrNoSuchInvocation:
		uiErrNoInvocation.render(c)
		return
	case err != nil:
		panic(err)
	}

	jobUI := makeJob(c.Context, job, nil)
	templates.MustRender(c.Context, c.Writer, "pages/invocation.html", map[string]interface{}{
		"Job": jobUI,
		"Inv": makeInvocation(jobUI, inv),
	})
}

////////////////////////////////////////////////////////////////////////////////
// Actions.

func abortInvocationAction(c *router.Context) {
	handleInvAction(c, func(job *engine.Job, invID int64) error {
		return config(c.Context).Engine.AbortInvocation(c.Context, job, invID)
	})
}

func handleInvAction(c *router.Context, cb func(*engine.Job, int64) error) {
	projectID := c.Params.ByName("ProjectID")
	jobName := c.Params.ByName("JobName")
	invID := c.Params.ByName("InvID")
	invIDAsInt, err := strconv.ParseInt(invID, 10, 64)
	if err != nil {
		uiErrBadInvocationID.render(c)
		return
	}

	job := jobFromEngine(c, projectID, jobName)
	if job == nil {
		return
	}

	switch err := cb(job, invIDAsInt); {
	case err == engine.ErrNoPermission:
		uiErrActionForbidden.render(c)
		return
	case err != nil:
		panic(err)
	default:
		http.Redirect(c.Writer, c.Request, fmt.Sprintf("/jobs/%s/%s/%s", projectID, jobName, invID), http.StatusFound)
	}
}
