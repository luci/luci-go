// Copyright 2018 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package frontend

import (
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/julienschmidt/httprouter"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	buildbotapi "go.chromium.org/luci/milo/api/buildbot"
	"go.chromium.org/luci/milo/api/config"
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
		return renderBuildLegacy(c, build, false, err)
	}
}

// handleLUCIBuildLegacy renders a LUCI build.
func handleLUCIBuildLegacy(c *router.Context, bucket, builder, numberOrId string) error {
	var address string
	if strings.HasPrefix(numberOrId, "b") {
		address = numberOrId[1:]
	} else {
		address = fmt.Sprintf("%s/%s/%s", bucket, builder, numberOrId)
	}

	build, err := buildbucket.GetBuildLegacy(c.Context, address, true)
	return renderBuildLegacy(c, build, true, err)
}

func handleSwarmingBuild(c *router.Context) error {
	build, err := swarming.GetBuild(
		c.Context,
		c.Request.FormValue("server"),
		c.Params.ByName("id"))
	return renderBuildLegacy(c, build, false, err)
}

func handleRawPresentationBuild(c *router.Context) error {
	build, err := rawpresentation.GetBuild(
		c.Context,
		c.Params.ByName("logdog_host"),
		types.ProjectName(c.Params.ByName("project")),
		types.StreamPath(strings.Trim(c.Params.ByName("path"), "/")))
	return renderBuildLegacy(c, build, false, err)
}

// renderBuildLegacy is a shortcut for rendering build or returning err if it is not
// nil. Also calls build.Fix().
func renderBuildLegacy(c *router.Context, build *ui.MiloBuildLegacy, renderTimeline bool, err error) error {
	if err != nil {
		return err
	}

	build.StepDisplayPref = getStepDisplayPrefCookie(c)
	build.Fix(c.Context)

	templates.MustRender(c.Context, c.Writer, "pages/build_legacy.html", templates.Args{
		"Build":             build,
		"BuildFeedbackLink": makeFeedbackLink(c, build),
	})
	return nil
}

// makeFeedbackLink attempts to create the feedback link for the build page. If the
// project is not configured for a custom feedback link or an interpolation placeholder
// cannot be satisfied an empty string is returned.
func makeFeedbackLink(c *router.Context, build *ui.MiloBuildLegacy) string {
	project, err := common.GetProject(c.Context, c.Params.ByName("project"))
	if err != nil || reflect.DeepEqual(project.BuildBugTemplate, config.BugTemplate{}) {
		return ""
	}

	buildURL := c.Request.URL
	var builderURL *url.URL
	if build.Summary.ParentLabel != nil && build.Summary.ParentLabel.URL != "" {
		builderURL, err = buildURL.Parse(build.Summary.ParentLabel.URL)
		if err != nil {
			logging.WithError(err).Errorf(c.Context, "Unable to parse build.Summary.ParentLabel.URL for custom feedback link")
			return ""
		}
	}

	link, err := MakeFeedbackLink(&project.BuildBugTemplate, map[string]interface{}{
		"Build":          makeBuild(c.Params, build),
		"MiloBuildUrl":   buildURL,
		"MiloBuilderUrl": builderURL,
	})

	if err != nil {
		logging.WithError(err).Errorf(c.Context, "Unable to make custom feedback link")
		return ""
	}

	return link
}

// makeBuild partially populates a buildbucketpb.Build. Currently it attempts to
// make available .Builder.Project, .Builder.Bucket, and .Builder.Builder.
func makeBuild(params httprouter.Params, build *ui.MiloBuildLegacy) *buildbucketpb.Build {
	return &buildbucketpb.Build{
		Builder: &buildbucketpb.BuilderID{
			Project: build.Trigger.Project,           // equivalent params.ByName("project")
			Bucket:  params.ByName("bucket"),         // way to get from ui.MiloBuildLegacy so don't need params here?
			Builder: build.Summary.ParentLabel.Label, // params.ByName("builder")
		},
	}
}
