// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"net/http"

	"cloud.google.com/go/datastore"

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
	sc, err := common.GetCurrentServiceConfig(c.Context)
	if err != nil && err != datastore.ErrNoSuchEntity {
		common.ErrorPage(
			c, http.StatusInternalServerError,
			"Error while getting service config: "+err.Error())
		return
	}

	templates.MustRender(c.Context, c.Writer, "pages/configs.html", templates.Args{
		"Projects":      projects,
		"ServiceConfig": sc,
	})
}

// UpdateHandler is an HTTP handler that handles configuration update requests.
func UpdateConfigHandler(ctx *router.Context) {
	c, h := ctx.Context, ctx.Writer
	projErr := common.UpdateProjectConfigs(c)
	if projErr != nil {
		logging.WithError(projErr).Errorf(c, "project update handler encountered error")
	}
	servErr := common.UpdateServiceConfig(c)
	if servErr != nil {
		logging.WithError(servErr).Errorf(c, "service update handler encountered error")
	}
	if projErr != nil || servErr != nil {
		h.WriteHeader(http.StatusInternalServerError)
		return
	}
	h.WriteHeader(http.StatusOK)
}
