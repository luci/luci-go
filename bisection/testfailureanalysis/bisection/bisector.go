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

// Package bisection performs bisection for test failures.
package bisection

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/proto"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/bisection/culpritverification/task"
	"go.chromium.org/luci/bisection/internal/buildbucket"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/lucianalysis"
	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/nthsectionsnapshot"
	pb "go.chromium.org/luci/bisection/proto/v1"
	tpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/testfailureanalysis"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection/analysis"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection/chromium"
	"go.chromium.org/luci/bisection/testfailureanalysis/bisection/projectbisector"
	"go.chromium.org/luci/bisection/util/changelogutil"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/bisection/util/loggingutil"

	// Add support for datastore transactions in TQ.
	_ "go.chromium.org/luci/server/tq/txn/datastore"
)

const (
	taskClass = "test-failure-bisection"
	queue     = "test-failure-bisection"
)

var taskClassRef = tq.RegisterTaskClass(tq.TaskClass{
	ID:        taskClass,
	Prototype: (*tpb.TestFailureBisectionTask)(nil),
	Queue:     queue,
	Kind:      tq.Transactional,
})

// RegisterTaskClass registers the task class for tq dispatcher.
func RegisterTaskClass(srv *server.Server, luciAnalysisProjectFunc func(luciProject string) string) error {
	ctx := srv.Context
	client, err := lucianalysis.NewClient(ctx, srv.Options.CloudProject, luciAnalysisProjectFunc)
	if err != nil {
		return err
	}
	srv.RegisterCleanup(func(context.Context) {
		client.Close()
	})
	handler := func(ctx context.Context, payload proto.Message) error {
		task := payload.(*tpb.TestFailureBisectionTask)
		analysisID := task.GetAnalysisId()
		ctx = loggingutil.SetAnalysisID(ctx, analysisID)
		logging.Infof(ctx, "Processing test failure bisection task with id = %d", analysisID)
		// maxRerun controls how many rerun we can run at a time.
		// For now, hard-code it to run trisection.
		// TODO (nqmtuan): Tune it when we have information about bot availability.
		maxRerun := 2
		err := Run(ctx, analysisID, client, maxRerun)
		if err != nil {
			err = errors.Annotate(err, "run bisection").Err()
			logging.Errorf(ctx, err.Error())
			// Return nil so the task will not be retried.
			// We intentionally disable retrying because bisector does not support retrying at the moment.
			return nil
		}
		return nil
	}
	taskClassRef.AttachHandler(handler)
	return nil
}

// Schedule enqueues a task to perform bisection.
func Schedule(ctx context.Context, analysisID int64) error {
	return tq.AddTask(ctx, &tq.Task{
		Payload: &tpb.TestFailureBisectionTask{
			AnalysisId: analysisID,
		},
		Title: fmt.Sprintf("analysisID-%d", analysisID),
	})
}

