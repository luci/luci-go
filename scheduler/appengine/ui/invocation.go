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
	"sync"

	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"

	"github.com/luci/luci-go/scheduler/appengine/engine"
)

func invocationPage(c *router.Context) {
	projectID := c.Params.ByName("ProjectID")
	jobName := c.Params.ByName("JobName")
	invID, err := strconv.ParseInt(c.Params.ByName("InvID"), 10, 64)
	if err != nil {
		http.Error(c.Writer, "No such invocation", http.StatusNotFound)
		return
	}

	var inv *engine.Invocation
	var job *engine.Job
	var err1, err2 error

	eng := config(c.Context).Engine

	wg := sync.WaitGroup{}
	wg.Add(1)
	go func() {
		defer wg.Done()
		inv, err1 = eng.GetVisibleInvocation(c.Context, projectID+"/"+jobName, invID)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		job, err2 = eng.GetVisibleJob(c.Context, projectID+"/"+jobName)
	}()
	wg.Wait()

	// panic on internal datastore errors to trigger HTTP 500.
	switch {
	case err2 == engine.ErrNoSuchJob:
		http.Error(c.Writer, "No such job or no permission", http.StatusNotFound)
		return
	case err1 == engine.ErrNoSuchInvocation:
		http.Error(c.Writer, "No such invocation", http.StatusNotFound)
		return
	case err1 != nil:
		panic(err1)
	case err2 != nil:
		panic(err2)
	}

	jobUI := makeJob(c.Context, job)
	templates.MustRender(c.Context, c.Writer, "pages/invocation.html", map[string]interface{}{
		"Job": jobUI,
		"Inv": makeInvocation(jobUI, inv),
	})
}

////////////////////////////////////////////////////////////////////////////////
// Actions.

func abortInvocationAction(c *router.Context) {
	handleInvAction(c, func(jobID string, invID int64) error {
		return config(c.Context).Engine.AbortInvocation(c.Context, jobID, invID)
	})
}

func handleInvAction(c *router.Context, cb func(string, int64) error) {
	projectID := c.Params.ByName("ProjectID")
	jobName := c.Params.ByName("JobName")
	invID := c.Params.ByName("InvID")
	invIDAsInt, err := strconv.ParseInt(invID, 10, 64)
	if err != nil {
		http.Error(c.Writer, "Bad invocation ID", 400)
		return
	}
	switch err := cb(projectID+"/"+jobName, invIDAsInt); {
	case err == engine.ErrNoOwnerPermission:
		http.Error(c.Writer, "Forbidden", 403)
		return
	case err != nil:
		panic(err)
	default:
		http.Redirect(c.Writer, c.Request, fmt.Sprintf("/jobs/%s/%s/%s", projectID, jobName, invID), http.StatusFound)
	}
}
