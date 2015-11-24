// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"net/http"
	"strconv"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
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
		"Inv":       makeInvocation(inv, now),
	})
}
