// Copyright 2025 The LUCI Authors.
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

// Package nthsection performs nthsection analysis for test failures.
package nthsection

import (
	"context"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/culpritverification/task"
	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/nthsectionsnapshot"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/rerun"
	"go.chromium.org/luci/bisection/testfailureanalysis"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection/analysis"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection/chromium"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection/projectbisector"
	"go.chromium.org/luci/bisection/util/changelogutil"
	"go.chromium.org/luci/bisection/util/datastoreutil"
)

const (
	// maxRerun controls how many reruns we can do at once.
	// maxRerun = 1 means bisection, maxRerun = 2 means trisection, etc.
	// For now, hard-coded to run trisection.
	// TODO (nqmtuan): Tune this based on bot availability.
	maxRerun = 2
)

// Analyze performs nthsection analysis on a test failure.
// It creates the nthsection model, analyzes the blame list, and either:
// - Returns nil with a found culprit (triggers verification)
// - Returns nil after triggering reruns for further analysis
// - Returns an error if something went wrong
func Analyze(ctx context.Context, tfa *model.TestFailureAnalysis, luciAnalysis analysis.AnalysisClient) error {
	// Get project-specific bisector for this analysis.
	projectBisector, err := GetProjectBisector(ctx, tfa)
	if err != nil {
		return errors.Fmt("get project bisector: %w", err)
	}

	// Prepare data for bisection (populates test names and suite names).
	err = projectBisector.Prepare(ctx, tfa, luciAnalysis)
	if err != nil {
		return errors.Fmt("prepare bisection: %w", err)
	}

	// Create nthsection model.
	primaryFailure, err := datastoreutil.GetPrimaryTestFailure(ctx, tfa)
	if err != nil {
		return errors.Fmt("get primary test failure: %w", err)
	}
	nsa, err := CreateNthSectionModel(ctx, tfa, primaryFailure)
	if err != nil {
		return errors.Fmt("create nth section model: %w", err)
	}

	snapshot, err := CreateSnapshot(ctx, nsa)
	if err != nil {
		return errors.Fmt("create snapshot: %w", err)
	}

	// The culprit may be found without any bisection rerun, it is the case
	// where we have only 1 commit in the blame list.
	// In such cases, we should save the culprit and trigger culprit verification.
	ok, cul := snapshot.GetCulprit()

	// Found culprit -> Update the nthsection analysis
	if ok {
		err := SaveSuspectAndTriggerCulpritVerification(ctx, tfa, nsa, snapshot.BlameList.Commits[cul])
		if err != nil {
			return errors.Fmt("save suspect and trigger culprit verification: %w", err)
		}
		return nil
	}

	commitHashes, err := snapshot.FindNextCommitsToRun(maxRerun)
	if err != nil {
		var badRangeError *nthsectionsnapshot.BadRangeError
		if !errors.As(err, &badRangeError) {
			return errors.Fmt("find next commits to run: %w", err)
		}
		// BadRangeError suggests that the regression range is invalid.
		// This is not really an error, but more of a indication of no suspect can be found
		// in this regression range. So we end the analysis with NOTFOUND status here.
		if err = testfailureanalysis.UpdateNthSectionAnalysisStatus(ctx, nsa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED); err != nil {
			return errors.Fmt("update nthsection analysis: %w", err)
		}
		if err = testfailureanalysis.UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED); err != nil {
			return errors.Fmt("update analysis status: %w", err)
		}
		logging.Warningf(ctx, "find next single commit to run %s", err.Error())
		return nil
	}

	option := projectbisector.RerunOption{}
	if err = TriggerRerunBuildForCommits(ctx, tfa, nsa, projectBisector, commitHashes, option); err != nil {
		return errors.Fmt("trigger rerun build for commits: %w", err)
	}
	return nil
}

