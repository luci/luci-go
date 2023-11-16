// Copyright 2023 The LUCI Authors.
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

// Package protoutil contains the utility functions to convert to protobuf.
package protoutil

import (
	"context"
	"strconv"

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection"
	"go.chromium.org/luci/bisection/util/changelogutil"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TestFailureAnalysisToPb converts model.TestFailureAnalysis to pb.TestAnalysis
func TestFailureAnalysisToPb(ctx context.Context, tfa *model.TestFailureAnalysis) (*pb.TestAnalysis, error) {
	result := &pb.TestAnalysis{
		AnalysisId:  tfa.ID,
		CreatedTime: timestamppb.New(tfa.CreateTime),
		Status:      tfa.Status,
		RunStatus:   tfa.RunStatus,
		Builder: &buildbucketpb.BuilderID{
			Project: tfa.Project,
			Bucket:  tfa.Bucket,
			Builder: tfa.Builder,
		},
		SampleBbid: tfa.FailedBuildID,
	}
	if tfa.HasStarted() {
		result.StartTime = timestamppb.New(tfa.StartTime)
	}
	if tfa.HasEnded() {
		result.EndTime = timestamppb.New(tfa.EndTime)
	}

	// Get test bundle.
	bundle, err := datastoreutil.GetTestFailureBundle(ctx, tfa)
	if err != nil {
		return nil, errors.Annotate(err, "get test failure bundle").Err()
	}
	result.TestFailures = TestFailureBundleToPb(ctx, bundle)
	primary := bundle.Primary()
	result.StartFailureRate = float32(primary.StartPositionFailureRate)
	result.EndFailureRate = float32(primary.EndPositionFailureRate)
	result.StartCommit = &buildbucketpb.GitilesCommit{
		Host:     primary.Ref.GetGitiles().GetHost(),
		Project:  primary.Ref.GetGitiles().GetProject(),
		Ref:      primary.Ref.GetGitiles().GetRef(),
		Id:       tfa.StartCommitHash,
		Position: uint32(primary.RegressionStartPosition),
	}
	result.EndCommit = &buildbucketpb.GitilesCommit{
		Host:     primary.Ref.GetGitiles().GetHost(),
		Project:  primary.Ref.GetGitiles().GetProject(),
		Ref:      primary.Ref.GetGitiles().GetRef(),
		Id:       tfa.EndCommitHash,
		Position: uint32(primary.RegressionEndPosition),
	}

	nsa, err := datastoreutil.GetTestNthSectionForAnalysis(ctx, tfa)
	if err != nil {
		return nil, errors.Annotate(err, "get test nthsection for analysis").Err()
	}
	if nsa != nil {
		nsaResult, err := NthSectionAnalysisToPb(ctx, tfa, nsa, primary.Ref)
		if err != nil {
			return nil, errors.Annotate(err, "nthsection analysis to pb").Err()
		}
		result.NthSectionResult = nsaResult
		culprit, err := datastoreutil.GetVerifiedCulpritForTestAnalysis(ctx, tfa)
		if err != nil {
			return nil, errors.Annotate(err, "get verified culprit").Err()
		}
		if culprit != nil {
			culpritPb, err := CulpritToPb(ctx, culprit, nsa)
			if err != nil {
				return nil, errors.Annotate(err, "culprit to pb").Err()
			}
			result.Culprit = culpritPb
		}
	}
	return result, nil
}

func NthSectionAnalysisToPb(ctx context.Context, tfa *model.TestFailureAnalysis, nsa *model.TestNthSectionAnalysis, sourceRef *pb.SourceRef) (*pb.TestNthSectionAnalysisResult, error) {
	result := &pb.TestNthSectionAnalysisResult{
		Status:    nsa.Status,
		RunStatus: nsa.RunStatus,
		BlameList: nsa.BlameList,
		StartTime: timestamppb.New(nsa.StartTime),
	}
	if nsa.HasEnded() {
		result.EndTime = timestamppb.New(nsa.EndTime)
	}
	// Populate culprit.
	if nsa.CulpritKey != nil {
		culprit, err := datastoreutil.GetSuspect(ctx, nsa.CulpritKey.IntID(), nsa.CulpritKey.Parent())
		if err != nil {
			return nil, errors.Annotate(err, "get suspect").Err()
		}
		culpritPb, err := CulpritToPb(ctx, culprit, nsa)
		if err != nil {
			return nil, errors.Annotate(err, "culprit to pb").Err()
		}
		result.Suspect = culpritPb
	}

	// Populate reruns.
	reruns, err := datastoreutil.GetTestNthSectionReruns(ctx, nsa)
	if err != nil {
		return nil, errors.Annotate(err, "get test nthsection reruns").Err()
	}
	pbReruns := []*pb.TestSingleRerun{}
	for _, rerun := range reruns {
		pbRerun, err := testSingleRerunToPb(ctx, rerun, nsa)
		if err != nil {
			return nil, errors.Annotate(err, "test single rerun to pb: %d", rerun.ID).Err()
		}
		pbReruns = append(pbReruns, pbRerun)
	}
	result.Reruns = pbReruns

	// Populate remaining range.
	if !nsa.HasEnded() {
		snapshot, err := bisection.CreateSnapshot(ctx, nsa)
		if err != nil {
			return nil, errors.Annotate(err, "couldn't create snapshot").Err()
		}
		ff, lp, err := snapshot.GetCurrentRegressionRange()
		// GetCurrentRegressionRange return error if the regression is invalid.
		// It is not exactly an error, but just a state of the analysis.
		if err == nil {
			// GetCurrentRegressionRange returns a pair of indices from the Snapshot that contains the culprit.
			// So to really get the last pass, we should add 1.
			lp++
			lpCommitID := ""

			// Blamelist only contains the possible commits for culprit.
			// So it may or may not contain last pass. It will contain
			// last pass if the regression range has been narrowed down during bisection,
			// and the original last pass has been updated.
			// In case that the blamelist does not contain last pass, we should get it
			// from LastPassCommit.
			if lp < len(nsa.BlameList.Commits) {
				lpCommitID = nsa.BlameList.Commits[lp].Commit
			} else {
				// Old data do not have LastPassCommit populated, so we will check here.
				if nsa.BlameList.LastPassCommit != nil {
					lpCommitID = nsa.BlameList.LastPassCommit.Commit
				}
			}
			if lpCommitID != "" {
				result.RemainingNthSectionRange = &pb.RegressionRange{
					FirstFailed: &buildbucketpb.GitilesCommit{
						Host:    sourceRef.GetGitiles().Host,
						Project: sourceRef.GetGitiles().Project,
						Ref:     sourceRef.GetGitiles().Ref,
						Id:      nsa.BlameList.Commits[ff].Commit,
					},
					LastPassed: &buildbucketpb.GitilesCommit{
						Host:    sourceRef.GetGitiles().Host,
						Project: sourceRef.GetGitiles().Project,
						Ref:     sourceRef.GetGitiles().Ref,
						Id:      lpCommitID,
					},
				}
			}
		}
	}

	return result, nil
}

func CulpritToPb(ctx context.Context, culprit *model.Suspect, nsa *model.TestNthSectionAnalysis) (*pb.TestCulprit, error) {
	result := &pb.TestCulprit{
		Commit: &buildbucketpb.GitilesCommit{
			Host:     culprit.GitilesCommit.Host,
			Project:  culprit.GitilesCommit.Project,
			Ref:      culprit.GitilesCommit.Ref,
			Id:       culprit.GitilesCommit.Id,
			Position: culprit.GitilesCommit.Position,
		},
		ReviewUrl:     culprit.ReviewUrl,
		ReviewTitle:   culprit.ReviewTitle,
		CulpritAction: CulpritActionsForSuspect(culprit),
	}

	verificationDetails, err := testVerificationDetails(ctx, culprit, nsa)
	if err != nil {
		return nil, errors.Annotate(err, "test verification details").Err()
	}
	result.VerificationDetails = verificationDetails
	return result, nil
}

func TestFailureBundleToPb(ctx context.Context, bundle *model.TestFailureBundle) []*pb.TestFailure {
	result := []*pb.TestFailure{}
	// Add primary test failure first, and the rest.
	// Primary should not be nil here, because it is from GetTestFailureBundle.
	primary := bundle.Primary()
	result = append(result, testFailureToPb(ctx, primary))
	for _, tf := range bundle.Others() {
		result = append(result, testFailureToPb(ctx, tf))
	}
	return result
}

func testFailureToPb(ctx context.Context, tf *model.TestFailure) *pb.TestFailure {
	return &pb.TestFailure{
		TestId:      tf.TestID,
		VariantHash: tf.VariantHash,
		RefHash:     tf.RefHash,
		Variant:     tf.Variant,
		IsDiverged:  tf.IsDiverged,
		IsPrimary:   tf.IsPrimary,
		StartHour:   timestamppb.New(tf.StartHour),
	}
}

func testVerificationDetails(ctx context.Context, culprit *model.Suspect, nsa *model.TestNthSectionAnalysis) (*pb.TestSuspectVerificationDetails, error) {
	verificationDetails := &pb.TestSuspectVerificationDetails{
		Status: verificationStatusToPb(culprit.VerificationStatus),
	}
	suspectRerun, parentRerun, err := datastoreutil.GetVerificationRerunsForTestCulprit(ctx, culprit)
	if err != nil {
		return nil, errors.Annotate(err, "get verification reruns for test culprit").Err()
	}

	if suspectRerun != nil {
		verificationDetails.SuspectRerun, err = testSingleRerunToPb(ctx, suspectRerun, nsa)
		if err != nil {
			return nil, errors.Annotate(err, "suspect rerun to pb").Err()
		}
	}
	if parentRerun != nil {
		verificationDetails.ParentRerun, err = testSingleRerunToPb(ctx, parentRerun, nsa)
		if err != nil {
			return nil, errors.Annotate(err, "parent rerun to pb").Err()
		}
	}
	return verificationDetails, nil
}

func verificationStatusToPb(status model.SuspectVerificationStatus) pb.SuspectVerificationStatus {
	switch status {
	case model.SuspectVerificationStatus_Unverified:
		return pb.SuspectVerificationStatus_UNVERIFIED
	case model.SuspectVerificationStatus_VerificationScheduled:
		return pb.SuspectVerificationStatus_VERIFICATION_SCHEDULED
	case model.SuspectVerificationStatus_UnderVerification:
		return pb.SuspectVerificationStatus_UNDER_VERIFICATION
	case model.SuspectVerificationStatus_ConfirmedCulprit:
		return pb.SuspectVerificationStatus_CONFIRMED_CULPRIT
	case model.SuspectVerificationStatus_Vindicated:
		return pb.SuspectVerificationStatus_VINDICATED
	case model.SuspectVerificationStatus_VerificationError:
		return pb.SuspectVerificationStatus_VERIFICATION_ERROR
	case model.SuspectVerificationStatus_Canceled:
		return pb.SuspectVerificationStatus_VERIFICATION_CANCELED
	default:
		return pb.SuspectVerificationStatus_SUSPECT_VERIFICATION_STATUS_UNSPECIFIED
	}
}

func testSingleRerunToPb(ctx context.Context, rerun *model.TestSingleRerun, nsa *model.TestNthSectionAnalysis) (*pb.TestSingleRerun, error) {
	result := &pb.TestSingleRerun{
		Bbid:       rerun.ID,
		CreateTime: timestamppb.New(rerun.CreateTime),
		Commit: &buildbucketpb.GitilesCommit{
			Host:    rerun.LUCIBuild.GitilesCommit.GetHost(),
			Project: rerun.LUCIBuild.GitilesCommit.GetProject(),
			Ref:     rerun.LUCIBuild.GitilesCommit.GetRef(),
			Id:      rerun.LUCIBuild.GitilesCommit.GetId(),
		},
	}
	if rerun.HasStarted() {
		result.StartTime = timestamppb.New(rerun.StartTime)
	}
	if rerun.HasEnded() {
		result.EndTime = timestamppb.New(rerun.EndTime)
	}
	if rerun.ReportTime.Unix() != 0 {
		result.ReportTime = timestamppb.New(rerun.ReportTime)
	}

	index := changelogutil.FindCommitIndexInBlameList(rerun.GitilesCommit, nsa.BlameList)
	// There is only one case where we cannot find the rerun in blamelist
	// It is when the rerun is part of the culprit verification and is
	// the "last pass" revision.
	// In this case, we should continue.
	if index != -1 {
		result.Index = strconv.FormatInt(int64(index), 10)
		result.Commit.Position = uint32(nsa.BlameList.Commits[index].Position)
	}

	// Update rerun results.
	pbRerunResults, err := rerunResultsToPb(ctx, rerun.TestResults, rerun.Status)
	if err != nil {
		return nil, errors.Annotate(err, "rerun results to pb").Err()
	}
	result.RerunResult = pbRerunResults
	return result, nil
}

func rerunResultsToPb(ctx context.Context, testResults model.RerunTestResults, status pb.RerunStatus) (*pb.RerunTestResults, error) {
	pb := &pb.RerunTestResults{
		RerunStatus: status,
	}
	if !testResults.IsFinalized {
		return pb, nil
	}
	for _, singleResult := range testResults.Results {
		pbSingleResult, err := rerunTestSingleResultToPb(ctx, singleResult)
		if err != nil {
			return nil, errors.Annotate(err, "rerun test single result to pb").Err()
		}
		pb.Results = append(pb.Results, pbSingleResult)
	}
	return pb, nil
}

func rerunTestSingleResultToPb(ctx context.Context, singleResult model.RerunSingleTestResult) (*pb.RerunTestSingleResult, error) {
	pb := &pb.RerunTestSingleResult{
		ExpectedCount:   singleResult.ExpectedCount,
		UnexpectedCount: singleResult.UnexpectedCount,
	}
	tf, err := datastoreutil.GetTestFailure(ctx, singleResult.TestFailureKey.IntID())
	if err != nil {
		return nil, errors.Annotate(err, "get test failure").Err()
	}
	pb.TestId = tf.TestID
	pb.VariantHash = tf.VariantHash
	return pb, nil
}

func CulpritActionsForSuspect(suspect *model.Suspect) []*pb.CulpritAction {
	culpritActions := []*pb.CulpritAction{}
	if suspect.IsRevertCommitted {
		// culprit action for auto-committing a revert
		culpritActions = append(culpritActions, &pb.CulpritAction{
			ActionType:  pb.CulpritActionType_CULPRIT_AUTO_REVERTED,
			RevertClUrl: suspect.RevertURL,
			ActionTime:  timestamppb.New(suspect.RevertCommitTime),
		})
	} else if suspect.IsRevertCreated {
		// culprit action for creating a revert
		culpritActions = append(culpritActions, &pb.CulpritAction{
			ActionType:  pb.CulpritActionType_REVERT_CL_CREATED,
			RevertClUrl: suspect.RevertURL,
			ActionTime:  timestamppb.New(suspect.RevertCreateTime),
		})
	} else if suspect.HasSupportRevertComment {
		// culprit action for commenting on an existing revert
		culpritActions = append(culpritActions, &pb.CulpritAction{
			ActionType:  pb.CulpritActionType_EXISTING_REVERT_CL_COMMENTED,
			RevertClUrl: suspect.RevertURL,
			ActionTime:  timestamppb.New(suspect.SupportRevertCommentTime),
		})
	} else if suspect.HasCulpritComment {
		// culprit action for commenting on the culprit
		culpritActions = append(culpritActions, &pb.CulpritAction{
			ActionType: pb.CulpritActionType_CULPRIT_CL_COMMENTED,
			ActionTime: timestamppb.New(suspect.CulpritCommentTime),
		})
	} else {
		action := &pb.CulpritAction{
			ActionType:     pb.CulpritActionType_NO_ACTION,
			InactionReason: suspect.InactionReason,
		}
		if suspect.RevertURL != "" {
			action.RevertClUrl = suspect.RevertURL
		}
		culpritActions = append(culpritActions, action)
	}
	return culpritActions
}
