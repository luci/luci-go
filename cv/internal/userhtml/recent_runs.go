// Copyright 2021 The LUCI Authors.
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

package userhtml

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/caching/layered"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/cv/internal/rpc/admin"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"
	"go.chromium.org/luci/cv/internal/run"
)

func recentsPage(c *router.Context) {
	project := c.Params.ByName("Project")

	params, err := parseFormParams(c)
	if err != nil {
		errPage(c, err)
		return
	}

	// TODO(crbug/1233963): check if user has permission to search this specific project.
	req := &adminpb.SearchRunsRequest{
		Project:   project,
		PageToken: params.pageToken,
		PageSize:  50,
		Status:    params.status,
		Mode:      string(params.mode),
	}

	adminServer := &admin.AdminServer{}
	resp, err := adminServer.SearchRuns(c.Context, req)
	if err != nil {
		errPage(c, err)
		return
	}
	logging.Debugf(c.Context, "%d runs retrieved", len(resp.Runs))

	prev, err := pageTokens(c.Context, params.pageToken, resp.NextPageToken)
	if err != nil {
		errPage(c, err)
		return
	}

	templates.MustRender(c.Context, c.Writer, "pages/recent_runs.html", map[string]interface{}{
		"Runs":         resp.Runs,
		"Project":      project,
		"PrevPage":     prev,
		"NextPage":     resp.NextPageToken,
		"FilterStatus": params.statusString(),
		"FilterMode":   params.modeString(),
	})
}

type recentRunsParams struct {
	status    run.Status
	mode      run.Mode
	pageToken string
}

func parseFormParams(c *router.Context) (recentRunsParams, error) {
	params := recentRunsParams{}
	if err := c.Request.ParseForm(); err != nil {
		return params, errors.Annotate(err, "failed to parse form").Err()
	}

	s := c.Request.Form.Get("status")
	switch val, ok := run.Status_value[strings.ToUpper(s)]; {
	case s == "":
	case ok:
		params.status = run.Status(val)
	default:
		return params, fmt.Errorf("invalid Run status %q", s)
	}

	switch m := run.Mode(c.Request.Form.Get("mode")); {
	case m == "":
	case m.Valid():
		params.mode = m
	default:
		return params, fmt.Errorf("invalid Run mode %q", params.mode)
	}

	params.pageToken = strings.TrimSpace(c.Request.Form.Get("page"))
	return params, nil
}

func (r *recentRunsParams) statusString() string {
	if r.status == run.Status_STATUS_UNSPECIFIED {
		return ""
	}
	return r.status.String()
}

func (r *recentRunsParams) modeString() string {
	return string(r.mode)
}

var tokenCache = layered.Cache{
	ProcessLRUCache: caching.RegisterLRUCache(1024),
	GlobalNamespace: "recent_cv_runs_page_token_cache",
	Marshal: func(item interface{}) ([]byte, error) {
		return []byte(item.(string)), nil
	},
	Unmarshal: func(blob []byte) (interface{}, error) {
		return string(blob), nil
	},
}

var tokenExp = 24 * time.Hour

// pageTokens caches the current pageToken associated to the next,
// so as to populate the previous page link when rendering the next page.
// Also returns a previously saved page token pointing to the previous page.
func pageTokens(ctx context.Context, pageToken, nextPageToken string) (prev string, err error) {

	// A whitespace will cause a 'Previous' link to render with no page token.
	// i.e. going to the first page.
	// An empty string will not render any 'Previous' link.

	// TODO(crbug.com/1249253): Consider redirecting to the first page of the
	// query when the given page token is valid, but we can't retrieve its
	// previous page.
	blankToken := " "
	var cachedV interface{}

	if pageToken == "" {
		pageToken = blankToken
	} else {
		cachedV, err = tokenCache.GetOrCreate(ctx, pageToken, func() (v interface{}, exp time.Duration, err error) {
			// We haven't seen this token yet, we don't know what its previous page is.
			return "", 0, nil
		})
		if err != nil {
			return
		}
		prev = cachedV.(string)
	}
	_, err = tokenCache.GetOrCreate(ctx, nextPageToken, func() (v interface{}, exp time.Duration, err error) {
		return pageToken, tokenExp, nil
	})
	return
}