// Run runs bisection for the given analysisID.
// maxRerun controls how many reruns we can do at once.
// maxRerun = 1 means bisection, maxRerun = 2 means trisection...
func Run(ctx context.Context, analysisID int64, luciAnalysis analysis.AnalysisClient, maxRerun int) (reterr error) {
	// Retrieves analysis from datastore.
	tfa, err := datastoreutil.GetTestFailureAnalysis(ctx, analysisID)
	if err != nil {
		return errors.Annotate(err, "get test failure analysis").Err()
	}

	defer func() {
		if reterr != nil {
			// If there is an error, mark the analysis as failing with error.
			err := testfailureanalysis.UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
			if err != nil {
				// Just log the error if there is something wrong.
				err = errors.Annotate(err, "update status").Err()
				logging.Errorf(ctx, err.Error())
			}
		}
	}()

	// Checks if test failure analysis is enabled.
	enabled, err := IsEnabled(ctx, tfa.Project)
	if err != nil {
		return errors.Annotate(err, "is enabled").Err()
	}
	if !enabled {
		logging.Infof(ctx, "Bisection is not enabled")
		err = testfailureanalysis.UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_DISABLED, pb.AnalysisRunStatus_ENDED)
		if err != nil {
			return errors.Annotate(err, "update status disabled").Err()
		}
		return nil
	}

	if tfa.Project != "chromium" {
		// We don't support other projects for now, so mark the analysis as unsupported.
		logging.Infof(ctx, "Unsupported project: %s", tfa.Project)
		err = testfailureanalysis.UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_UNSUPPORTED, pb.AnalysisRunStatus_ENDED)
		if err != nil {
			return errors.Annotate(err, "update status unsupported").Err()
		}
		return
	}

	// Update the analysis status.
	err = testfailureanalysis.UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
	if err != nil {
		return errors.Annotate(err, "update status").Err()
	}

	// Create nthsection model.
	primaryFailure, err := datastoreutil.GetPrimaryTestFailure(ctx, tfa)
	if err != nil {
		return errors.Annotate(err, "get primary test failure").Err()
	}
	nsa, err := createNthSectionModel(ctx, tfa, primaryFailure)
	if err != nil {
		return errors.Annotate(err, "create nth section model").Err()
	}

	projectBisector, err := GetProjectBisector(ctx, tfa)
	if err != nil {
		return errors.Annotate(err, "get individual project bisector").Err()
	}

	err = projectBisector.Prepare(ctx, tfa, luciAnalysis)
	if err != nil {
		return errors.Annotate(err, "prepare").Err()
	}

	snapshot, err := CreateSnapshot(ctx, nsa)
	if err != nil {
		return errors.Annotate(err, "create snapshot").Err()
	}

	// The culprit may be found without any bisection rerun, it is the case
	// where we have only 1 commit in the blame list.
	// In such cases, we should save the culprit and trigger culprit verification.
	ok, cul := snapshot.GetCulprit()

	// Found culprit -> Update the nthsection analysis
	if ok {
		err := SaveSuspectAndTriggerCulpritVerification(ctx, tfa, nsa, snapshot.BlameList.Commits[cul])
		if err != nil {
			return errors.Annotate(err, "save suspect and trigger culprit verification").Err()
		}
		return nil
	}

	commitHashes, err := snapshot.FindNextCommitsToRun(maxRerun)
	if err != nil {
		var badRangeError *nthsectionsnapshot.BadRangeError
		if !errors.As(err, &badRangeError) {
			return errors.Annotate(err, "find next commits to run").Err()
		}
		// BadRangeError suggests that the regression range is invalid.
		// This is not really an error, but more of a indication of no suspect can be found
		// in this regression range. So we end the analysis with NOTFOUND status here.
		if err = testfailureanalysis.UpdateNthSectionAnalysisStatus(ctx, nsa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED); err != nil {
			return errors.Annotate(err, "update nthsection analysis").Err()
		}
		if err = testfailureanalysis.UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_NOTFOUND, pb.AnalysisRunStatus_ENDED); err != nil {
			return errors.Annotate(err, "update analysis status").Err()
		}
		logging.Warningf(ctx, "find next single commit to run %s", err.Error())
		return nil
	}

	option := projectbisector.RerunOption{}
	if err = TriggerRerunBuildForCommits(ctx, tfa, nsa, projectBisector, commitHashes, option); err != nil {
		return errors.Annotate(err, "trigger rerun build for commits").Err()
	}
	return nil
}

