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

// Package updatetestrerun updates test failure analysis when we
// got test results from recipes.
package updatetestrerun

import (
	"context"

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/testfailureanalysis"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/bisection/util/loggingutil"
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Update is for updating test failure analysis given the request from recipe.
func Update(ctx context.Context, req *pb.UpdateTestAnalysisProgressRequest) error {
	err := validateRequest(req)
	if err != nil {
		return status.Errorf(codes.InvalidArgument, errors.Annotate(err, "validate request").Err().Error())
	}
	ctx = loggingutil.SetRerunBBID(ctx, req.Bbid)

	// Fetch rerun.
	rerun, err := datastoreutil.GetTestSingleRerun(ctx, req.Bbid)
	if err != nil {
		// We don't compare err == datastore.ErrNoSuchEntity because err may be annotated.
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			return status.Errorf(codes.NotFound, errors.Annotate(err, "get test single rerun").Err().Error())
		} else {
			return status.Errorf(codes.Internal, errors.Annotate(err, "get test single rerun").Err().Error())
		}
	}

	// Something is wrong here. We should not receive an update for ended rerun.
	if rerun.HasEnded() {
		return status.Errorf(codes.Internal, "rerun has ended")
	}

	// Fetch analysis
	tfa, err := datastoreutil.GetTestFailureAnalysis(ctx, rerun.AnalysisKey.IntID())
	if err != nil {
		// Do not return a NOTFOUND here since the rerun was found.
		// If the analysis is not found, there is likely something wrong.
		return status.Errorf(codes.Internal, errors.Annotate(err, "get test failure analysis").Err().Error())
	}
	ctx = loggingutil.SetAnalysisID(ctx, tfa.ID)

	// Safeguard, we don't really expect any other type.
	if rerun.Type != model.RerunBuildType_CulpritVerification && rerun.Type != model.RerunBuildType_NthSection {
		return status.Errorf(codes.Internal, "invalid rerun type %v", rerun.Type)
	}

	err = updateRerun(ctx, rerun, tfa, req)
	if err != nil {
		return status.Errorf(codes.Internal, errors.Annotate(err, "update rerun").Err().Error())
	}

	if rerun.Type == model.RerunBuildType_CulpritVerification {
		err := processCulpritVerificationUpdate(ctx, rerun, tfa, req)
		if err != nil {
			return status.Errorf(codes.Internal, errors.Annotate(err, "process culprit verification update").Err().Error())
		}
	}
	if rerun.Type == model.RerunBuildType_NthSection {
		err := processNthSectionUpdate(ctx, rerun, tfa)
		if err != nil {
			// TODO (nqmtuan): Update status of analysis.
			return status.Errorf(codes.Internal, errors.Annotate(err, "process nthsection update").Err().Error())
		}
	}
	return nil
}

func processCulpritVerificationUpdate(ctx context.Context, rerun *model.TestSingleRerun, tfa *model.TestFailureAnalysis, req *pb.UpdateTestAnalysisProgressRequest) error {
	// TODO (nqmtuan): implement this.
	return nil
}

func processNthSectionUpdate(ctx context.Context, rerun *model.TestSingleRerun, tfa *model.TestFailureAnalysis) (reterr error) {
	if rerun.NthSectionAnalysisKey == nil {
		return errors.New("nthsection_analysis_key not found")
	}
	nsa, err := datastoreutil.GetTestNthSectionAnalysis(ctx, rerun.NthSectionAnalysisKey.IntID())
	if err != nil {
		return errors.Annotate(err, "get test nthsection analysis").Err()
	}
	snapshot, err := bisection.CreateSnapshot(ctx, nsa)
	if err != nil {
		return errors.Annotate(err, "create snapshot").Err()
	}

	// Check if we already found the culprit or not.
	ok, cul := snapshot.GetCulprit()

	// Found culprit -> Update the nthsection analysis
	if ok {
		// Save nthsection result to datastore.
		_, err := saveSuspectAndUpdateNthSection(ctx, tfa, nsa, snapshot.BlameList.Commits[cul])
		if err != nil {
			return errors.Annotate(err, "store nthsection culprit to datastore").Err()
		}
		// TODO: Trigger culprit verification.
		return nil
	}
	// Culprit not found yet. Still need to trigger more rerun.
	enabled, err := bisection.IsEnabled(ctx)
	if err != nil {
		return errors.Annotate(err, "is enabled").Err()
	}
	if !enabled {
		logging.Infof(ctx, "Bisection not enabled")
		return nil
	}

	// Find the next commit to run.
	commit, err := snapshot.FindNextSingleCommitToRun()
	if err != nil {
		return errors.Annotate(err, "find next nthsection commit to run").Err()
	}
	if commit == "" {
		// We don't have more run to wait -> we've failed to find the suspect.
		if snapshot.NumInProgress == 0 {
			err = saveNthSectionAnalysis(ctx, nsa, func(nsa *model.TestNthSectionAnalysis) {
				nsa.Status = pb.AnalysisStatus_NOTFOUND
				nsa.RunStatus = pb.AnalysisRunStatus_ENDED
				nsa.EndTime = clock.Now(ctx)
			})
			if err != nil {
				return errors.Annotate(err, "save nthsection analysis").Err()
			}
			err = testfailureanalysis.UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED)
			if err != nil {
				return errors.Annotate(err, "update analysis status").Err()
			}
		}
		return nil
	}

	projectBisector, err := bisection.GetProjectBisector(ctx, tfa)
	if err != nil {
		return errors.Annotate(err, "get project bisector").Err()
	}
	err = bisection.TriggerRerunBuildForCommits(ctx, tfa, nsa, projectBisector, []string{commit})
	if err != nil {
		return errors.Annotate(err, "trigger rerun build for commits").Err()
	}
	return nil
}

