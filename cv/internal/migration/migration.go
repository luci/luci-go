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

package migration

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	gerritpb "go.chromium.org/luci/common/proto/gerrit"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/gerrit/botdata"
	"go.chromium.org/luci/cv/internal/run"
	runImpl "go.chromium.org/luci/cv/internal/run/impl"
)

// allowGroup is a Chrome Infra Auth group, members of which are allowed to call
// migration API. It's hardcoded here because this code is temporary.
const allowGroup = "luci-cv-migration-crbug-1141880"

type MigrationServer struct {
	migrationpb.UnimplementedMigrationServer
}

// ReportRuns notifies CV of the Runs CQDaemon is currently working with.
//
// Used to determine whether CV's view of the world matches that of CQDaemon.
// Initially, this is just FYI for CV.
func (m *MigrationServer) ReportRuns(ctx context.Context, req *migrationpb.ReportRunsRequest) (resp *empty.Empty, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()
	if err = m.checkAllowed(ctx); err != nil {
		return
	}

	project := "<UNKNOWN>"
	if i := auth.CurrentIdentity(ctx); i.Kind() == identity.Project {
		project = i.Value()
	}

	cls := 0
	for _, r := range req.Runs {
		project = r.Attempt.LuciProject
		cls += len(r.Attempt.GerritChanges)
	}
	logging.Infof(ctx, "CQD[%s] is working on %d attempts %d CLs right now", project, len(req.Runs), cls)
	resp = &empty.Empty{}
	return
}

// ReportFinishedRun notifies CV of the Run CQDaemon has just finalized.
func (m *MigrationServer) ReportFinishedRun(ctx context.Context, req *migrationpb.ReportFinishedRunRequest) (resp *empty.Empty, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()
	if err = m.checkAllowed(ctx); err != nil {
		return
	}

	a := req.Run.Attempt
	logging.Infof(ctx, "CQD[%s] finished working on %s (%s) attempt with %s", a.LuciProject, a.Key, clsOf(a), a.Status.String())
	resp = &empty.Empty{}
	return
}

func (m *MigrationServer) ReportUsedNetrc(ctx context.Context, req *migrationpb.ReportUsedNetrcRequest) (resp *empty.Empty, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()
	if err = m.checkAllowed(ctx); err != nil {
		return
	}
	if req.AccessToken == "" || req.GerritHost == "" {
		err = appstatus.Error(codes.InvalidArgument, "access_token and gerrit_host required")
		return
	}

	project := "<UNKNOWN>"
	if i := auth.CurrentIdentity(ctx); i.Kind() == identity.Project {
		project = i.Value()
	}
	logging.Infof(ctx, "CQD[%s] uses netrc access token for %s", project, req.GerritHost)
	resp = &empty.Empty{}
	err = gerrit.SaveLegacyNetrcToken(ctx, req.GerritHost, req.AccessToken)
	return
}

// FetchActiveRuns returns all RUNNING runs for the given LUCI Project.
func (m *MigrationServer) FetchActiveRuns(ctx context.Context, req *migrationpb.FetchActiveRunsRequest) (resp *migrationpb.FetchActiveRunsResponse, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()
	if err = m.checkAllowed(ctx); err != nil {
		return
	}

	runs := []run.Run{}
	q := run.NewQueryWithLUCIProject(ctx, req.GetLuciProject()).Eq("Status", run.Status_RUNNING)
	err = errors.Annotate(datastore.GetAll(ctx, q, &runs), "fetch Run entities").Tag(transient.Tag).Err()
	if err != nil {
		return
	}

	resp = &migrationpb.FetchActiveRunsResponse{}
	if len(runs) == 0 {
		return
	}
	poolSize := len(runs)
	if poolSize > 20 {
		poolSize = 20
	}
	var respMu sync.Mutex
	err = parallel.WorkPool(poolSize, func(workCh chan<- func() error) {
		for _, r := range runs {
			r := r
			workCh <- func() error {
				runKey := datastore.MakeKey(ctx, run.RunKind, string(r.ID))
				runCLs := make([]runImpl.RunCL, len(r.CLs))
				for i, cl := range r.CLs {
					runCLs[i] = runImpl.RunCL{
						ID:  cl,
						Run: runKey,
					}
				}
				if err := datastore.Get(ctx, runCLs); err != nil {
					return errors.Annotate(err, "fetch CLs for run %q", r.ID).Tag(transient.Tag).Err()
				}
				mcls := make([]*migrationpb.RunCL, len(runCLs))
				mode := cvbqpb.Mode_FULL_RUN
				if r.Mode == run.DryRun {
					mode = cvbqpb.Mode_DRY_RUN
				}
				for i, cl := range runCLs {
					trigger, err := findTrigger(cl.Detail.GetGerrit().GetInfo(), r.Mode)
					if err != nil {
						return errors.Annotate(err, "find trigger for CL %d", cl.ID).Err()
					}
					mcl := &migrationpb.RunCL{
						Id: int64(cl.ID),
						Gc: &cvbqpb.GerritChange{
							Host:                       cl.Detail.GetGerrit().GetHost(),
							Project:                    cl.Detail.GetGerrit().GetInfo().GetProject(),
							Change:                     cl.Detail.GetGerrit().GetInfo().GetNumber(),
							Patchset:                   int64(cl.Detail.GetPatchset()),
							EarliestEquivalentPatchset: int64(cl.Detail.GetMinEquivalentPatchset()),
							Mode:                       mode,
						},
						UpdatedTime: cl.Detail.GetExternalUpdateTime(),
						Trigger:     trigger,
						Deps:        make([]*migrationpb.RunCL_Dep, len(cl.Detail.GetDeps())),
					}
					for i, dep := range cl.Detail.GetDeps() {
						mcl.Deps[i] = &migrationpb.RunCL_Dep{
							Id: dep.GetClid(),
						}
						if dep.GetKind() == changelist.DepKind_HARD {
							mcl.Deps[i].Hard = true
						}
					}
					mcls[i] = mcl
				}
				respMu.Lock()
				defer respMu.Unlock()
				resp.Runs = append(resp.Runs, &migrationpb.Run{
					Attempt: &cvbqpb.Attempt{
						LuciProject: req.GetLuciProject(),
					},
					Id:  string(r.ID),
					Cls: mcls,
				})
				return nil
			}
		}
	})

	if err != nil {
		resp.Runs = nil
	}
	return
}

