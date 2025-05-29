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
	"fmt"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	gerritutil "go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/configs/srvcfg"
	"go.chromium.org/luci/cv/internal/gerrit"
)

const (
	// fullPollInterval is between querying Gerrit for all changes relevant to CV as
	// if from scratch.
	fullPollInterval = 30 * time.Minute

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
	// A value that's too low here will first affect full polls, since they have
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

// doOneQuery queries Gerrit and updates the query's state.
func (p *Poller) doOneQuery(ctx context.Context, luciProject string, qs *QueryState) error {
	q := singleQuery{
		luciProject: luciProject,
		qs:          qs,
	}
	var err error
	if q.client, err = p.gFactory.MakeClient(ctx, qs.GetHost(), luciProject); err != nil {
		return err
	}

	// Time to trigger a full-poll?
	now, lastFull := clock.Now(ctx), qs.GetLastFullTime()
	if lastFull == nil || now.After(lastFull.AsTime().Add(fullPollInterval)) {
		return p.doFullQuery(ctx, q)
	}

	// If pub/sub is enabled for the project, skip incremental-poll.
	switch yes, err := srvcfg.IsProjectEnabledInListener(ctx, luciProject); {
	case err != nil:
		return errors.Fmt("srvcfg.IsProjectEnabledInListener: %w", err)
	case yes:
		return nil
	}

	return p.doIncrementalQuery(ctx, q)
}

func (p *Poller) doFullQuery(ctx context.Context, q singleQuery) error {
	ctx = logging.SetField(ctx, "poll", "full")
	started := clock.Now(ctx)
	after := started.Add(-common.MaxTriggerAge)
	changes, err := q.fetch(ctx, after, q.qs.gerritString(queryLimited))
	// There can be partial result even if err != nil.
	switch err2 := p.notifyOnMatchedCLs(ctx, q.luciProject, q.qs.GetHost(), changes, true, changelist.UpdateCLTask_FULL_POLL_MATCHED); {
	case err != nil:
		return err
	case err2 != nil:
		return err2
	}

	cur := uniqueSortedIDsOf(changes)
	if diff := common.DifferenceSorted(q.qs.Changes, cur); len(diff) != 0 {
		// `diff` changes are no longer matching the limited query,
		// so they were probably updated since.
		if err := p.notifyOnUnmatchedCLs(ctx, q.luciProject, q.qs.GetHost(), diff, changelist.UpdateCLTask_FULL_POLL_UNMATCHED); err != nil {
			return err
		}
	}

	q.qs.Changes = cur
	q.qs.LastFullTime = timestamppb.New(started)
	q.qs.LastIncrTime = nil
	return nil
}

func (p *Poller) doIncrementalQuery(ctx context.Context, q singleQuery) error {
	ctx = logging.SetField(ctx, "poll", "incremental")
	started := clock.Now(ctx)

	lastInc := q.qs.GetLastIncrTime()
	if lastInc == nil {
		if lastInc = q.qs.GetLastFullTime(); lastInc == nil {
			return errors.New("must have been a full poll")
		}
	}
	after := lastInc.AsTime().Add(-incrementalPollOverlap)
	// Unlike the full poll, query for all changes regardless of status or CQ
	// vote. This ensures that CV notices quickly when previously NEW & CQ-ed
	// change has either CQ vote removed OR status changed (e.g. submitted or
	// abandoned).
	changes, err := q.fetch(ctx, after, q.qs.gerritString(queryAll))
	// There can be partial result even if err != nil.
	switch err2 := p.notifyOnMatchedCLs(ctx, q.luciProject, q.qs.GetHost(), changes, false, changelist.UpdateCLTask_INCR_POLL_MATCHED); {
	case err != nil:
		return err
	case err2 != nil:
		return err2
	}

	q.qs.Changes = common.UnionSorted(q.qs.Changes, uniqueSortedIDsOf(changes))
	q.qs.LastIncrTime = timestamppb.New(started)
	return nil
}

type singleQuery struct {
	luciProject string
	qs          *QueryState
	client      gerrit.Client
}

func (q singleQuery) fetch(ctx context.Context, after time.Time, query string) ([]*gerritpb.ChangeInfo, error) {
	opts := gerritutil.PagingListChangesOptions{
		Limit:                  changesPerPoll,
		PageSize:               pageSize,
		MoreChangesTrustFactor: moreChangesTrustFactor,
		UpdatedAfter:           after,
	}
	req := gerritpb.ListChangesRequest{
		Options: []gerritpb.QueryOption{
			gerritpb.QueryOption_SKIP_MERGEABLE,
		},
		Query: query,
	}
	resp, err := gerritutil.PagingListChanges(ctx, q.client, &req, opts)
	grpcStatus, _ := status.FromError(errors.Unwrap(err))
	switch grpcCode := grpcStatus.Code(); {
	case grpcCode == codes.OK:
		if resp.GetMoreChanges() {
			logging.Errorf(ctx, "Ignoring oldest changes because reached max (%d) allowed to process per poll", changesPerPoll)
		}
		return resp.GetChanges(), nil
	case grpcCode == codes.InvalidArgument && strings.Contains(grpcStatus.Message(), "Invalid authentication credentials. Please generate a new identifier:"):
		logging.Errorf(ctx, "crbug/1286454: got invalid authentication credential"+
			" error when paging changes. Mark it as transient so that it will be"+
			" retried.")
		return nil, transient.Tag.Apply(err)
	// TODO(tandrii): handle 403 and 404 if CV lacks access to entire host.
	default:
		// NOTE: resp may be set if there was partial success in fetching changes
		// followed by a typically transient error.
		return resp.GetChanges(), gerrit.UnhandledError(ctx, err, "PagingListChanges failed")
	}
}

type queryKind int

const (
	queryLimited queryKind = iota
	queryAll
)

// gerritString encodes query for Gerrit.
//
// If queryLimited, unlike queryAll, searches for NEW CLs with CQ vote.
func (qs *QueryState) gerritString(kind queryKind) string {
	buf := strings.Builder{}
	switch kind {
	case queryLimited:
		buf.WriteString("status:NEW ")
		// TODO(tandrii): make label optional to support Tricium use-case.
		buf.WriteString("label:Commit-Queue>0 ")
	case queryAll:
	default:
		panic(fmt.Errorf("unknown queryKind %d", kind))
	}
	// TODO(crbug/1163177): specify `branch:` search term to restrict search to
	// specific refs. For projects watching a single ref, this will provide more
	// targeted queries, reducing load on CV & Gerrit, but care must be taken to
	// to avoid excessive number of queries when multiple refs are watched.

	emitProjectValue := func(p string) {
		// Even though it appears to work without, Gerrit doc says project names
		// containing / must be surrounded by "" or {}:
		// https://gerrit-review.googlesource.com/Documentation/user-search.html#_argument_quoting
		buf.WriteRune('"')
		buf.WriteString(p)
		buf.WriteRune('"')
	}

	// One of .OrProjects or .CommonProjectPrefix must be set.
	switch prs := qs.GetOrProjects(); len(prs) {
	case 0:
		if qs.GetCommonProjectPrefix() == "" {
			panic("partitionConfig function should have ensured this")
		}
		// project*s* means find matching projects by prefix
		buf.WriteString("projects:")
		emitProjectValue(qs.GetCommonProjectPrefix())
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

func uniqueSortedIDsOf(changes []*gerritpb.ChangeInfo) []int64 {
	if len(changes) == 0 {
		return nil
	}

	out := make([]int64, len(changes))
	for i, c := range changes {
		out[i] = c.GetNumber()
	}
	return common.UniqueSorted(out)
}
