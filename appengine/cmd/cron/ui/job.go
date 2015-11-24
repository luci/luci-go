// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/server/templates"
)

func jobPage(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	projectID := p.ByName("ProjectID")
	jobID := p.ByName("JobID")

	// Grab the job from the datastore.
	job, err := config(c).Engine.GetCronJob(c, projectID+"/"+jobID)
	if err != nil {
		panic(err)
	}
	if job == nil {
		http.Error(w, "No such job", http.StatusNotFound)
		return
	}

	// Grab latest invocations from the datastore.
	invs, cursor, err := config(c).Engine.ListInvocations(c, job.JobID, 100, "")
	if err != nil {
		panic(err)
	}

	now := clock.Now(c).UTC()
	templates.MustRender(c, w, "pages/job.html", map[string]interface{}{
		"Job":               makeCronJob(job, now),
		"Invocations":       convertToInvocations(invs, now),
		"InvocationsCursor": cursor,
	})
}