func SaveSuspectAndTriggerCulpritVerification(ctx context.Context, tfa *model.TestFailureAnalysis, nsa *model.TestNthSectionAnalysis, commit *pb.BlameListSingleCommit) error {
	// Save nthsection result to datastore.
	_, err := saveSuspectAndUpdateNthSection(ctx, tfa, nsa, commit)
	if err != nil {
		return errors.Annotate(err, "store nthsection culprit to datastore").Err()
	}
	enabled, err := IsEnabled(ctx, tfa.Project)
	if err != nil {
		return errors.Annotate(err, "is enabled").Err()
	}
	if !enabled {
		logging.Infof(ctx, "Bisection not enabled")
		// If not enabled, consider analysis ended.
		err = testfailureanalysis.UpdateAnalysisStatus(ctx, tfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_ENDED)
		if err != nil {
			return errors.Annotate(err, "update analysis status").Err()
		}
		return nil
	}
	if err := task.ScheduleTestFailureTask(ctx, tfa.ID); err != nil {
		// Non-critical, just log the error
		err := errors.Annotate(err, "schedule culprit verification task %d", tfa.ID).Err()
		logging.Errorf(ctx, err.Error())
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
		CommitTime:         blCommit.GetCommitTime().AsTime(),
	}
	err = datastore.Put(ctx, suspect)
	if err != nil {
		return nil, errors.Annotate(err, "save suspect").Err()
	}

	err = SaveNthSectionAnalysis(ctx, nsa, func(nsa *model.TestNthSectionAnalysis) {
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

// SaveNthSectionAnalysis updates nthsection analysis in a way that avoid race condition
// if another thread also update the analysis.
func SaveNthSectionAnalysis(ctx context.Context, nsa *model.TestNthSectionAnalysis, updateFunc func(*model.TestNthSectionAnalysis)) error {
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

func TriggerRerunBuildForCommits(ctx context.Context, tfa *model.TestFailureAnalysis, nsa *model.TestNthSectionAnalysis, projectBisector projectbisector.ProjectBisector, commitHashes []string, option projectbisector.RerunOption) error {
	// Get test failure bundle
	bundle, err := datastoreutil.GetTestFailureBundle(ctx, tfa)
	if err != nil {
		return errors.Annotate(err, "get test failure bundle").Err()
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
			return errors.Annotate(err, "trigger rerun for commit %s", commitHash).Err()
		}
		_, err = CreateTestRerunModel(ctx, CreateRerunModelOptions{
			TestFailureAnalysis:   tfa,
			NthSectionAnalysisKey: datastore.KeyForObj(ctx, nsa),
			TestFailures:          tfs,
			Build:                 build,
			RerunType:             model.RerunBuildType_NthSection,
		})
		if err != nil {
			return errors.Annotate(err, "create test rerun model for build %d", build.GetId()).Err()
		}
	}
	return nil
}

type CreateRerunModelOptions struct {
	TestFailureAnalysis   *model.TestFailureAnalysis
	NthSectionAnalysisKey *datastore.Key
	SuspectKey            *datastore.Key
	TestFailures          []*model.TestFailure
	Build                 *bbpb.Build
	RerunType             model.RerunBuildType
}

func CreateTestRerunModel(ctx context.Context, options CreateRerunModelOptions) (*model.TestSingleRerun, error) {
	build := options.Build
	dimensions, err := buildbucket.GetBuildTaskDimension(ctx, build.GetId())
	if err != nil {
		return nil, errors.Annotate(err, "get build task dimension bbid %v", build.GetId()).Err()
	}
	testResults := model.RerunTestResults{}
	for _, tf := range options.TestFailures {
		testResults.Results = append(testResults.Results, model.RerunSingleTestResult{
			TestFailureKey: datastore.KeyForObj(ctx, tf),
		})
	}

	rerun := &model.TestSingleRerun{
		ID: build.GetId(),
		LUCIBuild: model.LUCIBuild{
			BuildID:     build.GetId(),
			Project:     build.Builder.Project,
			Bucket:      build.Builder.Bucket,
			Builder:     build.Builder.Builder,
			BuildNumber: int(build.Number),
			GitilesCommit: &bbpb.GitilesCommit{
				Host:    build.Input.GitilesCommit.Host,
				Project: build.Input.GitilesCommit.Project,
				Id:      build.Input.GitilesCommit.Id,
				Ref:     build.Input.GitilesCommit.Ref,
			},
			Status:     build.Status,
			CreateTime: build.CreateTime.AsTime(),
			StartTime:  build.StartTime.AsTime(),
		},
		Type:                  options.RerunType,
		AnalysisKey:           datastore.KeyForObj(ctx, options.TestFailureAnalysis),
		CulpritKey:            options.SuspectKey,
		NthSectionAnalysisKey: options.NthSectionAnalysisKey,
		Status:                pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
		Dimensions:            dimensions,
		Priority:              options.TestFailureAnalysis.Priority,
		TestResults:           testResults,
	}
	if err := datastore.Put(ctx, rerun); err != nil {
		return nil, err
	}
	return rerun, nil
}

func CreateSnapshot(ctx context.Context, nsa *model.TestNthSectionAnalysis) (*nthsectionsnapshot.Snapshot, error) {
	// Get all reruns for the current analysis.
	// We sort by create_time to make sure the latest rerun for a commit position
	// is considered.
	// If we retry INFRA_FAILURE rerun, we want to take the status of the retry,
	// instead of the INFRA_FAILURE status.
	reruns, err := datastoreutil.GetTestNthSectionReruns(ctx, nsa)
	if err != nil {
		return nil, errors.Annotate(err, "getting test nthsection rerun").Err()
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

func GetProjectBisector(ctx context.Context, tfa *model.TestFailureAnalysis) (projectbisector.ProjectBisector, error) {
	switch tfa.Project {
	case "chromium":
		bisector := &chromium.Bisector{}
		return bisector, nil
	default:
		return nil, errors.Reason("no bisector for project %s", tfa.Project).Err()
	}
}

func createNthSectionModel(ctx context.Context, tfa *model.TestFailureAnalysis, primaryTestFailure *model.TestFailure) (*model.TestNthSectionAnalysis, error) {
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
		return nil, errors.Annotate(err, "couldn't fetch changelog").Err()
	}
	blameList := changelogutil.ChangeLogsToBlamelist(ctx, changeLogs)
	if err := changelogutil.SetCommitPositionInBlamelist(blameList, primaryTestFailure.RegressionStartPosition, primaryTestFailure.RegressionEndPosition); err != nil {
		return nil, errors.Annotate(err, "set commit position in blamelist").Err()
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
		return nil, errors.Annotate(err, "save nthsection").Err()
	}
	return nsa, nil
}

func IsEnabled(ctx context.Context, project string) (bool, error) {
	cfg, err := config.Project(ctx, project)
	if err != nil {
		return false, err
	}
	return cfg.TestAnalysisConfig.GetBisectorEnabled(), nil
}
