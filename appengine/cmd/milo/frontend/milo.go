// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package frontend

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
	"github.com/luci/luci-go/server/templates"
	"golang.org/x/net/context"

	"github.com/luci/luci-go/appengine/cmd/milo/buildbot"
	"github.com/luci/luci-go/appengine/cmd/milo/settings"
	"github.com/luci/luci-go/appengine/cmd/milo/swarming"
	"github.com/luci/luci-go/appengine/gaeauth/server"
)

// Where it all begins!!!
func init() {
	// Register plain ol' http services.
	r := httprouter.New()
	server.InstallHandlers(r, settings.Base)
	r.GET("/", wrap(dummy{}))
	r.GET("/swarming/:server/:id/steps/*logname", wrap(swarming.Log{}))
	r.GET("/swarming/:server/:id", wrap(swarming.Build{}))

	// Buildbot
	r.GET("/buildbot/:master/:builder/:build", wrap(buildbot.Build{}))
	r.GET("/buildbot/:master/:builder/", wrap(buildbot.Builder{}))

	// User settings
	r.GET("/settings", wrap(settings.Settings{}))
	r.POST("/settings", wrap(settings.Settings{}))

	http.Handle("/", r)
}

type dummy struct{}

func (d dummy) GetTemplateName(t settings.Theme) string {
	return "base.html"
}

func (d dummy) Render(c context.Context, r *http.Request, p httprouter.Params) (*templates.Args, error) {
	return &templates.Args{"contents": "This is the root page"}, nil
}

// Do all the middleware initilization and theme handling.
func wrap(h settings.ThemedHandler) httprouter.Handle {
	return settings.Wrap(h)
}
