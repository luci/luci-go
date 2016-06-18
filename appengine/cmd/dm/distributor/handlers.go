// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package distributor

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/server/middleware"
	"golang.org/x/net/context"
)

// InstallHandlers installs the taskqueue callback handler.
//
// The `base` middleware must have a registry installed with WithRegistry.
func InstallHandlers(reg Registry, r *httprouter.Router, base middleware.Base) {
	r.POST(handlerPattern, base(
		gaemiddleware.RequireTaskQueue("", func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
			TaskQueueHandler(WithRegistry(c, reg), rw, r, p)
		})))

	r.POST("/_ah/push-handlers/"+notifyTopicSuffix, base(
		func(c context.Context, rw http.ResponseWriter, r *http.Request, p httprouter.Params) {
			PubsubReciever(WithRegistry(c, reg), rw, r, p)
		}))
}
