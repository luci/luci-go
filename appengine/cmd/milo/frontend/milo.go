// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"net/http"

	"github.com/julienschmidt/httprouter"

	"github.com/luci/luci-go/appengine/cmd/milo/buildbot"
	"github.com/luci/luci-go/appengine/cmd/milo/settings"
	"github.com/luci/luci-go/appengine/cmd/milo/swarming"
	"github.com/luci/luci-go/appengine/gaemiddleware"
)

// Where it all begins!!!
func init() {
	// Register plain ol' http services.
	r := httprouter.New()
	gaemiddleware.InstallHandlers(r, settings.Base)
	r.GET("/", wrap(frontpage{}))
	r.GET("/swarming/:server/:id/steps/*logname", wrap(swarming.Log{}))
	r.GET("/swarming/:server/:id", wrap(swarming.Build{}))

	// Buildbot
	r.GET("/buildbot/:master/:builder/:build", wrap(buildbot.Build{}))
	r.GET("/buildbot/:master/:builder/", wrap(buildbot.Builder{}))

	// User settings
	r.GET("/settings", wrap(settings.Settings{}))
	r.POST("/settings", settings.Base(settings.ChangeSettings))

	// PubSub subscription endpoints.
	r.POST("/pubsub/buildbot", settings.Base(buildbot.PubSubHandler))

	http.Handle("/", r)
}

// Do all the middleware initilization and theme handling.
func wrap(h settings.ThemedHandler) httprouter.Handle {
	return settings.Wrap(h)
}
