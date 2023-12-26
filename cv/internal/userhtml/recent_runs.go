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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/router"
	"go.chromium.org/luci/server/templates"

	"go.chromium.org/luci/cv/internal/acls"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/rpc/pagination"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/run/runquery"
)

func recentsPage(c *router.Context) {
	project := c.Params.ByName("Project")
	params, err := parseFormParams(c)
	if err != nil {
		errPage(c, err)
		return
	}

	runs, next, err := searchRuns(c.Request.Context(), project, params)
	if err != nil {
		errPage(c, err)
		return
	}
	if params.mode != "" {
		var filteredRuns []*run.Run
		for _, r := range runs {
			if r.Mode == params.mode {
				filteredRuns = append(filteredRuns, r)
			}
		}
		runs = filteredRuns
	}
	runsWithCLs, err := resolveRunsCLs(c.Request.Context(), runs)
	if err != nil {
		errPage(c, err)
		return
	}

	templates.MustRender(c.Request.Context(), c.Writer, "pages/recent_runs.html", map[string]any{
		"Runs":         runsWithCLs,
		"Project":      project,
		"NextPage":     next,
		"FilterStatus": params.statusString(),
		"FilterMode":   params.modeString(),
		"Now":          startTime(c.Request.Context()),
	})
}

type recentRunsParams struct {
	status          run.Status
	mode            run.Mode
	pageTokenString string
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

	if m := run.Mode(c.Request.Form.Get("mode")); m != "" {
		params.mode = m
	}

	params.pageTokenString = strings.TrimSpace(c.Request.Form.Get("page"))
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

func searchRuns(ctx context.Context, project string, params recentRunsParams) (runs []*run.Run, next string, err error) {
	var pageToken *runquery.PageToken
	if params.pageTokenString != "" {
		pageToken = &runquery.PageToken{}
		if err = pagination.DecryptPageToken(ctx, params.pageTokenString, pageToken); err != nil {
			// Log but don't return to the user entire error to avoid any accidental
			// leakage.
			logging.Warningf(ctx, "bad page token: %s", err)
			return nil, "", appstatus.Errorf(codes.InvalidArgument, "bad page token")
		}
	}

	var qb interface {
		LoadRuns(context.Context, ...run.LoadRunChecker) ([]*run.Run, *runquery.PageToken, error)
	}
	if project == "" {
		qb = runquery.RecentQueryBuilder{
			Limit:              50,
			CheckProjectAccess: acls.CheckProjectAccess,
			Status:             params.status,
		}.PageToken(pageToken)
	} else {
		switch ok, err := acls.CheckProjectAccess(ctx, project); {
		case err != nil:
			return nil, "", err
		case !ok:
			// Return NotFound error in the case of access denied.
			//
			// Rationale: the caller shouldn't be able to distinguish between
			// project not existing and not having access to the project, because
			// it may leak the existence of the project.
			return nil, "", appstatus.Errorf(codes.NotFound, "Project %q not found", project)
		}
		qb = runquery.ProjectQueryBuilder{
			Project: project,
			Limit:   50,
			Status:  params.status,
		}.PageToken(pageToken)
	}

	var nextPageToken *runquery.PageToken
	runs, nextPageToken, err = qb.LoadRuns(ctx, acls.NewRunReadChecker())
	if err != nil {
		return
	}
	logging.Debugf(ctx, "%d runs retrieved", len(runs))

	next, err = pagination.EncryptPageToken(ctx, nextPageToken)
	if err != nil {
		return nil, "", err
	}
	return runs, next, nil
}

func resolveRunsCLs(ctx context.Context, runs []*run.Run) ([]runWithExternalCLs, error) {
	cls := make(map[common.CLID]*changelist.CL, len(runs))
	for _, r := range runs {
		for _, clid := range r.CLs {
			if cls[clid] == nil {
				cls[clid] = &changelist.CL{ID: clid}
			}
		}
	}
	if _, err := changelist.LoadCLsMap(ctx, cls); err != nil {
		return nil, err
	}

	out := make([]runWithExternalCLs, len(runs))
	for i, r := range runs {
		out[i].Run = r
		out[i].ExternalCLs = make([]changelist.ExternalID, len(r.CLs))
		for j, clid := range r.CLs {
			out[i].ExternalCLs[j] = cls[clid].ExternalID
		}
	}
	return out, nil
}

type runWithExternalCLs struct {
	*run.Run
	ExternalCLs []changelist.ExternalID
}
