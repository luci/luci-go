// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	buildbotapi "go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbot/buildstore"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/milo/buildsource/rawpresentation"
	"go.chromium.org/luci/milo/buildsource/swarming"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/frontend/ui"
)

// handleBuildbotBuild renders a buildbot build.
// Requires emulationMiddleware.
func handleBuildbotBuild(c *router.Context) error {
	buildNum, err := strconv.Atoi(c.Params.ByName("number"))
	if err != nil {
		return errors.Annotate(err, "build number is not a number").
			Tag(common.CodeParameterError).
			Err()
	}
	id := buildbotapi.BuildID{
		Master:  c.Params.ByName("master"),
		Builder: c.Params.ByName("builder"),
		Number:  buildNum,
	}
	if err := id.Validate(); err != nil {
		return err
	}

	// If this build is emulated, redirect to LUCI.
	b, err := buildstore.EmulationOf(c.Context, id)
	switch {
	case err != nil:
		return err
	case b != nil && b.Number != nil:
		u := *c.Request.URL
		u.Path = fmt.Sprintf("/p/%s/builders/%s/%s/%d", b.Project, b.Bucket, b.Builder, *b.Number)
		http.Redirect(c.Writer, c.Request, u.String(), http.StatusFound)
		return nil
	default:
		build, err := buildbot.GetBuild(c.Context, id)
		return renderBuild(c, build, err)
	}
}

// handleLUCIBuildByNumber renders a LUCI build given a bucket, builder and a
// build number.
func handleLUCIBuildByNumber(c *router.Context) error {
	address := fmt.Sprintf("%s/%s/%s",
		c.Params.ByName("bucket"),
		c.Params.ByName("builder"),
		c.Params.ByName("number"))
	build, err := buildbucket.GetBuild(c.Context, address)
	return renderBuild(c, build, err)
}

// handleLUCIBuildByID renders a LUCI build given a build id.
func handleLUCIBuildByID(c *router.Context) error {
	idStr := c.Params.ByName("id")
	// Verify it is an int64.
	if _, err := strconv.ParseInt(idStr, 10, 64); err != nil {
		return errors.Annotate(err, "invalid id").Tag(common.CodeParameterError).Err()
	}

	build, err := buildbucket.GetBuild(c.Context, idStr)
	// TODO(nodir): redirect to build number handler if the build has a number.
	return renderBuild(c, build, err)
}

func handleSwarmingBuild(c *router.Context) error {
	build, err := swarming.GetBuild(
		c.Context,
		c.Request.FormValue("server"),
		c.Params.ByName("id"))
	return renderBuild(c, build, err)
}

func handleRawPresentationBuild(c *router.Context) error {
	build, err := rawpresentation.GetBuild(
		c.Context,
		c.Params.ByName("logdog_host"),
		types.ProjectName(c.Params.ByName("project")),
		types.StreamPath(strings.Trim(c.Params.ByName("path"), "/")))
	return renderBuild(c, build, err)
}

// renderBuild is a shortcut for rendering build or returning err if it is not
// nil. Also calls build.Fix().
func renderBuild(c *router.Context, build *ui.MiloBuild, err error) error {
	if err != nil {
		return err
	}

	build.Fix()
	templates.MustRender(c.Context, c.Writer, "pages/build.html", templates.Args{
		"Build": build,
	})
	return nil
}
