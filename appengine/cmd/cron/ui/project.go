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

func projectPage(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	projectID := p.ByName("ProjectID")
	jobs, err := config(c).Engine.GetProjectCronJobs(c, projectID)
	if err != nil {
		panic(err)
	}
	templates.MustRender(c, w, "pages/project.html", map[string]interface{}{
		"ProjectID": projectID,
		"Jobs":      convertToSortedCronJobs(jobs, clock.Now(c).UTC()),
	})
}
