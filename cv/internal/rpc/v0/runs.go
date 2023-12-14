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

	"go.chromium.org/luci/grpc/appstatus"

	apiv0pb "go.chromium.org/luci/cv/api/v0"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/rpc/versioning"
	"go.chromium.org/luci/cv/internal/run"
)

// RunsServer implements the v0 API.
type RunsServer struct {
	apiv0pb.UnimplementedRunsServer
}

// populateRunResponse constructs and populates a apiv0pb.Run to use in a response.
func populateRunResponse(ctx context.Context, r *run.Run) (resp *apiv0pb.Run, err error) {
	rcls, err := run.LoadRunCLs(ctx, r.ID, r.CLs)
	if err != nil {
		return nil, appstatus.Attachf(err, codes.Internal, "failed to load cls of the run")
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
			return nil, appstatus.Attachf(err, codes.Unimplemented, "only Gerrit CL is supported")
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

	res := &apiv0pb.Run{
		Id:         r.ID.PublicID(),
		Eversion:   int64(r.EVersion),
		Status:     versioning.RunStatusV0(r.Status),
		Mode:       string(r.Mode),
		CreateTime: common.Time2PBNillable(r.CreateTime),
		StartTime:  common.Time2PBNillable(r.StartTime),
		UpdateTime: common.Time2PBNillable(r.UpdateTime),
		EndTime:    common.Time2PBNillable(r.EndTime),
		Owner:      string(r.Owner),
		CreatedBy:  string(r.CreatedBy),
		Cls:        gcls,
		Tryjobs:    constructLegacyTryjobs(ctx, r),
		Submission: submission,
	}
	res.TryjobInvocations, err = makeTryjobInvocations(ctx, r)
	if err != nil {
		return nil, err
	}
	return res, nil
}

func constructLegacyTryjobs(ctx context.Context, r *run.Run) []*apiv0pb.Tryjob {
	var ret []*apiv0pb.Tryjob
	for i, execution := range r.Tryjobs.GetState().GetExecutions() {
		definition := r.Tryjobs.GetState().GetRequirement().GetDefinitions()[i]
		for _, attempt := range execution.GetAttempts() {
			tj := &apiv0pb.Tryjob{
				Status:   versioning.LegacyTryjobStatusV0(attempt.GetStatus()),
				Critical: definition.GetCritical(),
				Reuse:    attempt.GetReused(),
			}
			if result := attempt.GetResult(); result != nil {
				tj.Result = &apiv0pb.Tryjob_Result{
					Status: versioning.LegacyTryjobResultStatusV0(result.GetStatus()),
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
