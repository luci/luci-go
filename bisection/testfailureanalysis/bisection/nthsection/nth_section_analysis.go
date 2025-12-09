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
	"go.chromium.org/luci/bisection/testfailureanalysis"
	"go.chromium.org/luci/bisection/util/changelogutil"
	"go.chromium.org/luci/bisection/util/datastoreutil"
)

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
func SaveSuspectAndTriggerCulpritVerification(ctx context.Context, tfa *model.TestFailureAnalysis, nsa *model.TestNthSectionAnalysis, commit *pb.BlameListSingleCommit, isEnabled func(ctx context.Context, project string) (bool, error)) error {
	// Save nthsection result to datastore.
	_, err := saveSuspectAndUpdateNthSection(ctx, tfa, nsa, commit)
	if err != nil {
		return errors.Fmt("store nthsection culprit to datastore: %w", err)
	}
	enabled, err := isEnabled(ctx, tfa.Project)
	if err != nil {
		return errors.Fmt("is enabled: %w", err)
	}
	if !enabled {
		logging.Infof(ctx, "Bisection not enabled")
		// If not enabled, consider analysis ended.
		err = testfailureanalysis.UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
		if err != nil {
			return errors.Fmt("update analysis status: %w", err)
		}
		return nil
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
