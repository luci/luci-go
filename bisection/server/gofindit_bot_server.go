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

	"go.chromium.org/luci/bisection/compilefailureanalysis/nthsection"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto"
	"go.chromium.org/luci/bisection/util/datastoreutil"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GoFinditBotServer implements the proto service GoFinditBotService.
type GoFinditBotServer struct{}

// UpdateAnalysisProgress is an RPC endpoints used by the recipes to update
// analysis progress.
func (server *GoFinditBotServer) UpdateAnalysisProgress(c context.Context, req *pb.UpdateAnalysisProgressRequest) (*pb.UpdateAnalysisProgressResponse, error) {
	err := verifyUpdateAnalysisProgressRequest(c, req)
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "Invalid request: %s", err)
	}
	logging.Infof(c, "Update analysis with rerun_build_id = %d analysis_id = %d gitiles_commit=%v ", req.Bbid, req.AnalysisId, req.GitilesCommit)

	// Get rerun model
	rerunModel := &model.CompileRerunBuild{
		Id: req.Bbid,
	}
	switch err := datastore.Get(c, rerunModel); {
	case err == datastore.ErrNoSuchEntity:
		return nil, status.Errorf(codes.NotFound, "could not find rerun build with id %d", req.Bbid)
	case err != nil:
		return nil, status.Errorf(codes.Internal, "error finding rerun build")
	default:
		//continue
	}

	lastRerun, err := datastoreutil.GetLastRerunForRerunBuild(c, rerunModel)
	if err != nil {
		err = errors.Annotate(err, "failed getting last rerun for build %d. Analysis ID: %d", rerunModel.Id, req.AnalysisId).Err()
		errors.Log(c, err)
		return nil, status.Errorf(codes.Internal, "error getting last rerun build")
	}

	// Update rerun model
	err = updateRerun(c, req, lastRerun)
	if err != nil {
		err = errors.Annotate(err, "failed updating rerun for build %d. Analysis ID: %d", rerunModel.Id, req.AnalysisId).Err()
		errors.Log(c, err)
		return nil, status.Errorf(codes.Internal, "error updating rerun build")
	}

	// Safeguard, we really don't expect any other type
	if lastRerun.Type != model.RerunBuildType_CulpritVerification && lastRerun.Type != model.RerunBuildType_NthSection {
		logging.Errorf(c, "Invalid type %v for analysis %d", lastRerun.Type, req.AnalysisId)
		return nil, status.Errorf(codes.Internal, "Invalid type %v", lastRerun.Type)
	}

	// Culprit verification
	if lastRerun.Type == model.RerunBuildType_CulpritVerification {
		err := updateSuspectWithRerunData(c, lastRerun)
		if err != nil {
			err = errors.Annotate(err, "updateSuspectWithRerunData for build id %d. Analysis ID: %d", rerunModel.Id, req.AnalysisId).Err()
			errors.Log(c, err)
			return nil, status.Errorf(codes.Internal, "error updating suspect")
		}
		// TODO (nqmtuan): It is possible that we schedule an nth-section run right after
		// a culprit verification run within the same build. We will do this later, for
		// safety, after we verify nth-section analysis is running fine.
		return &pb.UpdateAnalysisProgressResponse{}, nil
	}

	// Nth section
	if lastRerun.Type == model.RerunBuildType_NthSection {
		err := processNthSectionUpdate(c, req)
		if err != nil {
			err = errors.Annotate(err, "processNthSectionUpdate").Err()
			logging.Errorf(c, err.Error())
			return nil, status.Errorf(codes.Internal, err.Error())
		}
		return &pb.UpdateAnalysisProgressResponse{}, nil
	}

	return nil, status.Errorf(codes.Internal, "unknown error")
}

// processNthSectionUpdate processes the bot update for nthsection analysis run
// It will schedule the next run for nthsection analysis targeting the same bot
func processNthSectionUpdate(c context.Context, req *pb.UpdateAnalysisProgressRequest) error {
	cfa, err := datastoreutil.GetCompileFailureAnalysis(c, req.AnalysisId)
	if err != nil {
		return err
	}
	nsa, err := datastoreutil.GetNthSectionAnalysis(c, cfa)
	if err != nil {
		return err
	}

	// There is no nthsection analysis for this analysis
	if nsa == nil {
		return nil
	}

	shouldRun, commit, err := findNextNthSectionCommitToRun(c, nsa)
	if err != nil {
		return errors.Annotate(err, "findNextNthSectionCommitToRun").Err()
	}
	if !shouldRun {
		return nil
	}

	// We got the next commit to run. We will schedule a rerun targetting the same bot
	gitilesCommit := &bbpb.GitilesCommit{
		Host:    req.GitilesCommit.Host,
		Project: req.GitilesCommit.Project,
		Ref:     req.GitilesCommit.Ref,
		Id:      commit,
	}
	dims := map[string]string{
		"id": req.BotId,
	}
	err = nthsection.RerunCommit(c, nsa, gitilesCommit, cfa.FirstFailedBuildId, dims)
	if err != nil {
		return errors.Annotate(err, "rerun commit for %s", commit).Err()
	}
	return nil
}

