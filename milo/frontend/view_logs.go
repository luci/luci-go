// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/milo/buildsource/swarming"
)

// HandleSwarmingLog renders a step log from a swarming build.
func HandleSwarmingLog(c *router.Context) error {
	log, closed, err := swarming.GetLog(
		c.Context,
		c.Request.FormValue("server"),
		c.Params.ByName("id"),
		c.Params.ByName("logname"))
	if err != nil {
		return err
	}

	templates.MustRender(c.Context, c.Writer, "pages/log.html", templates.Args{
		"Log":    log,
		"Closed": closed,
	})
	return nil
}
