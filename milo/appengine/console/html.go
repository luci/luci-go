// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package console

import (
	"net/http"

	"github.com/luci/luci-go/milo/appengine/common"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"
)

// ConsoleHandler renders the console page.
func ConsoleHandler(c *router.Context) {
	project := c.Params.ByName("project")
	if project == "" {
		common.ErrorPage(c, http.StatusBadRequest, "Missing Project")
		return
	}
	name := c.Params.ByName("name")

	result, err := console(c.Context, project, name)
	if err != nil {
		common.ErrorPage(c, http.StatusInternalServerError, err.Error())
		return
	}

	templates.MustRender(c.Context, c.Writer, "pages/console.html", templates.Args{
		"Console": result,
	})
}
