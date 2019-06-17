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
	"strconv"
	"strings"

	"go.chromium.org/luci/buildbucket/deprecated"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/milo/frontend/ui"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"
)

// handleDevBuild renders a canned build for development.
// TODO(hinoka): Use a mock buildbucket rpc client instead.
func handleDevBuild(c *router.Context) error {
	name := c.Params.ByName("name")
	b, err := buildbucket.GetTestBuild(c.Context, "../../buildsource/buildbucket", name)
	if err != nil {
		return err
	}
	bp := ui.NewBuildPage(c.Context, b)
	bp.BuildbucketHost = "example.com"
	return renderBuild(c, bp, nil)
}

// handleLUCIBuild renders a LUCI build.
func handleLUCIBuild(c *router.Context) error {
	bid := &buildbucketpb.BuilderID{
		Project: c.Params.ByName("project"),
		Bucket:  c.Params.ByName("bucket"),
		Builder: c.Params.ByName("builder"),
	}
	numberOrID := c.Params.ByName("numberOrId")
	forceBlamelist := c.Request.FormValue("blamelist") != ""

	// Redirect to short bucket names.
	if _, v2Bucket := deprecated.BucketNameToV2(bid.Bucket); v2Bucket != "" {
		// Parameter "bucket" is v1, e.g. "luci.chromium.try".
		u := *c.Request.URL
		u.Path = fmt.Sprintf("/p/%s/builders/%s/%s/%s", bid.Project, v2Bucket, bid.Builder, numberOrID)
		http.Redirect(c.Writer, c.Request, u.String(), http.StatusMovedPermanently)
	}

	br := buildbucketpb.GetBuildRequest{}
	if strings.HasPrefix(numberOrID, "b") {
		id, err := strconv.ParseInt(numberOrID[1:], 10, 64)
		if err != nil {
			return errors.Annotate(err, "bad build id").Tag(grpcutil.InvalidArgumentTag).Err()
		}
		br.Id = int64(id)
	} else {
		number, err := strconv.Atoi(numberOrID)
		if err != nil {
			return errors.Annotate(err, "bad build number").Tag(grpcutil.InvalidArgumentTag).Err()
		}
		br.Builder = bid
		br.BuildNumber = int32(number)
	}

	bp, err := buildbucket.GetBuildPage(c, br, forceBlamelist)
	return renderBuild(c, bp, err)
}

// renderBuild is a shortcut for rendering build or returning err if it is not nil.
func renderBuild(c *router.Context, bp *ui.BuildPage, err error) error {
	if err != nil {
		return err
	}

	bp.StepDisplayPref = getStepDisplayPrefCookie(c)
	bp.ShowDebugLogsPref = getShowDebugLogsPrefCookie(c)

	templates.MustRender(c.Context, c.Writer, "pages/build.html", templates.Args{
		"BuildPage": bp,
	})
	return nil
}

// redirectLUCIBuild redirects to a canonical build URL
// e.g. to /p/{project}/builders/{bucket}/{builder}/{number or id}.
func redirectLUCIBuild(c *router.Context) error {
	idStr := c.Params.ByName("id")
	// Verify it is an int64.
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		return errors.Annotate(err, "invalid id").Tag(grpcutil.InvalidArgumentTag).Err()
	}
	builder, number, err := buildbucket.GetBuilderID(c.Context, id)
	if err != nil {
		return err
	}
	numberOrID := fmt.Sprintf("%d", number)
	if number == 0 {
		numberOrID = fmt.Sprintf("b%d", id)
	}

	u := fmt.Sprintf("/p/%s/builders/%s/%s/%s", builder.Project, builder.Bucket, builder.Builder, numberOrID)
	http.Redirect(c.Writer, c.Request, u, http.StatusMovedPermanently)
	return nil
}

func getStepDisplayPrefCookie(c *router.Context) ui.StepDisplayPref {
	switch cookie, err := c.Request.Cookie("stepDisplayPref"); err {
	case nil:
		return ui.StepDisplayPref(cookie.Value)
	case http.ErrNoCookie:
		return ui.StepDisplayDefault
	default:
		logging.WithError(err).Errorf(c.Context, "failed to read stepDisplayPref cookie")
		return ui.StepDisplayDefault
	}
}

func getShowDebugLogsPrefCookie(c *router.Context) bool {
	switch cookie, err := c.Request.Cookie("showDebugLogsPref"); err {
	case nil:
		return cookie.Value == "true"
	case http.ErrNoCookie:
		return false
	default:
		logging.WithError(err).Errorf(c.Context, "failed to read showDebugLogsPref cookie")
		return false
	}
}
