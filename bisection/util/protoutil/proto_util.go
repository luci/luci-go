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

	"google.golang.org/protobuf/types/known/timestamppb"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"

	"go.chromium.org/luci/bisection/internal/tracing"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection/nthsection"
	"go.chromium.org/luci/bisection/util/changelogutil"
	"go.chromium.org/luci/bisection/util/datastoreutil"
)

// TestFailureAnalysisToPb converts model.TestFailureAnalysis to pb.TestAnalysis
func TestFailureAnalysisToPb(ctx context.Context, tfa *model.TestFailureAnalysis, tfaMask *mask.Mask) (analysis *pb.TestAnalysis, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/bisection/util/protoutil/proto_util.TestFailureAnalysisToPb")
	defer func() { tracing.End(ts, err) }()

	result := &pb.TestAnalysis{}
	if tfaMask.MustIncludes("analysis_id") == mask.IncludeEntirely {
		result.AnalysisId = tfa.ID
	}
	if tfaMask.MustIncludes("created_time") == mask.IncludeEntirely {
		result.CreatedTime = timestamppb.New(tfa.CreateTime)
	}
	if tfaMask.MustIncludes("status") == mask.IncludeEntirely {
		result.Status = tfa.Status
	}
	if tfaMask.MustIncludes("run_status") == mask.IncludeEntirely {
		result.RunStatus = tfa.RunStatus
	}
	if tfaMask.MustIncludes("sample_bbid") == mask.IncludeEntirely {
		result.SampleBbid = tfa.FailedBuildID
	}
	if tfaMask.MustIncludes("start_time") == mask.IncludeEntirely && tfa.HasStarted() {
		result.StartTime = timestamppb.New(tfa.StartTime)
	}
	if tfaMask.MustIncludes("end_time") == mask.IncludeEntirely && tfa.HasEnded() {
		result.EndTime = timestamppb.New(tfa.EndTime)
	}
	// It doesn't make sense to return builder information partially.
	// We don't check mask.IncludePartially here.
	if tfaMask.MustIncludes("builder") == mask.IncludeEntirely {
		result.Builder = &buildbucketpb.BuilderID{
			Project: tfa.Project,
			Bucket:  tfa.Bucket,
			Builder: tfa.Builder,
		}
	}

	// Get test bundle.
	var bundle *model.TestFailureBundle
	includeTestFailures := tfaMask.MustIncludes("test_failures")
	if includeTestFailures == mask.IncludeEntirely || includeTestFailures == mask.IncludePartially {
		// Test failure bundle can be large, only fetch it when it's needed.
		bundle, err = datastoreutil.GetTestFailureBundle(ctx, tfa)
		if err != nil {
			return nil, errors.Fmt("get test failure bundle: %w", err)
		}
		tfMask := tfaMask.MustSubmask("test_failures.*")
		result.TestFailures = TestFailureBundleToPb(ctx, bundle, tfMask)
	}

	var primary *model.TestFailure
	if bundle != nil {
		primary = bundle.Primary()
	} else {
		primary, err = datastoreutil.GetPrimaryTestFailure(ctx, tfa)
		if err != nil {
			return nil, errors.Fmt("get primary test failure: %w", err)
		}
	}

	// It doesn't make sense to return commit information partially.
	// We don't check mask.IncludePartially here.
	if tfaMask.MustIncludes("start_commit") == mask.IncludeEntirely {
		result.StartCommit = &buildbucketpb.GitilesCommit{
			Host:     primary.Ref.GetGitiles().GetHost(),
			Project:  primary.Ref.GetGitiles().GetProject(),
			Ref:      primary.Ref.GetGitiles().GetRef(),
			Id:       tfa.StartCommitHash,
			Position: uint32(primary.RegressionStartPosition),
		}
	}
	if tfaMask.MustIncludes("end_commit") == mask.IncludeEntirely {
		result.EndCommit = &buildbucketpb.GitilesCommit{
			Host:     primary.Ref.GetGitiles().GetHost(),
			Project:  primary.Ref.GetGitiles().GetProject(),
			Ref:      primary.Ref.GetGitiles().GetRef(),
			Id:       tfa.EndCommitHash,
			Position: uint32(primary.RegressionEndPosition),
		}
	}

	nsa, err := datastoreutil.GetTestNthSectionForAnalysis(ctx, tfa)
	if err != nil {
		return nil, errors.Fmt("get test nthsection for analysis: %w", err)
	}
	if nsa != nil {
		includeNsa := tfaMask.MustIncludes("nth_section_result")
		if includeNsa == mask.IncludeEntirely || includeNsa == mask.IncludePartially {
			nsaMask := tfaMask.MustSubmask("nth_section_result")
			nsaResult, err := NthSectionAnalysisToPb(ctx, tfa, nsa, primary.Ref, nsaMask)
			if err != nil {
				return nil, errors.Fmt("nthsection analysis to pb: %w", err)
			}
			result.NthSectionResult = nsaResult
		}
		includeCulprit := tfaMask.MustIncludes("culprit")
		if includeCulprit == mask.IncludeEntirely || includeCulprit == mask.IncludePartially {
			culpritMask := tfaMask.MustSubmask("culprit")
			culprit, err := datastoreutil.GetVerifiedCulpritForTestAnalysis(ctx, tfa)
			if err != nil {
				return nil, errors.Fmt("get verified culprit: %w", err)
			}
			if culprit != nil {
				culpritPb, err := CulpritToPb(ctx, culprit, nsa, culpritMask)
				if err != nil {
					return nil, errors.Fmt("culprit to pb: %w", err)
				}
				result.Culprit = culpritPb
			}
		}
	}
	return result, nil
}

