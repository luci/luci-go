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
	"go.chromium.org/luci/common/logging"
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

	if req.GetRun().GetId() == "" {
		err = status.Error(codes.InvalidArgument, "empty RunID")
		return
	}
	if err = saveFinishedRun(ctx, req.GetRun()); err != nil {
		return
	}
	if err = run.NotifyFinished(ctx, common.RunID(req.GetRun().GetId())); err != nil {
		return
	}

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

	resp = &migrationpb.FetchActiveRunsResponse{}
	resp.Runs, err = fetchActiveRuns(ctx, req.GetLuciProject())
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
