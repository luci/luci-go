// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package ui

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"

	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"

	"github.com/luci/luci-go/scheduler/appengine/engine"
)

func invocationPage(c *router.Context) {
	projectID := c.Params.ByName("ProjectID")
	jobID := c.Params.ByName("JobID")
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
		inv, err1 = eng.GetInvocation(c.Context, projectID+"/"+jobID, invID)
	}()
	wg.Add(1)
	go func() {
		defer wg.Done()
		job, err2 = eng.GetJob(c.Context, projectID+"/"+jobID)
	}()
	wg.Wait()

	// panic on internal datastore errors to trigger HTTP 500.
	switch {
	case err1 != nil:
		panic(err1)
	case err2 != nil:
		panic(err2)
	case inv == nil:
		http.Error(c.Writer, "No such invocation", http.StatusNotFound)
		return
	case job == nil:
		http.Error(c.Writer, "No such job", http.StatusNotFound)
		return
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
		who := auth.CurrentIdentity(c.Context)
		return config(c.Context).Engine.AbortInvocation(c.Context, jobID, invID, who)
	})
}

func handleInvAction(c *router.Context, cb func(string, int64) error) {
	projectID := c.Params.ByName("ProjectID")
	jobID := c.Params.ByName("JobID")
	invID := c.Params.ByName("InvID")
	if !isJobOwner(c.Context, projectID, jobID) {
		http.Error(c.Writer, "Forbidden", 403)
		return
	}
	invIDAsInt, err := strconv.ParseInt(invID, 10, 64)
	if err != nil {
		http.Error(c.Writer, "Bad invocation ID", 400)
		return
	}
	if err := cb(projectID+"/"+jobID, invIDAsInt); err != nil {
		panic(err)
	}
	http.Redirect(c.Writer, c.Request, fmt.Sprintf("/jobs/%s/%s/%s", projectID, jobID, invID), http.StatusFound)
}
