// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package distributor

import (
	"github.com/luci/luci-go/appengine/gaemiddleware"
	"github.com/luci/luci-go/server/router"
)

// InstallHandlers installs the taskqueue callback handler.
//
// The `base` middleware must have a registry installed with WithRegistry.
func InstallHandlers(reg Registry, r *router.Router, base router.MiddlewareChain) {
	r.POST(handlerPattern, append(base, gaemiddleware.RequireTaskQueue(""), func(c *router.Context, next router.Handler) {
		c.Context = WithRegistry(c.Context, reg)
		next(c)
	}), TaskQueueHandler)

	r.POST("/_ah/push-handlers/"+notifyTopicSuffix, append(base, func(c *router.Context, next router.Handler) {
		c.Context = WithRegistry(c.Context, reg)
		next(c)
	}), PubsubReceiver)
}