func (m *MigrationServer) checkAllowed(ctx context.Context) error {
	i := auth.CurrentIdentity(ctx)
	if i.Kind() == identity.Project {
		// Only small list of LUCI services is allowed,
		// we can assume no malicious access, hence this is CQDaemon.
		return nil
	}
	logging.Warningf(ctx, "Unusual caller %s", i)

	switch yes, err := auth.IsMember(ctx, allowGroup); {
	case err != nil:
		return status.Errorf(codes.Internal, "failed to check ACL")
	case !yes:
		return status.Errorf(codes.PermissionDenied, "not a member of %s", allowGroup)
	default:
		return nil
	}
}

// clsOf emits CL of the Attempt (aka Run) preserving the order but avoiding
// duplicating hostnames.
func clsOf(a *cvbqpb.Attempt) string {
	if len(a.GerritChanges) == 0 {
		return "NO CLS"
	}
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "%d CLs:", len(a.GerritChanges))
	priorIdx := 0
	emit := func(excluding int) {
		fmt.Fprintf(&buf, " [%s", a.GerritChanges[priorIdx].Host)
		for i := priorIdx; i < excluding; i++ {
			cl := a.GerritChanges[i]
			fmt.Fprintf(&buf, " %d/%d", cl.Change, cl.Patchset)
		}
		buf.WriteString("]")
		priorIdx = excluding
	}
	for j, cl := range a.GerritChanges {
		if a.GerritChanges[priorIdx].Host != cl.Host {
			emit(j)
		}
	}
	emit(len(a.GerritChanges))
	return buf.String()
}

const cqLabelName = "Commit-Queue"

var modeToVoteValue = map[run.Mode]int32{
	run.DryRun:  1,
	run.FullRun: 2,
}

func findTrigger(ci *gerritpb.ChangeInfo, mode run.Mode) (*migrationpb.RunCL_Trigger, error) {
	li := ci.GetLabels()[cqLabelName]
	if li == nil {
		return nil, errors.Reason("missing LabelInfo for label %q", cqLabelName).Err()
	}

	voteVal, ok := modeToVoteValue[mode]
	if !ok {
		return nil, errors.Reason("mode %q doesn't have a corresponding vote value for label %q", mode, cqLabelName).Err()
	}

	// Check if there was a previous attempt that got canceled by means of a
	// comment. Normally, CQDaemon would remove appropriate label, but in case
	// of ACLs misconfiguration preventing CQDaemon from removing votes on
	// behalf of users, CQDaemon will abort attempt by posting special comment.
	var prevAttemptTs time.Time
	for _, msg := range ci.GetMessages() {
		if bd, ok := botdata.Parse(msg); ok {
			if bd.Action == botdata.Cancel && bd.Revision == ci.GetCurrentRevision() && bd.TriggeredAt.After(prevAttemptTs) {
				prevAttemptTs = bd.TriggeredAt
			}
		}
	}

	// Trigger is the earliest vote that has value == `voteVal`.
	var ret *migrationpb.RunCL_Trigger
	for _, ai := range li.GetAll() {
		if ai.GetValue() != voteVal {
			continue
		}
		if voteTs := ai.GetDate().AsTime(); voteTs.After(prevAttemptTs) && (ret == nil || voteTs.Before(ret.GetTime().AsTime())) {
			ret = &migrationpb.RunCL_Trigger{
				Time:      ai.GetDate(),
				AccountId: ai.GetUser().GetAccountId(),
				Email:     ai.GetUser().GetEmail(),
			}
		}
	}
	if ret == nil {
		return nil, errors.Reason("no vote found for %q +%d", cqLabelName, voteVal).Err()
	}

	// Gerrit may copy CQ vote(s) to next patchset in some project configurations.
	// In such cases, CQ vote timestamp will be *before* new patchset creation,
	// and yet *this* CQ attempt started only when new patchset was created.
	// So, attempt start is the latest of timestamps (CQ vote, this patchset
	// creation).
	if revisionTs := ci.Revisions[ci.GetCurrentRevision()].GetCreated(); ret.GetTime().AsTime().Before(revisionTs.AsTime()) {
		ret.Time = revisionTs
	}
	return ret, nil
}
