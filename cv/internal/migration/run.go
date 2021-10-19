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

package migration

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/gerrit/trigger"
	"go.chromium.org/luci/cv/internal/run"
)

func fetchActiveRuns(ctx context.Context, project string) ([]*migrationpb.ActiveRun, error) {
	runs, err := run.ProjectQueryBuilder{Project: project, Status: run.Status_RUNNING}.LoadRuns(ctx)
	switch {
	case err != nil:
		return nil, err
	case len(runs) == 0:
		return nil, nil
	}
	// Remove runs with corresponding VerifiedCQDRun entities or those being
	// cancelled.
	runs, err = pruneInactiveRuns(ctx, runs)
	switch {
	case err != nil:
		return nil, err
	case len(runs) == 0:
		return nil, nil
	}

	// Load all RunCLs and populate ActiveRuns concurrently, but leave FyiDeps
	// computation for later, since these can't be filled from RunCLs anyway.
	ret := make([]*migrationpb.ActiveRun, len(runs))
	err = parallel.WorkPool(min(len(runs), 32), func(workCh chan<- func() error) {
		for i, r := range runs {
			i, r := i, r
			workCh <- func() error {
				var err error
				ret[i], err = makeActiveRun(ctx, r)
				return err
			}
		}
	})
	if err != nil {
		return nil, common.MostSevereError(err)
	}

	// Finally, process all FyiDeps at once.
	cls := map[common.CLID]*changelist.CL{}
	for _, r := range ret {
		for _, d := range r.GetFyiDeps() {
			clid := common.CLID(d.GetId())
			if _, exists := cls[clid]; !exists {
				cls[clid] = &changelist.CL{ID: clid}
			}
		}
	}
	if len(cls) == 0 {
		return ret, nil
	}
	if _, err := changelist.LoadCLsMap(ctx, cls); err != nil {
		return nil, err
	}
	for _, r := range ret {
		for _, d := range r.GetFyiDeps() {
			cl := cls[common.CLID(d.GetId())]
			d.Gc = &cvbqpb.GerritChange{
				Host:                       cl.Snapshot.GetGerrit().GetHost(),
				Project:                    cl.Snapshot.GetGerrit().GetInfo().GetProject(),
				Change:                     cl.Snapshot.GetGerrit().GetInfo().GetNumber(),
				Patchset:                   int64(cl.Snapshot.GetPatchset()),
				EarliestEquivalentPatchset: int64(cl.Snapshot.GetMinEquivalentPatchset()),
			}
			d.Files = cl.Snapshot.GetGerrit().GetFiles()
			d.Info = cl.Snapshot.GetGerrit().GetInfo()
		}
	}
	return ret, nil
}

// makeActiveRun makes ActiveRun except for filling FYI deps with details,
// which is done later for all Runs in order to de-dupe common FYI deps.
// This is especially helpful for large CL stacks in single-CL Run mode.
func makeActiveRun(ctx context.Context, r *run.Run) (*migrationpb.ActiveRun, error) {
	runCLs, err := run.LoadRunCLs(ctx, r.ID, r.CLs)
	if err != nil {
		return nil, err
	}
	known := make(common.CLIDsSet, len(r.CLs))
	allDeps := common.CLIDsSet{}
	mcls := make([]*migrationpb.RunCL, len(runCLs))
	for i, cl := range runCLs {
		known.Add(cl.ID)
		trigger := &migrationpb.RunCL_Trigger{
			Email:     cl.Trigger.GetEmail(),
			Time:      cl.Trigger.GetTime(),
			AccountId: cl.Trigger.GetGerritAccountId(),
		}
		mcl := &migrationpb.RunCL{
			Id: int64(cl.ID),
			Gc: &cvbqpb.GerritChange{
				Host:                       cl.Detail.GetGerrit().GetHost(),
				Project:                    cl.Detail.GetGerrit().GetInfo().GetProject(),
				Change:                     cl.Detail.GetGerrit().GetInfo().GetNumber(),
				Patchset:                   int64(cl.Detail.GetPatchset()),
				EarliestEquivalentPatchset: int64(cl.Detail.GetMinEquivalentPatchset()),
				Mode:                       r.Mode.BQAttemptMode(),
			},
			Files:   cl.Detail.GetGerrit().GetFiles(),
			Info:    cl.Detail.GetGerrit().GetInfo(),
			Trigger: trigger,
			Deps:    make([]*migrationpb.RunCL_Dep, len(cl.Detail.GetDeps())),
		}
		for i, dep := range cl.Detail.GetDeps() {
			allDeps.AddI64(dep.GetClid())
			mcl.Deps[i] = &migrationpb.RunCL_Dep{
				Id: dep.GetClid(),
			}
			if dep.GetKind() == changelist.DepKind_HARD {
				mcl.Deps[i].Hard = true
			}
		}
		mcls[i] = mcl
	}
	var fyiDeps []*migrationpb.RunCL
	for clid := range allDeps {
		if known.Has(clid) {
			continue
		}
		fyiDeps = append(fyiDeps, &migrationpb.RunCL{Id: int64(clid)})
	}
	return &migrationpb.ActiveRun{
		Id:      string(r.ID),
		Cls:     mcls,
		FyiDeps: fyiDeps,
	}, nil
}

