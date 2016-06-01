// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package ui

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/server/templates"
)

func indexPage(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	jobs, err := config(c).Engine.GetAllCronJobs(c)
	if err != nil {
		panic(err)
	}
	templates.MustRender(c, w, "pages/index.html", map[string]interface{}{
		"Jobs": convertToSortedCronJobs(jobs, clock.Now(c).UTC()),
	})
}
