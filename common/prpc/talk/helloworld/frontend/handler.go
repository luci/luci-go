// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package helloworld

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