// VerifiedCQDRun is the Run reported by CQDaemon after verification completes.
type VerifiedCQDRun struct {
	_kind string `gae:"$kind,migration.VerifiedCQDRun"`
	// ID is ID of this Run in CV.
	ID common.RunID `gae:"$id"`
	// Payload is what CQDaemon has reported.
	Payload *migrationpb.ReportVerifiedRunRequest
	// RecordTime is when this entity was inserted.
	UpdateTime time.Time `gae:",noindex"`
}

func saveVerifiedCQDRun(ctx context.Context, req *migrationpb.ReportVerifiedRunRequest, notify func(context.Context) error) error {
	runID := common.RunID(req.GetRun().GetId())
	req.GetRun().Id = "" // will be stored as VerifiedCQDRun.ID

	try := 0
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		try++
		v := VerifiedCQDRun{ID: runID}
		switch err := datastore.Get(ctx, &v); {
		case err == datastore.ErrNoSuchEntity:
			// expected.
		case err != nil:
			return err
		default:
			// Do not overwrite existing one, since CV must be already finalizing it.
			logging.Warningf(ctx, "VerifiedCQDRun %q in %d-th try: already exists", runID, try)
			return nil
		}
		v = VerifiedCQDRun{
			ID:         runID,
			UpdateTime: datastore.RoundTime(clock.Now(ctx).UTC()),
			Payload:    req,
		}
		if err := datastore.Put(ctx, &v); err != nil {
			return err
		}
		return notify(ctx)
	}, nil)
	return errors.Annotate(err, "failed to record VerifiedCQDRun %q after %d tries", runID, try).Tag(transient.Tag).Err()
}

// pruneInactiveRuns removes Runs for which VerifiedCQDRun have already been
// written or iff the run is about to be cancelled.
//
// Modifies the Runs slice in place, but also returns it for readability.
func pruneInactiveRuns(ctx context.Context, in []*run.Run) ([]*run.Run, error) {
	out := in[:0]
	keys := make([]*datastore.Key, len(in))
	for i, r := range in {
		keys[i] = datastore.MakeKey(ctx, "migration.VerifiedCQDRun", string(r.ID))
	}
	exists, err := datastore.Exists(ctx, keys)
	if err != nil {
		return nil, errors.Annotate(err, "failed to check VerifiedCQDRun existence").Tag(transient.Tag).Err()
	}
	for i, r := range in {
		if !exists.Get(0, i) {
			out = append(out, r)
		}
	}
	return out, nil
}

// makeGerritSetReviewRequest creates request to post a message to Gerrit at
// CQDaemon's request.
func makeGerritSetReviewRequest(r *run.Run, ci *gerritpb.ChangeInfo, msg, curRevision string, sendEmail bool) *gerritpb.SetReviewRequest {
	req := &gerritpb.SetReviewRequest{
		Number:     ci.GetNumber(),
		Project:    ci.GetProject(),
		RevisionId: curRevision,
		Message:    msg,
		Tag:        "autogenerated:cq",
		Notify:     gerritpb.Notify_NOTIFY_OWNER, // by default
	}
	switch {
	case !sendEmail:
		req.Notify = gerritpb.Notify_NOTIFY_NONE
	case r.Mode == run.FullRun:
		req.Notify = gerritpb.Notify_NOTIFY_OWNER_REVIEWERS
		fallthrough
	default:
		// notify CQ label voters, too.
		// This doesn't take into account additional labels, but it's good enough
		// during the migration.
		var accounts []int64
		for _, vote := range ci.GetLabels()[trigger.CQLabelName].GetAll() {
			if vote.GetValue() != 0 {
				accounts = append(accounts, vote.GetUser().GetAccountId())
			}
		}
		req.NotifyDetails = &gerritpb.NotifyDetails{
			Recipients: []*gerritpb.NotifyDetails_Recipient{
				{
					RecipientType: gerritpb.NotifyDetails_RECIPIENT_TYPE_TO,
					Info: &gerritpb.NotifyDetails_Info{
						Accounts: accounts,
					},
				},
			},
		}
	}
	return req
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}
