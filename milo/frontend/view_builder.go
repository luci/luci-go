// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"github.com/luci/luci-go/milo/buildsource"
	"github.com/luci/luci-go/server/router"
	"github.com/luci/luci-go/server/templates"
)

// BuilderHandler is responsible for taking a universal builder ID and rendering
// the builder page (defined in ./appengine/templates/pages/builder.html).
func BuilderHandler(c *router.Context, builderID buildsource.BuilderID) {
	builder, err := builderID.Get(c.Context, c.Params.ByName("limit"), c.Params.ByName("cursor"))
	// TODO(iannucci, hinoka): make MiloBuild refer to annotation stream by
	// host/prefix/path instead of by directly pulling it. Do all annotation
	// stream rendering in the frontend.
	if err != nil {
		ErrorHandler(c, err)
	} else {
		templates.MustRender(c.Context, c.Writer, "pages/builder.html", templates.Args{
			"Builder": builder,
		})
	}
}
