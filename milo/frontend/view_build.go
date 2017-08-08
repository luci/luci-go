// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"go.chromium.org/luci/milo/buildsource"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"
)

// BuildHandler is responsible for taking a universal build ID and rendering the
// build page (defined in ./appengine/templates/pages/build.html).
func BuildHandler(c *router.Context, buildID buildsource.ID) {
	build, err := buildID.Get(c.Context)
	// TODO(iannucci, hinoka): make MiloBuild refer to annotation stream by
	// host/prefix/path instead of by directly pulling it. Do all annotation
	// stream rendering in the frontend.
	if err != nil {
		ErrorHandler(c, err)
	} else {
		templates.MustRender(c.Context, c.Writer, "pages/build.html", templates.Args{
			"Build": build,
		})
	}
}