func saveSuspectAndUpdateNthSection(ctx context.Context, tfa *model.TestFailureAnalysis, nsa *model.TestNthSectionAnalysis, blCommit *pb.BlameListSingleCommit) (*model.Suspect, error) {
	primary, err := datastoreutil.GetPrimaryTestFailure(ctx, tfa)
	if err != nil {
		return nil, errors.Annotate(err, "get primary test failure").Err()
	}
	suspect := &model.Suspect{
		Type: model.SuspectType_NthSection,
		GitilesCommit: bbpb.GitilesCommit{
			Host:    primary.Ref.GetGitiles().GetHost(),
			Project: primary.Ref.GetGitiles().GetProject(),
			Ref:     primary.Ref.GetGitiles().GetRef(),
			Id:      blCommit.Commit,
		},
		ParentAnalysis:     datastore.KeyForObj(ctx, nsa),
		VerificationStatus: model.SuspectVerificationStatus_Unverified,
		ReviewUrl:          blCommit.ReviewUrl,
		ReviewTitle:        blCommit.ReviewTitle,
		AnalysisType:       pb.AnalysisType_TEST_FAILURE_ANALYSIS,
	}
	err = datastore.Put(ctx, suspect)
	if err != nil {
		return nil, errors.Annotate(err, "save suspect").Err()
	}

	err = saveNthSectionAnalysis(ctx, nsa, func(nsa *model.TestNthSectionAnalysis) {
		nsa.Status = pb.AnalysisStatus_SUSPECTFOUND
		nsa.CulpritKey = datastore.KeyForObj(ctx, suspect)
		nsa.RunStatus = pb.AnalysisRunStatus_ENDED
		nsa.EndTime = clock.Now(ctx)
	})

	if err != nil {
		return nil, errors.Annotate(err, "save nthsection analysis").Err()
	}

	// It is not ended because we still need to run suspect verification.
	err = testfailureanalysis.UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
	if err != nil {
		return nil, errors.Annotate(err, "update analysis status").Err()
	}

	return suspect, nil
}

