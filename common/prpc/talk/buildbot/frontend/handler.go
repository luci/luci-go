// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package buildbot

import (
	"net/http"

	"github.com/julienschmidt/httprouter"

	"github.com/luci/luci-go/appengine/gaemiddleware"
)

func init() {
	router := httprouter.New()
	gaemiddleware.InstallHandlers(router, gaemiddleware.BaseProd)

	InstallAPIRoutes(router, gaemiddleware.BaseProd)

	http.DefaultServeMux.Handle("/", router)
}
