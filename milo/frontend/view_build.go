// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package frontend

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
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

// handleLUCIBuild renders a LUCI build.
func handleLUCIBuild(c *router.Context) error {
	bucket := c.Params.ByName("bucket")
	builder := c.Params.ByName("builder")
	numberOrId := c.Params.ByName("numberOrId")

	var address string
	if strings.HasPrefix(numberOrId, "b") {
		address = numberOrId[1:]
	} else {
		address = fmt.Sprintf("%s/%s/%s", bucket, builder, numberOrId)
	}

	build, err := buildbucket.GetBuild(c.Context, address, true)
	// TODO(nodir): after switching to API v2, check that project, bucket
	// and builder in parameters indeed match the returned build. This is
	// relevant when the build is loaded by id.
	return renderBuild(c, build, err)
}

// redirectLUCIBuild redirects to a canonical build URL
// e.g. to /p/{project}/builders/{bucket}/{builder}/{number or id}.
func redirectLUCIBuild(c *router.Context) error {
	idStr := c.Params.ByName("id")
	// Verify it is an int64.
	if _, err := strconv.ParseInt(idStr, 10, 64); err != nil {
		return errors.Annotate(err, "invalid id").Tag(common.CodeParameterError).Err()
	}

	build, err := buildbucket.GetRawBuild(c.Context, idStr)
	if err != nil {
		return err
	}

	// If the build has a number, redirect to a URL with it.
	builder := ""
	u := *c.Request.URL
	for _, t := range build.Tags {
		switch k, v := strpair.Parse(t); k {
		case bbv1.TagBuildAddress:
			_, project, bucket, builder, number, _ := bbv1.ParseBuildAddress(v)
			if number > 0 {
				u.Path = fmt.Sprintf("/p/%s/builders/%s/%s/%d", project, bucket, builder, number)
				http.Redirect(c.Writer, c.Request, u.String(), http.StatusMovedPermanently)
				return nil
			}

		case bbv1.TagBuilder:
			builder = v
		}
	}
	if builder == "" {
		return errors.Reason("build %s does not have a builder", idStr).Tag(common.CodeParameterError).Err()
	}

	u.Path = fmt.Sprintf("/p/%s/builders/%s/%s/b%d", build.Project, build.Bucket, builder, build.Id)
	http.Redirect(c.Writer, c.Request, u.String(), http.StatusMovedPermanently)
	return nil
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

func getStepDisplayPrefCookie(c *router.Context) ui.StepDisplayPref {
	switch cookie, err := c.Request.Cookie("stepDisplayPref"); err {
	case nil:
		return ui.StepDisplayPref(cookie.Value)
	case http.ErrNoCookie:
		return ui.Collapsed
	default:
		logging.WithError(err).Errorf(c.Context, "failed to read stepDisplayPref cookie")
		return ui.Collapsed
	}
}

// renderBuild is a shortcut for rendering build or returning err if it is not
// nil. Also calls build.Fix().
func renderBuild(c *router.Context, build *ui.MiloBuild, err error) error {
	if err != nil {
		return err
	}

	build.StepDisplayPref = getStepDisplayPrefCookie(c)
	build.Fix(c.Context)
	templates.MustRender(c.Context, c.Writer, "pages/build.html", templates.Args{
		"Build": build,
	})
	return nil
}
