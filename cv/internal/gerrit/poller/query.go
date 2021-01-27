// Copyright 2020 The LUCI Authors.
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

package poller

import (
	"context"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	gerritutil "go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/updater"
)

const (
	// maxChangeAge limits search only to changes updated at most this long ago.
	maxChangeAge = 24 * 7 * time.Hour

	// fullPollInterval is between querying Gerrit for all changes relevant to CV as
	// if from scratch.
	fullPollInterval = time.Hour

	// incrementalPollOverlap is safety overlap of time range of Change.Updated
	// between two successive polls.
	//
	// While this doesn't guarantee that CV won't miss changes in-between
	// incremental polls, it should mitigate the most common reasons:
	//   * time skew between CV and Gerrit clocks,
	//   * hopping between potentially out of sync Gerrit mirrors.
	incrementalPollOverlap = time.Minute

	// changesPerPoll is how many changes CV will process per poll.
	//
	// A value that's too low here will first affects full polls, since they have
	// to (re-)process all interesting changes watched by a LUCI project.
	//
	// 10k is OK to fetch sequentially and keep in RAM without OOM-ing,
	// and currently enough for each of the LUCI projects.
	//
	// Higher values may need smarter full polling techniques.
	changesPerPoll = 10000

	// pageSize is how many changes to request in a single ListChangesRequest.
	pageSize = 1000

	// moreChangesTrustFactor controls when CV must not trust false value of
	// ListChangesResponse.MoreChanges.
	//
	// Value of 0.5 combined with pageSize of 1000 means that CV will trust
	// MoreChanges iff Gerrit returns <= 500 CLs.
	//
	// For more info, see corresponding field in
	// https://godoc.org/go.chromium.org/luci/common/api/gerrit#PagingListChangesOptions
	moreChangesTrustFactor = 0.5
)

// subpoll queries Gerrit and updates the state of individual SubPoller.
func subpoll(ctx context.Context, luciProject string, sp *SubPoller) error {
	q := singleQuery{
		luciProject: luciProject,
		sp:          sp,
		query:       buildQuery(sp),
	}
	var err error
	if q.client, err = gerrit.CurrentClient(ctx, sp.GetHost(), luciProject); err != nil {
		return err
	}
	if sp.GetLastFullTime() == nil {
		return q.full(ctx)
	}
	nextFullAt := sp.GetLastFullTime().AsTime().Add(fullPollInterval)
	if clock.Now(ctx).Before(nextFullAt) {
		return q.incremental(ctx)
	}
	return q.full(ctx)
}

type singleQuery struct {
	luciProject string
	sp          *SubPoller
	query       string
	client      gerrit.QueryClient
}

func (q *singleQuery) full(ctx context.Context) error {
	ctx = logging.SetFields(ctx, logging.Fields{
		"luciProject": q.luciProject,
		"poll":        "full",
	})
	started := clock.Now(ctx)
	after := started.Add(-maxChangeAge)
	changes, err := q.fetch(ctx, after)
	// There can be partial result even if err != nil.
	switch err2 := q.scheduleTasks(ctx, changes, true); {
	case err != nil:
		return err
	case err2 != nil:
		return err2
	}
	q.sp.LastFullTime = timestamppb.New(started)
	q.sp.LastIncrTime = nil
	return nil
}

func (q *singleQuery) incremental(ctx context.Context) error {
	ctx = logging.SetFields(ctx, logging.Fields{
		"luciProject": q.luciProject,
		"poll":        "incremental",
	})
	started := clock.Now(ctx)

	lastInc := q.sp.GetLastIncrTime()
	if lastInc == nil {
		if lastInc = q.sp.GetLastFullTime(); lastInc == nil {
			panic("must have been a full poll")
		}
	}
	after := lastInc.AsTime().Add(-incrementalPollOverlap)
	changes, err := q.fetch(ctx, after)
	// There can be partial result even if err != nil.
	switch err2 := q.scheduleTasks(ctx, changes, false); {
	case err != nil:
		return err
	case err2 != nil:
		return err2
	}
	q.sp.LastIncrTime = timestamppb.New(started)
	return nil
}

