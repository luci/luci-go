// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package ui

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/templates"
)

func invocationPage(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	projectID := p.ByName("ProjectID")
	jobID := p.ByName("JobID")
	invID, err := strconv.ParseInt(p.ByName("InvID"), 10, 64)
	if err != nil {
		http.Error(w, "No such invocation", http.StatusNotFound)
		return
	}

	inv, err := config(c).Engine.GetInvocation(c, projectID+"/"+jobID, invID)
	if err != nil {
		panic(err)
	}
	if inv == nil {
		http.Error(w, "No such invocation", http.StatusNotFound)
		return
	}

	now := clock.Now(c).UTC()
	templates.MustRender(c, w, "pages/invocation.html", map[string]interface{}{
		"ProjectID": projectID,
		"JobID":     jobID,
		"Inv":       makeInvocation(projectID, jobID, inv, now),
	})
}

////////////////////////////////////////////////////////////////////////////////
// Actions.

func abortInvocationAction(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	handleInvAction(c, w, r, p, func(jobID string, invID int64) error {
		who := auth.CurrentIdentity(c)
		return config(c).Engine.AbortInvocation(c, jobID, invID, who)
	})
}

func handleInvAction(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params, cb func(string, int64) error) {
	projectID := p.ByName("ProjectID")
	jobID := p.ByName("JobID")
	invID := p.ByName("InvID")
	if !isJobOwner(c, projectID, jobID) {
		http.Error(w, "Forbidden", 403)
		return
	}
	invIDAsInt, err := strconv.ParseInt(invID, 10, 64)
	if err != nil {
		http.Error(w, "Bad invocation ID", 400)
		return
	}
	if err := cb(projectID+"/"+jobID, invIDAsInt); err != nil {
		panic(err)
	}
	http.Redirect(w, r, fmt.Sprintf("/jobs/%s/%s/%s", projectID, jobID, invID), http.StatusFound)
}
