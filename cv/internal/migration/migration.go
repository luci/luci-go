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

	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"

	cvbqpb "go.chromium.org/luci/cv/api/bigquery/v1"
	migrationpb "go.chromium.org/luci/cv/api/migration"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/gerrit"
	"go.chromium.org/luci/cv/internal/run"
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
		return nil, err
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
	return &empty.Empty{}, nil
}

// ReportVerifiedRun notifies CV of the Run CQDaemon has just finished
// verifying.
//
// Only called iff run was given to CQDaemon by CV via FetchActiveRuns.
func (m *MigrationServer) ReportVerifiedRun(ctx context.Context, req *migrationpb.ReportVerifiedRunRequest) (resp *empty.Empty, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()
	if err = m.checkAllowed(ctx); err != nil {
		return nil, err
	}

	rid := common.RunID(req.GetRun().GetId())
	if rid == "" {
		err = status.Error(codes.InvalidArgument, "empty RunID")
		return
	}
	logging.Debugf(ctx, "ReportVerifiedRun(key %q, CV ID %q)", req.GetRun().GetAttempt().GetKey(), rid)

	vr := &VerifiedCQDRun{
		ID:      rid,
		Payload: req,
	}
	vr.Payload.Run.Id = "" // available as key.
	if err = datastore.Put(ctx, vr); err != nil {
		err = errors.Annotate(err, "failed to put VerifiedRun %q", rid).Tag(transient.Tag).Err()
		return
	}
	if err = run.NotifyCQDVerificationCompleted(ctx, rid); err != nil {
		return
	}

	return &empty.Empty{}, nil
}

// ReportFinishedRun is used by CQD to report runs it handled to completion.
//
// It'll removed upon hitting Milestone 1.
func (m *MigrationServer) ReportFinishedRun(ctx context.Context, req *migrationpb.ReportFinishedRunRequest) (resp *empty.Empty, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()
	if err = m.checkAllowed(ctx); err != nil {
		return nil, err
	}
	if k := req.GetRun().GetAttempt().GetKey(); k == "" {
		return nil, appstatus.Error(codes.InvalidArgument, "attempt key is required")
	}
	logging.Debugf(ctx, "ReportFinishedRun(key %q, CV ID %q)", req.GetRun().GetAttempt().GetKey(), req.GetRun().GetId())
	if err = saveFinishedCQDRun(ctx, req.GetRun()); err != nil {
		return nil, err
	}
	// TODO(tandrii,yiwzhang): actually *finalize* the counterpart CV Runs,
	// even if CV ID isn't known, as it can be deduced from the CQD's Attempt Key
	// (assuming CV's generated Runs are compatible).
	return &empty.Empty{}, nil
}

func (m *MigrationServer) ReportUsedNetrc(ctx context.Context, req *migrationpb.ReportUsedNetrcRequest) (resp *empty.Empty, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()
	if err = m.checkAllowed(ctx); err != nil {
		return nil, err
	}
	if req.AccessToken == "" || req.GerritHost == "" {
		return nil, appstatus.Error(codes.InvalidArgument, "access_token and gerrit_host required")
	}

	project := "<UNKNOWN>"
	if i := auth.CurrentIdentity(ctx); i.Kind() == identity.Project {
		project = i.Value()
	}
	logging.Infof(ctx, "CQD[%s] uses netrc access token for %s", project, req.GerritHost)
	if err = gerrit.SaveLegacyNetrcToken(ctx, req.GerritHost, req.AccessToken); err != nil {
		return nil, err
	}
	return &empty.Empty{}, nil
}

// FetchActiveRuns returns all RUNNING runs for the given LUCI Project.
func (m *MigrationServer) FetchActiveRuns(ctx context.Context, req *migrationpb.FetchActiveRunsRequest) (resp *migrationpb.FetchActiveRunsResponse, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()
	if err = m.checkAllowed(ctx); err != nil {
		return nil, err
	}

	resp = &migrationpb.FetchActiveRunsResponse{}
	if resp.Runs, err = fetchActiveRuns(ctx, req.GetLuciProject()); err != nil {
		return nil, err
	}
	return resp, nil
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
