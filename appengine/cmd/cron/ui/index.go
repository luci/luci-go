// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/server/templates"
)

func indexPage(c context.Context, w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	templates.MustRender(c, w, "pages/index.html", nil)
}