// CreateNthSectionModel creates a new TestNthSectionAnalysis model.
func CreateNthSectionModel(ctx context.Context, tfa *model.TestFailureAnalysis, primaryTestFailure *model.TestFailure) (*model.TestNthSectionAnalysis, error) {
	gitiles := primaryTestFailure.Ref.GetGitiles()
	if gitiles == nil {
		return nil, errors.New("no gitiles")
	}

	regressionRange := &pb.RegressionRange{
		LastPassed: &bbpb.GitilesCommit{
			Host:    gitiles.Host,
			Project: gitiles.Project,
			Ref:     gitiles.Ref,
			Id:      tfa.StartCommitHash,
		},
		FirstFailed: &bbpb.GitilesCommit{
			Host:    gitiles.Host,
			Project: gitiles.Project,
			Ref:     gitiles.Ref,
			Id:      tfa.EndCommitHash,
		},
	}
	changeLogs, err := changelogutil.GetChangeLogs(ctx, regressionRange, true)
	if err != nil {
		return nil, errors.Fmt("couldn't fetch changelog: %w", err)
	}
	blameList := changelogutil.ChangeLogsToBlamelist(ctx, changeLogs)
	if err := changelogutil.SetCommitPositionInBlamelist(blameList, primaryTestFailure.RegressionStartPosition, primaryTestFailure.RegressionEndPosition); err != nil {
		return nil, errors.Fmt("set commit position in blamelist: %w", err)
	}
	nsa := &model.TestNthSectionAnalysis{
		ParentAnalysisKey: datastore.KeyForObj(ctx, tfa),
		StartTime:         clock.Now(ctx),
		Status:            pb.AnalysisStatus_RUNNING,
		RunStatus:         pb.AnalysisRunStatus_STARTED,
		BlameList:         blameList,
	}

	err = datastore.Put(ctx, nsa)
	if err != nil {
		return nil, errors.Fmt("save nthsection: %w", err)
	}
	return nsa, nil
}

// CreateSnapshot creates a snapshot of the nthsection analysis state.
func CreateSnapshot(ctx context.Context, nsa *model.TestNthSectionAnalysis) (*nthsectionsnapshot.Snapshot, error) {
	// Get all reruns for the current analysis.
	// We sort by create_time to make sure the latest rerun for a commit position
	// is considered.
	// If we retry INFRA_FAILURE rerun, we want to take the status of the retry,
	// instead of the INFRA_FAILURE status.
	reruns, err := datastoreutil.GetTestNthSectionReruns(ctx, nsa)
	if err != nil {
		return nil, errors.Fmt("getting test nthsection rerun: %w", err)
	}

	snapshot := &nthsectionsnapshot.Snapshot{
		BlameList: nsa.BlameList,
		Runs:      []*nthsectionsnapshot.Run{},
	}

	statusMap := map[string]pb.RerunStatus{}
	for _, r := range reruns {
		statusMap[r.GitilesCommit.GetId()] = r.Status
		switch r.Status {
		case pb.RerunStatus_RERUN_STATUS_INFRA_FAILED:
			snapshot.NumInfraFailed++
		case pb.RerunStatus_RERUN_STATUS_IN_PROGRESS:
			snapshot.NumInProgress++
		case pb.RerunStatus_RERUN_STATUS_TEST_SKIPPED:
			snapshot.NumTestSkipped++
		}
	}

	blamelist := nsa.BlameList
	for index, cl := range blamelist.Commits {
		if stat, ok := statusMap[cl.Commit]; ok {
			snapshot.Runs = append(snapshot.Runs, &nthsectionsnapshot.Run{
				Index:  index,
				Commit: cl.Commit,
				Status: stat,
				Type:   model.RerunBuildType_NthSection,
			})
		}
	}
	return snapshot, nil
}

// SaveNthSectionAnalysis updates nthsection analysis in a way that avoids race condition
// if another thread also updates the analysis.
func SaveNthSectionAnalysis(ctx context.Context, nsa *model.TestNthSectionAnalysis, updateFunc func(*model.TestNthSectionAnalysis)) error {
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		err := datastore.Get(ctx, nsa)
		if err != nil {
			return errors.Fmt("get nthsection analysis: %w", err)
		}
		updateFunc(nsa)
		err = datastore.Put(ctx, nsa)
		if err != nil {
			return errors.Fmt("save nthsection analysis: %w", err)
		}
		return nil
	}, nil)
}