// findNextNthSectionCommitToRun return true (and the commit) if it can find a nthsection commit to run next
func findNextNthSectionCommitToRun(c context.Context, nsa *model.CompileNthSectionAnalysis) (bool, string, error) {
	snapshot, err := nthsection.CreateSnapshot(c, nsa)
	if err != nil {
		return false, "", errors.Annotate(err, "couldn't create snapshot").Err()
	}

	// We pass 1 as argument here because at this moment, we only have 1 "slot" left for nth section
	commits, err := snapshot.FindNextCommitsToRun(1)
	if err != nil {
		return false, "", errors.Annotate(err, "couldn't find next commits to run").Err()
	}
	if len(commits) != 1 {
		return false, "", errors.Annotate(err, "expect only 1 commits to rerun. Got %d", len(commits)).Err()
	}
	return true, commits[0], nil
}

func updateSuspectWithRerunData(c context.Context, rerun *model.SingleRerun) error {
	// Get the suspect for the rerun build
	if rerun.Suspect == nil {
		return fmt.Errorf("no suspect for rerun %d", rerun.Id)
	}

	suspect := &model.Suspect{
		Id:             rerun.Suspect.IntID(),
		ParentAnalysis: rerun.Suspect.Parent(),
	}
	err := datastore.Get(c, suspect)
	if err != nil {
		return errors.Annotate(err, "couldn't find suspect for rerun %d", rerun.Id).Err()
	}

	err = updateSuspect(c, suspect)
	if err != nil {
		return errors.Annotate(err, "error updating suspect for rerun %d", rerun.Id).Err()
	}

	if suspect.VerificationStatus == model.SuspectVerificationStatus_ConfirmedCulprit {
		err = updateSuspectAsConfirmedCulprit(c, suspect)
		if err != nil {
			return errors.Annotate(err, "error updateSuspectAsConfirmedCulprit for rerun %d", rerun.Id).Err()
		}
	}
	return nil
}

func verifyUpdateAnalysisProgressRequest(c context.Context, req *pb.UpdateAnalysisProgressRequest) error {
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
	if req.BotId == "" {
		return fmt.Errorf("bot_id is required")
	}
	return nil
}

// updateSuspect looks at rerun and set the suspect status
func updateSuspect(c context.Context, suspect *model.Suspect) error {
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
func updateSuspectAsConfirmedCulprit(c context.Context, suspect *model.Suspect) error {
	analysisKey := suspect.ParentAnalysis.Parent()
	analysis := &model.CompileFailureAnalysis{
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
	analysis.Status = pb.AnalysisStatus_FOUND
	analysis.EndTime = clock.Now(c)
	return datastore.Put(c, analysis)
}

func getSuspectStatus(c context.Context, rerunStatus pb.RerunStatus, parentRerunStatus pb.RerunStatus) model.SuspectVerificationStatus {
	if rerunStatus == pb.RerunStatus_FAILED && parentRerunStatus == pb.RerunStatus_PASSED {
		return model.SuspectVerificationStatus_ConfirmedCulprit
	}
	if rerunStatus == pb.RerunStatus_PASSED || parentRerunStatus == pb.RerunStatus_FAILED {
		return model.SuspectVerificationStatus_Vindicated
	}
	if rerunStatus == pb.RerunStatus_INFRA_FAILED || parentRerunStatus == pb.RerunStatus_INFRA_FAILED {
		return model.SuspectVerificationStatus_VerificationError
	}
	if rerunStatus == pb.RerunStatus_RERUN_STATUS_UNSPECIFIED || parentRerunStatus == pb.RerunStatus_RERUN_STATUS_UNSPECIFIED {
		return model.SuspectVerificationStatus_Unverified
	}
	return model.SuspectVerificationStatus_UnderVerification
}

// updateRerun updates the last SingleRerun for rerunModel with the information from req.
// Returns the last SingleRerun and error (if it occur).
func updateRerun(c context.Context, req *pb.UpdateAnalysisProgressRequest, rerun *model.SingleRerun) error {
	// Verify the gitiles commit, making sure it was the right rerun we are updating
	if !sameGitilesCommit(req.GitilesCommit, &rerun.GitilesCommit) {
		logging.Errorf(c, "Got different Gitles commit for rerun build %d", req.Bbid)
		return fmt.Errorf("different gitiles commit for rerun")
	}

	rerun.EndTime = clock.Now(c)
	rerun.Status = req.RerunResult.RerunStatus

	err := datastore.Put(c, rerun)
	if err != nil {
		logging.Errorf(c, "Error updating SingleRerun for build %d: %s", req.Bbid, rerun)
		return errors.Annotate(err, "saving SingleRerun").Err()
	}
	return nil
}

func getSingleRerunStatus(c context.Context, rerunId int64) (pb.RerunStatus, error) {
	rerunBuild := &model.CompileRerunBuild{
		Id: rerunId,
	}
	err := datastore.Get(c, rerunBuild)
	if err != nil {
		return pb.RerunStatus_RERUN_STATUS_UNSPECIFIED, err
	}

	// Get SingleRerun
	singleRerun, err := datastoreutil.GetLastRerunForRerunBuild(c, rerunBuild)
	if err != nil {
		return pb.RerunStatus_RERUN_STATUS_UNSPECIFIED, err
	}

	return singleRerun.Status, nil
}

func sameGitilesCommit(g1 *bbpb.GitilesCommit, g2 *bbpb.GitilesCommit) bool {
	return g1.Host == g2.Host && g1.Project == g2.Project && g1.Id == g2.Id && g1.Ref == g2.Ref
}
