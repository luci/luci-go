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
	"net/http"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/rpc/admin"
	adminpb "go.chromium.org/luci/cv/internal/rpc/admin/api"
)

func recentsPage(c *router.Context) {
	project := c.Params.ByName("Project")

	if project == "" {
		http.Redirect(c.Writer, c.Request, c.HandlerPath+"/infra", http.StatusFound)
		return
	}

	if err := c.Request.ParseForm(); err != nil {
		errPage(c, errors.Annotate(err, "failed to parse form").Err())
		return
	}
	pageToken := c.Request.Form.Get("page")

	// TODO(crbug/1233963): check if user has permission to search this specific project.

	adminServer := &admin.AdminServer{}
	resp, err := adminServer.SearchRuns(c.Context, &adminpb.SearchRunsRequest{Project: project, PageToken: pageToken})
	if err != nil {
		errPage(c, err)
		return
	}
	logging.Debugf(c.Context, "%d runs retrieved", len(resp.Runs))

	prev, next, err := pageTokens(c.Context, "recent-runs", pageToken, resp.NextPageToken)
	if err != nil {
		if common.IsDev(c.Context) {
			logging.Warningf(c.Context, "Could not connect to Redis, paging back disabled. %s", err.Error())
			next = resp.NextPageToken
		} else {
			errPage(c, err)
			return
		}
	}

	templates.MustRender(c.Context, c.Writer, "pages/recent_runs.html", map[string]interface{}{
		"Runs":    resp.Runs,
		"Prev":    prev,
		"Next":    next,
		"Project": project,
	})
}

// pageTokens caches the current pageToken associated to the next,
// so as to populate the previous page link when rendering the next page.
func pageTokens(ctx context.Context, namespace, pageToken, nextPageToken string) (prev, next string, err error) {
	if strings.Contains(namespace, "/") {
		panic(fmt.Errorf("namespace %q must not contain slashes", namespace))
	}

	key := func(token string) string {
		return fmt.Sprintf("%s/%s", namespace, token)
	}

	next = nextPageToken
	conn, err := redisconn.Get(ctx)
	if err != nil {
		err = errors.Annotate(err, "failed to connect to Redis").Err()
		return
	}
	defer conn.Close()

	var val interface{}
	if pageToken != "" {
		val, err = conn.Do("GET", key(pageToken))
		if err != nil {
			err = errors.Annotate(err, "failed to GET previous page token").Err()
			return
		}
		if val != nil {
			switch v := val.(type) {
			case string:
				// TODO(robertocn): delete if this case is never useful.
				prev = v
			case []byte:
				prev = string(v)
			default:
				err = errors.Annotate(err, "failed to decode previous page token: %v", val).Err()
				return
			}
			if prev == "NULL" {
				// Hack to get the second page to correctly show a previous
				// page link with no page token.
				prev = " "
			}
		}
	}
	if next != "" {
		if pageToken == "" {
			pageToken = "NULL"
		}
		// Keep the previous token for 24 hours.
		_, err = conn.Do("SET", key(next), pageToken, "EX", 24*60*60)
		if err != nil {
			err = errors.Annotate(err, "failed to SET current page token").Err()
			return
		}
	}
	return
}
