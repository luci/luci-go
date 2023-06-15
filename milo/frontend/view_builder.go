// Copyright 2017 The LUCI Authors.
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

	"go.chromium.org/luci/milo/internal/buildsource/buildbucket"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"
)

// BuilderHandler renders the builder page.
// Does not control access directly because this endpoint delegates all access
// control to Buildbucket with the RPC calls.
func BuilderHandler(c *router.Context) error {
	bid := &buildbucketpb.BuilderID{
		Project: c.Params.ByName("project"),
		Bucket:  c.Params.ByName("bucket"),
		Builder: c.Params.ByName("builder"),
	}
	pageSize := GetLimit(c.Request, -1)

	// TODO(iannucci): standardize to "page token" term instead of cursor.
	pageToken := c.Request.FormValue("cursor")

	// Redirect to short bucket names.
	if _, v2Bucket := bbv1.BucketNameToV2(bid.Bucket); v2Bucket != "" {
		// Parameter "bucket" is v1, e.g. "luci.chromium.try".
		u := *c.Request.URL
		u.Path = fmt.Sprintf("/p/%s/builders/%s/%s", bid.Project, v2Bucket, bid.Builder)
		http.Redirect(c.Writer, c.Request, u.String(), http.StatusMovedPermanently)
		return nil
	}

	if pageSize <= 0 {
		pageSize = 25
	}

	page, err := buildbucket.GetBuilderPage(c.Request.Context(), bid, pageSize, pageToken)
	if err != nil {
		return err
	}

	templates.MustRender(c.Request.Context(), c.Writer, "pages/builder.html", templates.Args{
		"BuilderPage": page,
		"BannerHTML":  page.NewBuilderPageOptInHTML(),
	})
	return nil
}

func getShowNewBuilderPageCookie(c *router.Context) bool {
	switch cookie, err := c.Request.Cookie("showNewBuilderPage"); err {
	case nil:
		return cookie.Value == "true"
	case http.ErrNoCookie:
		return true
	default:
		logging.WithError(err).Errorf(c.Request.Context(), "failed to read showNewBuilderPage cookie")
		return true
	}
}
