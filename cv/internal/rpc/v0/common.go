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

package rpc

import (
	"context"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"

	apiv0pb "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/cv/internal/acls"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/rpc/versioning"
	"go.chromium.org/luci/cv/internal/run"
)

// RunsServer implements the v0 API.
type RunsServer struct {
	apiv0pb.UnimplementedRunsServer
}

// checkCanUseAPI ensures that calling user is granted permission to use
// unstable v0 API.
func checkCanUseAPI(ctx context.Context, name string) error {
	switch yes, err := auth.IsMember(ctx, acls.V0APIAllowGroup); {
	case err != nil:
		return appstatus.Errorf(codes.Internal, "failed to check ACL")
	case !yes:
		return appstatus.Errorf(codes.PermissionDenied, "not a member of %s", acls.V0APIAllowGroup)
	default:
		logging.Debugf(ctx, "%s is calling %s", auth.CurrentIdentity(ctx), name)
		return nil
	}
}

// populateRunResponse constructs and populates a apiv0pb.Run to use in a response.
func populateRunResponse(ctx context.Context, r *run.Run) (resp *apiv0pb.Run, err error) {
	rcls, err := run.LoadRunCLs(ctx, r.ID, r.CLs)
	if err != nil {
		return nil, err
	}
	gcls := make([]*apiv0pb.GerritChange, len(rcls))
	sCLSet := common.MakeCLIDsSet(r.Submission.GetSubmittedCls()...)
	fCLSet := common.MakeCLIDsSet(r.Submission.GetFailedCls()...)
	sCLIndexes := make([]int32, 0, len(fCLSet))
	fCLIndexes := make([]int32, 0, len(sCLSet))

	for i, rcl := range rcls {
		host, change, err := rcl.ExternalID.ParseGobID()
		switch {
		case err != nil:
			// As of Sep 2, 2021, CV works only with Gerrit (GoB) CL.
			panic(errors.Annotate(err, "ParseGobID").Err())
		case sCLSet.Has(rcl.ID):
			sCLIndexes = append(sCLIndexes, int32(i))
		case fCLSet.Has(rcl.ID):
			fCLIndexes = append(fCLIndexes, int32(i))
		}
		gcls[i] = &apiv0pb.GerritChange{
			Host:     host,
			Change:   change,
			Patchset: rcl.Detail.GetPatchset(),
		}
	}

	var submission *apiv0pb.Run_Submission
	if len(sCLIndexes) > 0 || len(fCLIndexes) > 0 {
		submission = &apiv0pb.Run_Submission{
			SubmittedClIndexes: sCLIndexes,
			FailedClIndexes:    fCLIndexes,
		}
	}

	return &apiv0pb.Run{
		Id:         r.ID.PublicID(),
		Eversion:   int64(r.EVersion),
		Status:     versioning.RunStatusV0(r.Status),
		Mode:       string(r.Mode),
		CreateTime: common.Time2PBNillable(r.CreateTime),
		StartTime:  common.Time2PBNillable(r.StartTime),
		UpdateTime: common.Time2PBNillable(r.UpdateTime),
		EndTime:    common.Time2PBNillable(r.EndTime),
		Owner:      string(r.Owner),
		Cls:        gcls,
		Tryjobs:    constructTryjobs(ctx, r),
		Submission: submission,
	}, nil
}

func constructTryjobs(ctx context.Context, r *run.Run) []*apiv0pb.Tryjob {
	var ret []*apiv0pb.Tryjob
	for i, execution := range r.Tryjobs.GetState().GetExecutions() {
		definition := r.Tryjobs.GetState().GetRequirement().GetDefinitions()[i]
		for _, attempt := range execution.GetAttempts() {
			tj := &apiv0pb.Tryjob{
				Status:   versioning.TryjobStatusV0(attempt.GetStatus()),
				Critical: definition.GetCritical(),
				Reuse:    attempt.GetReused(),
			}
			if result := attempt.GetResult(); result != nil {
				tj.Result = &apiv0pb.Tryjob_Result{
					Status: versioning.TryjobResultStatusV0(result.GetStatus()),
				}
				if bbid := result.GetBuildbucket().GetId(); bbid != 0 {
					tj.Result.Backend = &apiv0pb.Tryjob_Result_Buildbucket_{
						Buildbucket: &apiv0pb.Tryjob_Result_Buildbucket{
							Id: bbid,
						},
					}
				}
			}
			ret = append(ret, tj)
		}
	}
	return ret
}

func min(i, j int) int {
	if i < j {
		return i
	}
	return j
}