func (q *singleQuery) fetch(ctx context.Context, after time.Time) ([]*gerritpb.ChangeInfo, error) {
	opts := gerritutil.PagingListChangesOptions{
		Limit:                  changesPerPoll,
		PageSize:               pageSize,
		MoreChangesTrustFactor: moreChangesTrustFactor,
		UpdatedAfter:           after,
	}
	req := gerritpb.ListChangesRequest{
		Options: []gerritpb.QueryOption{
			// TODO(https://crbug.com/gerrit/10749): remove CURRENT_REVISION.
			// CV doesn't actually need the whole current revision info, but
			// that's the only way to get actual "current_revision" hash.
			gerritpb.QueryOption_CURRENT_REVISION,
			gerritpb.QueryOption_SKIP_MERGEABLE,
		},
		Query: q.query,
	}
	resp, err := gerritutil.PagingListChanges(ctx, q.client, &req, opts)
	switch grpcutil.Code(err) {
	case codes.OK:
		if resp.GetMoreChanges() {
			logging.Errorf(ctx, "Ignoring oldest changes because reached max (%d) allowed to process per poll", changesPerPoll)
		}
		return resp.GetChanges(), nil
	// TODO(tandrii): handle 403 and 404 if CV lacks access to entire host.
	default:
		// NOTE: resp may be set if there was partial success in fetching changes
		// followed by a typically transient error.
		return resp.GetChanges(), gerrit.UnhandledError(ctx, err, "PagingListChanges failed")
	}
}

func (q *singleQuery) scheduleTasks(ctx context.Context, changes []*gerritpb.ChangeInfo, forceNotifyPM bool) error {
	// TODO(tandrii): optimize by checking if CV is interested in the
	// (host,project,ref) these changes come from before triggering tasks.
	logging.Debugf(ctx, "scheduling %d CLUpdate tasks", len(changes))
	errs := parallel.WorkPool(10, func(work chan<- func() error) {
		for _, c := range changes {
			c := c
			work <- func() error {
				payload := &updater.RefreshGerritCL{
					LuciProject:   q.luciProject,
					Host:          q.sp.GetHost(),
					Change:        c.GetNumber(),
					UpdatedHint:   c.GetUpdated(),
					ForceNotifyPm: forceNotifyPM,
				}
				return updater.Schedule(ctx, payload)
			}
		}
	})
	if errs != nil {
		return common.MostSevereError(errs.(errors.MultiError))
	}
	return nil
}

func buildQuery(sp *SubPoller) string {
	buf := strings.Builder{}
	buf.WriteString("status:NEW ")
	// TODO(tandrii): make label optional to support Tricium use-case.
	buf.WriteString("label:Commit-Queue>0 ")
	// TODO(crbug/1163177): specify `branch:` search term to restrict search to
	// specific refs. This requires changing partitioning poller into subpollers,
	// but will provide more targeted queries, reducing load on CV & Gerrit.

	emitProjectValue := func(p string) {
		// Even though it appears to work without, Gerrit doc says project names
		// containing / must be surrounded by "" or {}:
		// https://gerrit-review.googlesource.com/Documentation/user-search.html#_argument_quoting
		buf.WriteRune('"')
		buf.WriteString(p)
		buf.WriteRune('"')
	}

	// One of .OrProjects or .CommonProjectPrefix must be set.
	switch prs := sp.GetOrProjects(); len(prs) {
	case 0:
		if sp.GetCommonProjectPrefix() == "" {
			panic("partitionConfig function should have ensured this")
		}
		// project*s* means find matching projects by prefix
		buf.WriteString("projects:")
		emitProjectValue(sp.GetCommonProjectPrefix())
	case 1:
		buf.WriteString("project:")
		emitProjectValue(prs[0])
	default:
		buf.WriteRune('(')
		for i, p := range prs {
			if i > 0 {
				buf.WriteString(" OR ")
			}
			buf.WriteString("project:")
			emitProjectValue(p)
		}
		buf.WriteRune(')')
	}
	return buf.String()
}