func NthSectionAnalysisToPb(ctx context.Context, tfa *model.TestFailureAnalysis, nsa *model.TestNthSectionAnalysis, sourceRef *pb.SourceRef, nsaMask *mask.Mask) (*pb.TestNthSectionAnalysisResult, error) {
	result := &pb.TestNthSectionAnalysisResult{
		StartTime: timestamppb.New(nsa.StartTime),
	}
	if nsaMask.MustIncludes("status") == mask.IncludeEntirely {
		result.Status = nsa.Status
	}
	if nsaMask.MustIncludes("run_status") == mask.IncludeEntirely {
		result.RunStatus = nsa.RunStatus
	}
	if nsaMask.MustIncludes("blame_list") == mask.IncludeEntirely {
		result.BlameList = nsa.BlameList
	}
	if nsaMask.MustIncludes("start_time") == mask.IncludeEntirely {
		result.StartTime = timestamppb.New(nsa.StartTime)
	}
	if nsaMask.MustIncludes("end_time") == mask.IncludeEntirely && nsa.HasEnded() {
		result.EndTime = timestamppb.New(nsa.EndTime)
	}

	// Populate culprit.
	includeSuspect := nsaMask.MustIncludes("suspect")
	if (includeSuspect == mask.IncludeEntirely || includeSuspect == mask.IncludePartially) && nsa.CulpritKey != nil {
		suspectMask := nsaMask.MustSubmask("suspect")
		culprit, err := datastoreutil.GetSuspect(ctx, nsa.CulpritKey.IntID(), nsa.CulpritKey.Parent())
		if err != nil {
			return nil, errors.Fmt("get suspect: %w", err)
		}
		culpritPb, err := CulpritToPb(ctx, culprit, nsa, suspectMask)
		if err != nil {
			return nil, errors.Fmt("culprit to pb: %w", err)
		}
		result.Suspect = culpritPb
	}

	// TODO(nqmtuan): Support selecting subfields of reruns.
	// However, we don't need them now.
	if nsaMask.MustIncludes("reruns") == mask.IncludeEntirely {
		reruns, err := datastoreutil.GetTestNthSectionReruns(ctx, nsa)
		if err != nil {
			return nil, errors.Fmt("get test nthsection reruns: %w", err)
		}
		pbReruns := []*pb.TestSingleRerun{}
		for _, rerun := range reruns {
			pbRerun, err := testSingleRerunToPb(ctx, rerun, nsa)
			if err != nil {
				return nil, errors.Fmt("test single rerun to pb: %d: %w", rerun.ID, err)
			}
			pbReruns = append(pbReruns, pbRerun)
		}
		result.Reruns = pbReruns
	}

	// Populate remaining range.
	// remaining regression range should be returned as whole.
	if nsaMask.MustIncludes("remaining_nth_section_range") == mask.IncludeEntirely && !nsa.HasEnded() {
		snapshot, err := nthsection.CreateSnapshot(ctx, nsa)
		if err != nil {
			return nil, errors.Fmt("couldn't create snapshot: %w", err)
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

func CulpritToPb(ctx context.Context, culprit *model.Suspect, nsa *model.TestNthSectionAnalysis, culpritMask *mask.Mask) (c *pb.TestCulprit, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/bisection/util/protoutil/proto_util.CulpritToPb")
	defer func() { tracing.End(ts, err) }()

	result := &pb.TestCulprit{}

	if culpritMask.MustIncludes("commit") == mask.IncludeEntirely {
		result.Commit = &buildbucketpb.GitilesCommit{
			Host:     culprit.GitilesCommit.Host,
			Project:  culprit.GitilesCommit.Project,
			Ref:      culprit.GitilesCommit.Ref,
			Id:       culprit.GitilesCommit.Id,
			Position: uint32(culprit.GitilesCommit.Position),
		}
	}

	if culpritMask.MustIncludes("review_url") == mask.IncludeEntirely {
		result.ReviewUrl = culprit.ReviewUrl
	}

	if culpritMask.MustIncludes("review_title") == mask.IncludeEntirely {
		result.ReviewTitle = culprit.ReviewTitle
	}

	// TODO (nqmtuan): Support selecting subfields of actions.
	// However, we don't need them now.
	if culpritMask.MustIncludes("culprit_action") == mask.IncludeEntirely {
		result.CulpritAction = CulpritActionsForSuspect(culprit)
	}

	includeDetails := culpritMask.MustIncludes("verification_details")
	if includeDetails == mask.IncludeEntirely || includeDetails == mask.IncludePartially {
		detailsMask := culpritMask.MustSubmask("verification_details")
		verificationDetails, err := testVerificationDetails(ctx, culprit, nsa, detailsMask)
		if err != nil {
			return nil, errors.Fmt("test verification details: %w", err)
		}
		result.VerificationDetails = verificationDetails
	}
	return result, nil
}

func TestFailureBundleToPb(ctx context.Context, bundle *model.TestFailureBundle, mask *mask.Mask) []*pb.TestFailure {
	result := []*pb.TestFailure{}
	// Add primary test failure first, and the rest.
	// Primary should not be nil here, because it is from GetTestFailureBundle.
	primary := bundle.Primary()
	result = append(result, testFailureToPb(ctx, primary, mask))
	for _, tf := range bundle.Others() {
		result = append(result, testFailureToPb(ctx, tf, mask))
	}
	return result
}

func testFailureToPb(ctx context.Context, tf *model.TestFailure, tfMask *mask.Mask) *pb.TestFailure {
	result := &pb.TestFailure{}
	if tfMask.MustIncludes("test_id") == mask.IncludeEntirely {
		result.TestId = tf.TestID
	}
	if tfMask.MustIncludes("variant_hash") == mask.IncludeEntirely {
		result.VariantHash = tf.VariantHash
	}
	if tfMask.MustIncludes("ref_hash") == mask.IncludeEntirely {
		result.RefHash = tf.RefHash
	}
	if tfMask.MustIncludes("variant") == mask.IncludeEntirely {
		result.Variant = tf.Variant
	}
	if tfMask.MustIncludes("is_diverged") == mask.IncludeEntirely {
		result.IsDiverged = tf.IsDiverged
	}
	if tfMask.MustIncludes("is_primary") == mask.IncludeEntirely {
		result.IsPrimary = tf.IsPrimary
	}
	if tfMask.MustIncludes("start_hour") == mask.IncludeEntirely {
		result.StartHour = timestamppb.New(tf.StartHour)
	}
	if tfMask.MustIncludes("start_unexpected_result_rate") == mask.IncludeEntirely {
		result.StartUnexpectedResultRate = float32(tf.StartPositionFailureRate)
	}
	if tfMask.MustIncludes("end_unexpected_result_rate") == mask.IncludeEntirely {
		result.EndUnexpectedResultRate = float32(tf.EndPositionFailureRate)
	}
	return result
}

func testVerificationDetails(ctx context.Context, culprit *model.Suspect, nsa *model.TestNthSectionAnalysis, detailsMask *mask.Mask) (result *pb.TestSuspectVerificationDetails, err error) {
	ctx, ts := tracing.Start(ctx, "go.chromium.org/luci/bisection/util/protoutil/proto_util.testVerificationDetails")
	defer func() { tracing.End(ts, err) }()

	verificationDetails := &pb.TestSuspectVerificationDetails{}
	if detailsMask.MustIncludes("status") == mask.IncludeEntirely {
		verificationDetails.Status = verificationStatusToPb(culprit.VerificationStatus)
	}

	// TODO(nqmtuan): Support selecting subfields of reruns.
	// However, we don't need them now.
	if detailsMask.MustIncludes("suspect_rerun") == mask.IncludeEntirely || detailsMask.MustIncludes("parent_rerun") == mask.IncludeEntirely {
		suspectRerun, parentRerun, err := datastoreutil.GetVerificationRerunsForTestCulprit(ctx, culprit)
		if err != nil {
			return nil, errors.Fmt("get verification reruns for test culprit: %w", err)
		}

		if detailsMask.MustIncludes("suspect_rerun") == mask.IncludeEntirely && suspectRerun != nil {
			verificationDetails.SuspectRerun, err = testSingleRerunToPb(ctx, suspectRerun, nsa)
			if err != nil {
				return nil, errors.Fmt("suspect rerun to pb: %w", err)
			}
		}
		if detailsMask.MustIncludes("parent_rerun") == mask.IncludeEntirely && parentRerun != nil {
			verificationDetails.ParentRerun, err = testSingleRerunToPb(ctx, parentRerun, nsa)
			if err != nil {
				return nil, errors.Fmt("parent rerun to pb: %w", err)
			}
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
		return nil, errors.Fmt("rerun results to pb: %w", err)
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
			return nil, errors.Fmt("rerun test single result to pb: %w", err)
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
		return nil, errors.Fmt("get test failure: %w", err)
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

func SuspectToCulpritPb(s *model.Suspect) *pb.Culprit {
	if s == nil {
		return nil
	}
	return &pb.Culprit{
		Commit: &buildbucketpb.GitilesCommit{
			Host:     s.GitilesCommit.Host,
			Project:  s.GitilesCommit.Project,
			Ref:      s.GitilesCommit.Ref,
			Id:       s.GitilesCommit.Id,
			Position: uint32(s.GitilesCommit.Position),
		},
		ReviewUrl:     s.ReviewUrl,
		ReviewTitle:   s.ReviewTitle,
		CulpritAction: CulpritActionsForSuspect(s),
		VerificationDetails: &pb.SuspectVerificationDetails{
			Status: verificationStatusToPb(s.VerificationStatus).String(),
		},
	}
}

func SuspectToGenAiSuspectPb(s *model.Suspect) *pb.GenAiSuspect {
	if s == nil {
		return nil
	}
	return &pb.GenAiSuspect{
		Commit: &buildbucketpb.GitilesCommit{
			Host:     s.GitilesCommit.Host,
			Project:  s.GitilesCommit.Project,
			Ref:      s.GitilesCommit.Ref,
			Id:       s.GitilesCommit.Id,
			Position: uint32(s.GitilesCommit.Position),
		},
		ReviewUrl:   s.ReviewUrl,
		ReviewTitle: s.ReviewTitle,
		Verified:    s.VerificationStatus == model.SuspectVerificationStatus_ConfirmedCulprit,
	}
}

func CompileGenAIAnalysisToPb(ga *model.CompileGenAIAnalysis, genaiSuspect *model.Suspect) *pb.GenAiAnalysisResult {
	if ga == nil {
		return nil
	}
	res := &pb.GenAiAnalysisResult{
		Status:    ga.Status,
		StartTime: timestamppb.New(ga.StartTime),
	}
	if genaiSuspect != nil {
		res.Suspect = SuspectToGenAiSuspectPb(genaiSuspect)
	}
	if ga.HasEnded() {
		res.EndTime = timestamppb.New(ga.EndTime)
	}
	return res
}

func CompileFailureToPb(cf *model.CompileFailure) *pb.BuildFailure {
	if cf == nil {
		return nil
	}
	return &pb.BuildFailure{
		// TODO(b/265359409): Add fields from compile_failure.proto once it is defined
	}
}
