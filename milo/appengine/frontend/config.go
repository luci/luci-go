// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"net/http"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/milo/appengine/common"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"
)

// ConfigsHandler renders the page showing the currently loaded set of luci-configs.
func ConfigsHandler(c *router.Context) {
	projects, err := common.GetAllProjects(c.Context)
	if err != nil {
		common.ErrorPage(
			c, http.StatusInternalServerError,
			"Error while getting projects: "+err.Error())
		return
	}

	templates.MustRender(c.Context, c.Writer, "pages/config.html", templates.Args{
		"Projects": projects,
	})
}

// UpdateHandler is an HTTP handler that handles configuration update requests.
// TODO(hinoka): Migrate to cfgclient and remove this.
func UpdateConfigHandler(ctx *router.Context) {
	c, h := ctx.Context, ctx.Writer
	err := common.UpdateProjectConfigs(c)
	if err != nil {
		logging.WithError(err).Errorf(c, "Update Handler encountered error")
		h.WriteHeader(500)
		return
	}
	logging.Infof(c, "Successfully completed")
	h.WriteHeader(200)
}
