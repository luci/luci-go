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

package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/julienschmidt/httprouter"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/logdog/common/types"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/milo/frontend/handlers/ui"
	"go.chromium.org/luci/milo/internal/buildsource/rawpresentation"
	"go.chromium.org/luci/milo/internal/buildsource/swarming"
)

func handleSwarmingBuild(c *router.Context) error {
	host := c.Request.FormValue("server")
	taskID := c.Params.ByName("id")

	// Redirect to build page if possible.
	switch buildID, ldURL, err := swarming.RedirectsFromTask(c.Request.Context(), host, taskID); {
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

	build, err := swarming.GetBuild(c.Request.Context(), host, taskID)
	return renderBuildLegacy(c, build, false, err)
}

func handleRawPresentationBuild(c *router.Context) error {
	legacyBuild, build, err := rawpresentation.GetBuild(
		c.Request.Context(),
		c.Params.ByName("logdog_host"),
		c.Params.ByName("project"),
		types.StreamPath(strings.Trim(c.Params.ByName("path"), "/")))
	if build != nil {
		return renderBuild(c, build, false, err)
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
	build.Fix(c.Request.Context())

	templates.MustRender(c.Request.Context(), c.Writer, "pages/build_legacy.html", templates.Args{
		"Build": build,
	})
	return nil
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
