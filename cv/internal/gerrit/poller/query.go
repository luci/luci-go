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
	"google.golang.org/protobuf/types/known/timestamppb"

	gerritutil "go.chromium.org/luci/common/api/gerrit"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/updater"
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

// subpoll queries Gerrit and updates the state of individual SubPoller.
func (p *Poller) subpoll(ctx context.Context, luciProject string, sp *SubPoller) error {
	q := singleQuery{
		luciProject: luciProject,
		sp:          sp,
		p:           p,
	}
	var err error
	if q.client, err = p.gFactory(ctx, sp.GetHost(), luciProject); err != nil {
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
	p           *Poller
	client      gerrit.Client
}

func (q *singleQuery) full(ctx context.Context) error {
	ctx = logging.SetField(ctx, "poll", "full")
	started := clock.Now(ctx)
	after := started.Add(-common.MaxTriggerAge)
	changes, err := q.fetch(ctx, after, buildQuery(q.sp, queryLimited))
	// There can be partial result even if err != nil.
	switch err2 := q.scheduleTasks(ctx, changes, true); {
	case err != nil:
		return err
	case err2 != nil:
		return err2
	}

	cur := uniqueSortedIDsOf(changes)
	if diff := common.DifferenceSorted(q.sp.Changes, cur); len(diff) != 0 {
		// `diff` changes are no longer matching the limited query,
		// so they were probably updated since.
		if err := q.p.scheduleRefreshTasks(ctx, q.luciProject, q.sp.GetHost(), diff); err != nil {
			return err
		}
	}

	q.sp.Changes = cur
	q.sp.LastFullTime = timestamppb.New(started)
	q.sp.LastIncrTime = nil
	return nil
}

func (q *singleQuery) incremental(ctx context.Context) error {
	ctx = logging.SetField(ctx, "poll", "incremental")
	started := clock.Now(ctx)

	lastInc := q.sp.GetLastIncrTime()
	if lastInc == nil {
		if lastInc = q.sp.GetLastFullTime(); lastInc == nil {
			panic("must have been a full poll")
		}
	}
	after := lastInc.AsTime().Add(-incrementalPollOverlap)
	// Unlike the full poll, query for all changes regardless of status or CQ
	// vote. This ensures that CV notices quickly when previously NEW & CQ-ed
	// change has either CQ vote removed OR status changed (e.g. submitted or
	// abandoned).
	changes, err := q.fetch(ctx, after, buildQuery(q.sp, queryAll))
	// There can be partial result even if err != nil.
	switch err2 := q.scheduleTasks(ctx, changes, false); {
	case err != nil:
		return err
	case err2 != nil:
		return err2
	}

	q.sp.Changes = common.UnionSorted(q.sp.Changes, uniqueSortedIDsOf(changes))
	q.sp.LastIncrTime = timestamppb.New(started)
	return nil
}

func (q *singleQuery) fetch(ctx context.Context, after time.Time, query string) ([]*gerritpb.ChangeInfo, error) {
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

// maxLoadCLBatchSize limits how many CL entities are loaded at once for
// notifying PM.
const maxLoadCLBatchSize = 100

func (q *singleQuery) scheduleTasks(ctx context.Context, changes []*gerritpb.ChangeInfo, forceNotifyPM bool) error {
	// TODO(tandrii): optimize by checking if CV is interested in the
	// (host,project,ref) of these changes from before triggering tasks.
	logging.Debugf(ctx, "scheduling %d CLUpdate tasks", len(changes))

	var clids []common.CLID
	if forceNotifyPM {
		// Objective: make PM aware of all CLs.
		// Optimization: avoid RefreshGerritCL with forceNotifyPM=true if possible,
		// as this removes ability to de-duplicate.
		var err error
		eids := make([]changelist.ExternalID, len(changes))
		for i, c := range changes {
			if eids[i], err = changelist.GobID(q.sp.GetHost(), c.GetNumber()); err != nil {
				return err
			}
		}
		clids, err = changelist.Lookup(ctx, eids)
		if err != nil {
			return err
		}
		if err := q.p.notifyPMifKnown(ctx, q.luciProject, clids, maxLoadCLBatchSize); err != nil {
			return err
		}
	}

	errs := parallel.WorkPool(min(10, len(changes)), func(work chan<- func() error) {
		for i, c := range changes {
			payload := &updater.RefreshGerritCL{
				LuciProject: q.luciProject,
				Host:        q.sp.GetHost(),
				Change:      c.GetNumber(),
				UpdatedHint: c.GetUpdated(),
				ForceNotify: forceNotifyPM && (clids[i] == 0),
			}
			work <- func() error {
				return q.p.clUpdater.Schedule(ctx, payload)
			}
		}
	})
	return common.MostSevereError(errs)
}

func (p *Poller) scheduleRefreshTasks(ctx context.Context, luciProject, host string, changes []int64) error {
	logging.Debugf(ctx, "scheduling %d CLUpdate tasks for no longer matched CLs", len(changes))
	var err error
	eids := make([]changelist.ExternalID, len(changes))
	for i, c := range changes {
		if eids[i], err = changelist.GobID(host, c); err != nil {
			return err
		}
	}
	// Objective: make PM aware of all CLs.
	// Optimization: avoid RefreshGerritCL with forceNotifyPM=true if possible,
	// as this removes ability to de-duplicate.
	// Since all no longer matched CLs are typically already stored in the
	// Datastore, get their internal CL IDs and notify PM directly.
	clids, err := changelist.Lookup(ctx, eids)
	if err != nil {
		return err
	}

	if err := p.notifyPMifKnown(ctx, luciProject, clids, maxLoadCLBatchSize); err != nil {
		return err
	}

	errs := parallel.WorkPool(min(10, len(changes)), func(work chan<- func() error) {
		for i, c := range changes {
			payload := &updater.RefreshGerritCL{
				LuciProject: luciProject,
				Host:        host,
				Change:      c,
				ForceNotify: 0 == clids[i], // notify iff CL ID isn't yet known.
			}
			// Distribute these tasks in time to avoid high peaks (e.g. see
			// https://crbug.com/1211057).
			delay := (fullPollInterval * time.Duration(i)) / time.Duration(len(clids))
			work <- func() error {
				return p.clUpdater.ScheduleDelayed(ctx, payload, delay)
			}
		}
	})
	return common.MostSevereError(errs)
}

// notifyPMifKnown notifies PM to update its CLs for each non-0 CLID.
//
// Obtains EVersion of each CL before notify a PM. Unfortunately, this loads a
// lot of information we don't need, such as Snapshot. So, loads CLs in batches
// to avoid large memory footprint.
//
// In practice, most of these CLs would be already dscache-ed, so loading them
// is fast.
func (p *Poller) notifyPMifKnown(ctx context.Context, luciProject string, clids []common.CLID, maxBatchSize int) error {
	cls := make([]*changelist.CL, 0, maxBatchSize)
	flush := func() error {
		if err := datastore.Get(ctx, cls); err != nil {
			return errors.Annotate(common.MostSevereError(err), "failed to load CLs").Tag(transient.Tag).Err()
		}
		return p.pm.NotifyCLsUpdated(ctx, luciProject, cls)
	}

	for _, clid := range clids {
		switch l := len(cls); {
		case clid == 0:
		case l == maxBatchSize:
			if err := flush(); err != nil {
				return err
			}
			cls = cls[:0]
		default:
			cls = append(cls, &changelist.CL{ID: clid})
		}
	}
	if len(cls) > 0 {
		return flush()
	}
	return nil
}

type queryKind int

const (
	queryLimited queryKind = iota
	queryAll
)

// buildQuery returns query string.
//
// If queryLimited, unlike queryAll, searches for NEW CLs with CQ vote.
func buildQuery(sp *SubPoller, kind queryKind) string {
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

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}
