// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"strconv"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/milo/api/proto"
	"go.chromium.org/luci/milo/buildsource"
	"go.chromium.org/luci/milo/buildsource/buildbot/buildstore"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"
)

// BuilderHandler is responsible for taking a universal builder ID and rendering
// the builder page (defined in ./appengine/templates/pages/builder.html).
func BuilderHandler(c *router.Context, builderID buildsource.BuilderID) {
	limit := 25
	if tLimit := GetLimit(c.Request, -1); tLimit >= 0 {
		limit = tLimit
	}

	backend, masterName, builderName, err := builderID.Split()
	if err != nil {
		ErrorHandler(c, err)
		return
	}
	// TODO(nodir, hinoka): fix the leaking BuilderHandler abstraction.
	if bucket := c.Request.FormValue("em-bucket"); backend == "buildbot" && bucket != "" {
		emOptions := milo.EmulationOptions{Bucket: bucket}
		if s := c.Request.FormValue("em-start"); s != "" {
			start, err := strconv.Atoi(s)
			if err != nil {
				err := errors.Annotate(err, "invalid emulation-start").Tag(common.CodeParameterError).Err()
				ErrorHandler(c, err)
				return
			}
			emOptions.StartFrom = int32(start)
		}
		c.Context = buildstore.WithEmulationOptions(c.Context, masterName, builderName, emOptions)
	}

	builder, err := builderID.Get(c.Context, limit, c.Request.FormValue("cursor"))
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
