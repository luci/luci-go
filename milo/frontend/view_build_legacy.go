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
	"strconv"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/julienschmidt/httprouter"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/milo/api/config"
	"go.chromium.org/luci/milo/buildsource/buildbot"
	"go.chromium.org/luci/milo/buildsource/buildbot/buildbotapi"
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
			Tag(grpcutil.InvalidArgumentTag).
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

	build, err := buildbot.GetBuild(c.Context, id)
	return renderBuildLegacy(c, build, false, err)
}

func handleSwarmingBuild(c *router.Context) error {
	host := c.Request.FormValue("server")
	taskID := c.Params.ByName("id")

	// Redirect to build page if possible.
	switch buildID, ldURL, err := swarming.RedirectsFromTask(c.Context, host, taskID); {
	case err != nil:
		return err
	case buildID != 0:
		u := *c.Request.URL
		u.Path = fmt.Sprintf("/b/%d", buildID)
		http.Redirect(c.Writer, c.Request, u.String(), http.StatusFound)
		return nil
	case ldURL != "":
		u := *c.Request.URL
		u.Path = fmt.Sprintf("/raw/build/%s", ldURL)
		http.Redirect(c.Writer, c.Request, u.String(), http.StatusFound)
		return nil
	}

	build, err := swarming.GetBuild(c.Context, host, taskID)
	return renderBuildLegacy(c, build, false, err)
}

func handleRawPresentationBuild(c *router.Context) error {
	legacyBuild, build, err := rawpresentation.GetBuild(
		c.Context,
		c.Params.ByName("logdog_host"),
		c.Params.ByName("project"),
		types.StreamPath(strings.Trim(c.Params.ByName("path"), "/")))
	if build != nil {
		return renderBuild(c, build, err)
	}
	return renderBuildLegacy(c, legacyBuild, false, err)
}

// renderBuildLegacy is a shortcut for rendering build or returning err if it is not
// nil. Also calls build.Fix().
func renderBuildLegacy(c *router.Context, build *ui.MiloBuildLegacy, renderTimeline bool, err error) error {
	if err != nil {
		return err
	}

	build.StepDisplayPref = getStepDisplayPrefCookie(c)
	build.ShowDebugLogsPref = getShowDebugLogsPrefCookie(c)
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
	if err != nil || proto.Equal(&project.BuildBugTemplate, &config.BugTemplate{}) {
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

	link, err := buildbucket.MakeBuildBugLink(&project.BuildBugTemplate, map[string]interface{}{
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
	// NOTE: on led builds, some of these params may not be populated.
	return &buildbucketpb.Build{
		Builder: &buildbucketpb.BuilderID{
			Project: params.ByName("project"), // equivalent build.Trigger.Project
			Bucket:  params.ByName("bucket"),  // way to get from ui.MiloBuildLegacy so don't need params here?
			Builder: params.ByName("builder"), // build.Summary.ParentLabel.Label
		},
	}
}
