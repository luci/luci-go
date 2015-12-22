// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"fmt"
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/gae/service/memcache"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/server/auth"
	"github.com/luci/luci-go/server/auth/xsrf"
	"github.com/luci/luci-go/server/templates"
)

func jobPage(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	projectID := p.ByName("ProjectID")
	jobID := p.ByName("JobID")
	cursor := r.URL.Query().Get("c")

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
	invs, nextCursor, err := config(c).Engine.ListInvocations(c, job.JobID, 50, cursor)
	if err != nil {
		panic(err)
	}

	// Cheesy way of implementing bidirectional pagination with forward-only
	// datastore cursors: store mapping from a page cursor to a previous page
	// cursor in the memcache.
	keyPrefix := fmt.Sprintf("v1:cursors:list_invocations:%s:", job.JobID)
	mc := memcache.Get(c)
	prevCursor := ""
	if cursor != "" {
		if itm, err := mc.Get(keyPrefix + cursor); err == nil {
			prevCursor = string(itm.Value())
		}
	}
	if nextCursor != "" {
		itm := mc.NewItem(keyPrefix + nextCursor)
		if cursor == "" {
			itm.SetValue([]byte("NULL"))
		} else {
			itm.SetValue([]byte(cursor))
		}
		itm.SetExpiration(24 * time.Hour)
		mc.Set(itm)
	}

	now := clock.Now(c).UTC()
	templates.MustRender(c, w, "pages/job.html", map[string]interface{}{
		"XsrfTokenField": xsrf.TokenField(c),
		"Job":            makeCronJob(job, now),
		"Invocations":    convertToInvocations(invs, now),
		"PrevCursor":     prevCursor,
		"NextCursor":     nextCursor,
	})
}

func runJobAction(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// TODO(vadimsh): Do real ACLs.
	switch ok, err := auth.IsMember(c, "administrators"); {
	case err != nil:
		panic(err)
	case !ok:
		http.Error(w, "Forbidden", 403)
		return
	}

	projectID := p.ByName("ProjectID")
	jobID := p.ByName("JobID")

	err := config(c).Engine.TriggerInvocation(c, projectID+"/"+jobID, auth.CurrentIdentity(c))
	templates.MustRender(c, w, "pages/run_job_result.html", map[string]interface{}{
		"ProjectID": projectID,
		"JobID":     jobID,
		"Error":     err,
	})
}
