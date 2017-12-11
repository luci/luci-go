// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"go.chromium.org/luci/buildbucket/access"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/milo/buildsource"
	"go.chromium.org/luci/milo/common"
)

// BuilderHandler is responsible for taking a universal builder ID and rendering
// the builder page (defined in ./appengine/templates/pages/builder.html).
func BuilderHandler(c *router.Context, builderID buildsource.BuilderID) error {
	builderType, bucket, _, err := builderID.Split()
	if err != nil {
		return err
	}
	if builderType == "buildbucket" {
		perms, err := common.BucketPermissions(c.Context, bucket)
		switch {
		case err != nil:
			return err
		case !perms.Can(bucket, access.AccessBucket):
			return errors.Reason("builder %q not found", builderID).Tag(common.CodeNotFound).Err()
		}
	}
	limit := 25
	if tLimit := GetLimit(c.Request, -1); tLimit >= 0 {
		limit = tLimit
	}
	builder, err := builderID.Get(c.Context, limit, c.Request.FormValue("cursor"))
	// TODO(iannucci, hinoka): make MiloBuild refer to annotation stream by
	// host/prefix/path instead of by directly pulling it. Do all annotation
	// stream rendering in the frontend.
	if err != nil {
		return err
	}
	templates.MustRender(c.Context, c.Writer, "pages/builder.html", templates.Args{
		"Builder": builder,
	})
	return nil
}
