// Copyright 2022 The LUCI Authors.
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

// package server implements the server to handle pRPC requests.
package server

import (
	"context"
	"fmt"

	"go.chromium.org/luci/bisection/model"
	gfim "go.chromium.org/luci/bisection/model"
	gfipb "go.chromium.org/luci/bisection/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GoFinditBotServer implements the proto service GoFinditBotService.
type GoFinditBotServer struct{}

// UpdateAnalysisProgress is an RPC endpoints used by the recipes to update
// analysis progress.
func (server *GoFinditBotServer) UpdateAnalysisProgress(c context.Context, req *gfipb.UpdateAnalysisProgressRequest) (*gfipb.UpdateAnalysisProgressResponse, error) {
	err := verifyUpdateAnalysisProgressRequest(c, req)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: %s", err)
	}
	logging.Infof(c, "Update analysis with rerun_build_id = %d analysis_id = %d gitiles_commit=%v ", req.Bbid, req.AnalysisId, req.GitilesCommit)

	// Get rerun model
	rerunModel := &gfim.CompileRerunBuild{
		Id: req.Bbid,
	}
	switch err := datastore.Get(c, rerunModel); {
	case err == datastore.ErrNoSuchEntity:
		return nil, status.Errorf(codes.NotFound, "could not find rerun build with id %d", req.Bbid)
	case err != nil:
		return nil, status.Errorf(codes.Internal, "error finding rerun build %s", err)
	default:
		//continue
	}

	// We only support analysis progress for culprit verification now
	// TODO (nqmtuan): remove this when we support updating progress for nth-section
	if rerunModel.Type != gfim.RerunBuildType_CulpritVerification {
		return nil, status.Errorf(codes.Unimplemented, "only CulpritVerification is supported at the moment")
	}

	// Update rerun model
	err = updateRerun(c, req, rerunModel)
	if err != nil {
		logging.Errorf(c, "Error updating rerun build %d: %s", req.Bbid, err)
		return nil, status.Errorf(codes.Internal, "error updating rerun build %s", err)
	}

	// Get the suspect for the rerun build
	suspect := &gfim.Suspect{
		Id:             rerunModel.Suspect.IntID(),
		ParentAnalysis: rerunModel.Suspect.Parent(),
	}
	err = datastore.Get(c, suspect)
	if err != nil {
		logging.Errorf(c, "Cannot find suspect for rerun build %d: %s", req.Bbid, err)
		return nil, status.Errorf(codes.Internal, "cannot find suspect for rerun build %s", err)
	}

	err = updateSuspect(c, suspect)
	if err != nil {
		logging.Errorf(c, "Error updating suspect for rerun build %d: %s", req.Bbid, err)
		return nil, status.Errorf(codes.Internal, "error updating suspect %s", err)
	}

	if suspect.VerificationStatus == gfim.SuspectVerificationStatus_ConfirmedCulprit {
		err = updateSuspectAsConfirmedCulprit(c, suspect)
		if err != nil {
			logging.Errorf(c, "Error updating suspect as confirmed culprit for rerun build %d: %s", req.Bbid, err)
			return nil, status.Errorf(codes.Internal, "error updating suspect as confirmed culprit %s", err)
		}
	}

	return &gfipb.UpdateAnalysisProgressResponse{}, nil
}

func verifyUpdateAnalysisProgressRequest(c context.Context, req *gfipb.UpdateAnalysisProgressRequest) error {
	if req.AnalysisId == 0 {
		return fmt.Errorf("analysis_id is required")
	}
	if req.Bbid == 0 {
		return fmt.Errorf("build bucket id is required")
	}
	if req.GitilesCommit == nil {
		return fmt.Errorf("gitiles commit is required")
	}
	if req.RerunResult == nil {
		return fmt.Errorf("rerun result is required")
	}
	return nil
}

// updateSuspect looks at rerun and set the suspect status
func updateSuspect(c context.Context, suspect *gfim.Suspect) error {
	rerunStatus, err := getSingleRerunStatus(c, suspect.SuspectRerunBuild.IntID())
	if err != nil {
		return err
	}
	parentRerunStatus, err := getSingleRerunStatus(c, suspect.ParentRerunBuild.IntID())
	if err != nil {
		return err
	}

	// Update suspect based on rerunStatus and parentRerunStatus
	suspectStatus := getSuspectStatus(c, rerunStatus, parentRerunStatus)
	suspect.VerificationStatus = suspectStatus
	return datastore.Put(c, suspect)
}