// updateRerun updates TestSingleRerun and TestFailure with the results from recipe.
func updateRerun(ctx context.Context, rerun *model.TestSingleRerun, tfa *model.TestFailureAnalysis, req *pb.UpdateTestAnalysisProgressRequest) (reterr error) {
	defer func() {
		// If there is any error, consider the rerun having infra failure.
		if reterr != nil {
			err := saveRerun(ctx, rerun, func(rerun *model.TestSingleRerun) {
				rerun.Status = pb.RerunStatus_RERUN_STATUS_INFRA_FAILED
				rerun.ReportTime = clock.Now(ctx)
			})
			if err != nil {
				// Nothing we can do now, just log the error.
				logging.Errorf(ctx, "Error when saving rerun")
			}
		}
	}()

	if !req.RunSucceeded {
		err := saveRerun(ctx, rerun, func(rerun *model.TestSingleRerun) {
			rerun.Status = pb.RerunStatus_RERUN_STATUS_INFRA_FAILED
			rerun.ReportTime = clock.Now(ctx)
		})
		if err != nil {
			return errors.Annotate(err, "save rerun").Err()
		}
		// Return nil here because the request is valid and INFRA_FAILED is expected.
		return nil
	}

	rerunTestResults := rerun.TestResults
	rerunTestResults.IsFinalized = true
	var rerunStatus pb.RerunStatus

	// Handle primary test failure.
	// The result of the primary test failure will determine the status of the rerun.
	primary, err := datastoreutil.GetPrimaryTestFailure(ctx, tfa)
	if err != nil {
		return errors.Annotate(err, "get primary test failure").Err()
	}

	recipeResults := req.Results
	// We expect primary failure to have result.
	primaryResult := findTestResult(recipeResults, primary.TestID, primary.VariantHash)
	if primaryResult == nil {
		return errors.New("no result for primary failure")
	}

	// We are bisecting from expected -> unexpected, so we consider
	// expected as "PASSED" and unexpected as "FAILED".
	// Skipped should be treated separately.
	if primaryResult.IsExpected {
		rerunStatus = pb.RerunStatus_RERUN_STATUS_PASSED
	} else {
		rerunStatus = pb.RerunStatus_RERUN_STATUS_FAILED
	}
	if primaryResult.Status == pb.TestResultStatus_SKIP {
		rerunStatus = pb.RerunStatus_RERUN_STATUS_TEST_SKIPPED
	}

	divergedTestFailures := []*model.TestFailure{}
	// rerunTestResults.Results should be pre-populate with test failure keys.
	for i := range rerunTestResults.Results {
		tf, err := datastoreutil.GetTestFailure(ctx, rerunTestResults.Results[i].TestFailureKey.IntID())
		if err != nil {
			return errors.Reason("could not find test failure %d", tf.ID).Err()
		}
		recipeTestResult := findTestResult(recipeResults, tf.TestID, tf.VariantHash)
		if divergedFromPrimary(recipeTestResult, primaryResult) {
			tf.IsDiverged = true
			divergedTestFailures = append(divergedTestFailures, tf)
		}
		if recipeTestResult != nil && recipeTestResult.Status != pb.TestResultStatus_SKIP {
			if recipeTestResult.IsExpected {
				rerunTestResults.Results[i].ExpectedCount = 1
			} else {
				rerunTestResults.Results[i].UnexpectedCount = 1
			}
		}
	}

	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		// Get and save the rerun.
		err := datastore.Get(ctx, rerun)
		if err != nil {
			return errors.Annotate(err, "get rerun").Err()
		}

		rerun.Status = rerunStatus
		rerun.TestResults = rerunTestResults
		rerun.ReportTime = clock.Now(ctx)

		err = datastore.Put(ctx, rerun)
		if err != nil {
			return errors.Annotate(err, "save rerun").Err()
		}

		// It should be safe to just save the test failures here because we don't expect
		// any update to other fields of test failures.
		err = datastore.Put(ctx, divergedTestFailures)
		if err != nil {
			return errors.Annotate(err, "save test failures to update").Err()
		}
		return nil
	}, nil)
}

// saveRerun updates reruns in a way that avoid race condition
// if another thread also update the rerun.
func saveRerun(ctx context.Context, rerun *model.TestSingleRerun, updateFunc func(*model.TestSingleRerun)) error {
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		// Get rerun to avoid race condition if something also update the rerun.
		err := datastore.Get(ctx, rerun)
		if err != nil {
			return errors.Annotate(err, "get rerun").Err()
		}
		updateFunc(rerun)
		// Save the rerun.
		err = datastore.Put(ctx, rerun)
		if err != nil {
			return errors.Annotate(err, "save rerun").Err()
		}
		return nil
	}, nil)
}

// saveNthSectionAnalysis updates nthsection analysis in a way that avoid race condition
// if another thread also update the analysis.
func saveNthSectionAnalysis(ctx context.Context, nsa *model.TestNthSectionAnalysis, updateFunc func(*model.TestNthSectionAnalysis)) error {
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		err := datastore.Get(ctx, nsa)
		if err != nil {
			return errors.Annotate(err, "get nthsection analysis").Err()
		}
		updateFunc(nsa)
		err = datastore.Put(ctx, nsa)
		if err != nil {
			return errors.Annotate(err, "save nthsection analysis").Err()
		}
		return nil
	}, nil)
}

// divergedFromPrimary returns true if testResult diverged from primary result.
// Assuming primaryResult is not nil.
func divergedFromPrimary(testResult *pb.TestResult, primaryResult *pb.TestResult) bool {
	// In case the test was not found or is not run, testResult is nil.
	// In this case, we consider it to be diverged from primary result.
	if testResult == nil {
		return true
	}
	// If the primary test is skipped, we will not know if test result is diverged.
	// But perhaps it should not matter, since we will not be able to continue anyway.
	if primaryResult.Status == pb.TestResultStatus_SKIP {
		return false
	}
	// Primary not skip and test skip -> diverge.
	if testResult.Status == pb.TestResultStatus_SKIP {
		return true
	}
	return testResult.IsExpected != primaryResult.IsExpected
}

// findTestResult returns TestResult given testID and variantHash.
func findTestResult(results []*pb.TestResult, testID string, variantHash string) *pb.TestResult {
	for _, result := range results {
		if result.TestId == testID && result.VariantHash == variantHash {
			return result
		}
	}
	return nil
}

func validateRequest(req *pb.UpdateTestAnalysisProgressRequest) error {
	if req.Bbid == 0 {
		return errors.New("no rerun bbid specified")
	}
	if req.BotId == "" {
		return errors.New("no bot id specified")
	}
	return nil
}
