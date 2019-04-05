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

	bb "go.chromium.org/luci/buildbucket"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/milo/buildsource/buildbucket"
	"go.chromium.org/luci/milo/common"
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
	bucket := c.Params.ByName("bucket")
	buildername := c.Params.ByName("builder")
	numberOrId := c.Params.ByName("numberOrId")
	forceBlamelist := c.Request.FormValue("blamelist") != ""

	if _, v2Bucket := bb.BucketNameToV2(bucket); v2Bucket != "" {
		// Params bucket is a v1 bucket, so call the legacy endpoint.
		return handleLUCIBuildLegacy(c, bucket, buildername, numberOrId)
	}

	// TODO(hinoka): Once v2 is default, redirect v1 bucketnames to v2 bucketname URLs.
	br := buildbucketpb.GetBuildRequest{}
	if strings.HasPrefix(numberOrId, "b") {
		id, err := strconv.ParseInt(numberOrId[1:], 10, 64)
		if err != nil {
			return errors.Annotate(err, "bad build id").Tag(common.CodeParameterError).Err()
		}
		br.Id = int64(id)
	} else {
		number, err := strconv.Atoi(numberOrId)
		if err != nil {
			return errors.Annotate(err, "bad build number").Tag(common.CodeParameterError).Err()
		}
		br.BuildNumber = int32(number)
		br.Builder = &buildbucketpb.BuilderID{
			Project: c.Params.ByName("project"),
			Bucket:  bucket,
			Builder: buildername,
		}
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
		return errors.Annotate(err, "invalid id").Tag(common.CodeParameterError).Err()
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