// updateSuspectAsConfirmedCulprit update the suspect as the confirmed culprit of analysis
func updateSuspectAsConfirmedCulprit(c context.Context, suspect *gfim.Suspect) error {
	analysisKey := suspect.ParentAnalysis.Parent()
	analysis := &gfim.CompileFailureAnalysis{
		Id: analysisKey.IntID(),
	}
	err := datastore.Get(c, analysis)
	if err != nil {
		return err
	}
	verifiedCulprits := analysis.VerifiedCulprits
	verifiedCulprits = append(verifiedCulprits, datastore.KeyForObj(c, suspect))
	if len(verifiedCulprits) > 1 {
		// Just log the warning here, as it is a rare case
		logging.Warningf(c, "found more than 2 suspects for analysis %d", analysis.Id)
	}
	analysis.VerifiedCulprits = verifiedCulprits
	analysis.Status = gfipb.AnalysisStatus_FOUND
	return datastore.Put(c, analysis)
}

func getSuspectStatus(c context.Context, rerunStatus gfipb.RerunStatus, parentRerunStatus gfipb.RerunStatus) gfim.SuspectVerificationStatus {
	if rerunStatus == gfipb.RerunStatus_FAILED && parentRerunStatus == gfipb.RerunStatus_PASSED {
		return gfim.SuspectVerificationStatus_ConfirmedCulprit
	}
	if rerunStatus == gfipb.RerunStatus_PASSED || parentRerunStatus == gfipb.RerunStatus_FAILED {
		return gfim.SuspectVerificationStatus_Vindicated
	}
	if rerunStatus == gfipb.RerunStatus_INFRA_FAILED || parentRerunStatus == gfipb.RerunStatus_INFRA_FAILED {
		return gfim.SuspectVerificationStatus_VerificationError
	}
	if rerunStatus == gfipb.RerunStatus_RERUN_STATUS_UNSPECIFIED || parentRerunStatus == gfipb.RerunStatus_RERUN_STATUS_UNSPECIFIED {
		return gfim.SuspectVerificationStatus_Unverified
	}
	return gfim.SuspectVerificationStatus_UnderVerification
}

func updateRerun(c context.Context, req *gfipb.UpdateAnalysisProgressRequest, rerunModel *gfim.CompileRerunBuild) error {
	// Find the last SingleRerun
	reruns, err := getRerunsForRerunBuild(c, rerunModel)
	if err != nil {
		return err
	}
	if len(reruns) == 0 {
		logging.Errorf(c, "No SingleRerun has been created for rerun build %d", req.Bbid)
		return fmt.Errorf("SingleRerun not found for build %d", req.Bbid)
	}

	lastRerun := reruns[len(reruns)-1]

	// Verify the gitiles commit, making sure it was the right rerun we are updating
	if !sameGitilesCommit(req.GitilesCommit, &lastRerun.GitilesCommit) {
		logging.Errorf(c, "Got different Gitles commit for rerun build %d", req.Bbid)
		return fmt.Errorf("different gitiles commit for rerun")
	}

	lastRerun.EndTime = clock.Now(c)
	lastRerun.Status = req.RerunResult.RerunStatus

	err = datastore.Put(c, lastRerun)
	if err != nil {
		logging.Errorf(c, "Error updating SingleRerun for build %d: %s", req.Bbid, err)
		return fmt.Errorf("error saving SingleRerun %s", err)
	}
	return nil
}

func getRerunsForRerunBuild(c context.Context, rerunBuild *gfim.CompileRerunBuild) ([]*gfim.SingleRerun, error) {
	q := datastore.NewQuery("SingleRerun").Eq("rerun_build", datastore.KeyForObj(c, rerunBuild)).Order("start_time")
	singleReruns := []*model.SingleRerun{}
	err := datastore.GetAll(c, q, &singleReruns)
	return singleReruns, err
}

func getSingleRerunStatus(c context.Context, rerunId int64) (gfipb.RerunStatus, error) {
	rerunBuild := &gfim.CompileRerunBuild{
		Id: rerunId,
	}
	err := datastore.Get(c, rerunBuild)
	if err != nil {
		return gfipb.RerunStatus_RERUN_STATUS_UNSPECIFIED, err
	}

	// Get SingleRerun
	singleReruns, err := getRerunsForRerunBuild(c, rerunBuild)
	if err != nil {
		return gfipb.RerunStatus_RERUN_STATUS_UNSPECIFIED, err
	}
	if len(singleReruns) != 1 {
		return gfipb.RerunStatus_RERUN_STATUS_UNSPECIFIED, fmt.Errorf("expect 1 single rerun for build %d, got %d", rerunBuild.Id, len(singleReruns))
	}

	return singleReruns[0].Status, nil
}

func sameGitilesCommit(g1 *bbpb.GitilesCommit, g2 *bbpb.GitilesCommit) bool {
	return g1.Host == g2.Host && g1.Project == g2.Project && g1.Id == g2.Id && g1.Ref == g2.Ref
}