// SaveSuspectAndTriggerCulpritVerification saves the suspect and triggers culprit verification.
func SaveSuspectAndTriggerCulpritVerification(ctx context.Context, tfa *model.TestFailureAnalysis, nsa *model.TestNthSectionAnalysis, commit *pb.BlameListSingleCommit) error {
	// Save nthsection result to datastore.
	_, err := saveSuspectAndUpdateNthSection(ctx, tfa, nsa, commit)
	if err != nil {
		return errors.Fmt("store nthsection culprit to datastore: %w", err)
	}
	if err := task.ScheduleTestFailureTask(ctx, tfa.ID); err != nil {
		// Non-critical, just log the error
		err := errors.Fmt("schedule culprit verification task %d: %w", tfa.ID, err)
		logging.Errorf(ctx, err.Error())
	}
	return nil
}

func saveSuspectAndUpdateNthSection(ctx context.Context, tfa *model.TestFailureAnalysis, nsa *model.TestNthSectionAnalysis, blCommit *pb.BlameListSingleCommit) (*model.Suspect, error) {
	primary, err := datastoreutil.GetPrimaryTestFailure(ctx, tfa)
	if err != nil {
		return nil, errors.Fmt("get primary test failure: %w", err)
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
		CommitTime:         blCommit.GetCommitTime().AsTime(),
	}
	err = datastore.Put(ctx, suspect)
	if err != nil {
		return nil, errors.Fmt("save suspect: %w", err)
	}

	err = SaveNthSectionAnalysis(ctx, nsa, func(nsa *model.TestNthSectionAnalysis) {
		nsa.Status = pb.AnalysisStatus_SUSPECTFOUND
		nsa.CulpritKey = datastore.KeyForObj(ctx, suspect)
		nsa.RunStatus = pb.AnalysisRunStatus_ENDED
		nsa.EndTime = clock.Now(ctx)
	})

	if err != nil {
		return nil, errors.Fmt("save nthsection analysis: %w", err)
	}

	// It is not ended because we still need to run suspect verification.
	err = testfailureanalysis.UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
	if err != nil {
		return nil, errors.Fmt("update analysis status: %w", err)
	}

	return suspect, nil
}

// GetProjectBisector returns the appropriate project-specific bisector.
func GetProjectBisector(ctx context.Context, tfa *model.TestFailureAnalysis) (projectbisector.ProjectBisector, error) {
	switch tfa.Project {
	case "chromium":
		return &chromium.Bisector{}, nil
	default:
		return nil, errors.Fmt("no bisector for project %s", tfa.Project)
	}
}

// TriggerRerunBuildForCommits triggers rerun builds for the given commits.
func TriggerRerunBuildForCommits(ctx context.Context, tfa *model.TestFailureAnalysis, nsa *model.TestNthSectionAnalysis, projectBisector projectbisector.ProjectBisector, commitHashes []string, option projectbisector.RerunOption) error {
	// Get test failure bundle
	bundle, err := datastoreutil.GetTestFailureBundle(ctx, tfa)
	if err != nil {
		return errors.Fmt("get test failure bundle: %w", err)
	}
	// Only rerun the non-diverged test failures.
	// At first rerun, all test failures are non-diverged, so all will be run.
	tfs := bundle.NonDiverged()
	primaryFailure := bundle.Primary()
	for _, commitHash := range commitHashes {
		gitilesCommit := &bbpb.GitilesCommit{
			Host:    primaryFailure.Ref.GetGitiles().GetHost(),
			Project: primaryFailure.Ref.GetGitiles().GetProject(),
			Ref:     primaryFailure.Ref.GetGitiles().GetRef(),
			Id:      commitHash,
		}
		build, err := projectBisector.TriggerRerun(ctx, tfa, tfs, gitilesCommit, option)
		if err != nil {
			return errors.Fmt("trigger rerun for commit %s: %w", commitHash, err)
		}
		_, err = rerun.CreateTestRerunModel(ctx, rerun.CreateTestRerunModelOptions{
			TestFailureAnalysis:   tfa,
			NthSectionAnalysisKey: datastore.KeyForObj(ctx, nsa),
			TestFailures:          tfs,
			Build:                 build,
			RerunType:             model.RerunBuildType_NthSection,
		})
		if err != nil {
			return errors.Fmt("create test rerun model for build %d: %w", build.GetId(), err)
		}
	}
	return nil
}
