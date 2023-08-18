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
	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server"
	"go.chromium.org/luci/server/tq"
	"google.golang.org/protobuf/proto"

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
func RegisterTaskClass(srv *server.Server, luciAnalysisProject string) error {
	ctx := srv.Context
	client, err := lucianalysis.NewClient(ctx, srv.Options.CloudProject, luciAnalysisProject)
	if err != nil {
		return err
	}
	srv.RegisterCleanup(func(context.Context) {
		client.Close()
	})
	handler := func(ctx context.Context, payload proto.Message) error {
		task := payload.(*tpb.TestFailureBisectionTask)
		analysisID := task.GetAnalysisId()
		loggingutil.SetAnalysisID(ctx, analysisID)
		logging.Infof(ctx, "Processing test failure bisection task with id = %d", analysisID)
		err := Run(ctx, analysisID, client)
		if err != nil {
			err = errors.Annotate(err, "run bisection").Err()
			logging.Errorf(ctx, err.Error())
			// If the error is transient, return err to retry.
			if transient.Tag.In(err) {
				return err
			}
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
func Run(ctx context.Context, analysisID int64, luciAnalysis analysis.AnalysisClient) (reterr error) {
	// Retrieves analysis from datastore.
	tfa, err := datastoreutil.GetTestFailureAnalysis(ctx, analysisID)
	if err != nil {
		return errors.Annotate(err, "get test failure analysis").Err()
	}

	defer func() {
		if reterr != nil {
			// If there is an error, mark the analysis as failing with error.
			err := testfailureanalysis.UpdateStatus(ctx, tfa, pb.AnalysisStatus_ERROR, pb.AnalysisRunStatus_ENDED)
			if err != nil {
				// Just log the error if there is something wrong.
				err = errors.Annotate(err, "update status").Err()
				logging.Errorf(ctx, err.Error())
			}
		}
	}()

	// Checks if test failure analysis is enabled.
	enabled, err := isEnabled(ctx)
	if err != nil {
		return errors.Annotate(err, "is enabled").Err()
	}
	if !enabled {
		logging.Infof(ctx, "Bisection is not enabled")
		err = testfailureanalysis.UpdateStatus(ctx, tfa, pb.AnalysisStatus_DISABLED, pb.AnalysisRunStatus_ENDED)
		if err != nil {
			return errors.Annotate(err, "update status disabled").Err()
		}
		return nil
	}

	if tfa.Project != "chromium" {
		// We don't support other projects for now, so mark the analysis as unsupported.
		logging.Infof(ctx, "Unsupported project: %s", tfa.Project)
		// TODO (nqmtuan): Also update the Nthsection analysis status.
		err = testfailureanalysis.UpdateStatus(ctx, tfa, pb.AnalysisStatus_UNSUPPORTED, pb.AnalysisRunStatus_ENDED)
		if err != nil {
			return errors.Annotate(err, "update status unsupported").Err()
		}
		return
	}

	// Update the analysis status.
	err = testfailureanalysis.UpdateStatus(ctx, tfa, pb.AnalysisStatus_RUNNING, pb.AnalysisRunStatus_STARTED)
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

	projectBisector, err := getProjectBisector(ctx, tfa, luciAnalysis)
	if err != nil {
		return errors.Annotate(err, "get individual project bisector").Err()
	}

	err = projectBisector.Prepare(ctx, tfa)
	if err != nil {
		return errors.Annotate(err, "prepare").Err()
	}

	// TODO (nqmtuan): Run nthsection here
	snapshot, err := CreateSnapshot(ctx, nsa)
	if err != nil {
		return errors.Annotate(err, "create snapshot").Err()
	}

	// maxRerun controls how many rerun we can run at a time.
	// For now, hard-code it to run bisection.
	// TODO (nqmtuan): Tune it when we have information about bot availability.
	maxRerun := 1 // bisection
	commitHashes, err := snapshot.FindNextCommitsToRun(maxRerun)
	if err != nil {
		return errors.Annotate(err, "find next commits to run").Err()
	}

	// Get test failure bundle
	bundle, err := datastoreutil.GetTestFailureBundle(ctx, tfa)
	if err != nil {
		return errors.Annotate(err, "get test failure bundle").Err()
	}
	tfs := bundle.All()
	for _, commitHash := range commitHashes {
		gitilesCommit := &bbpb.GitilesCommit{
			Host:    primaryFailure.Ref.GetGitiles().GetHost(),
			Project: primaryFailure.Ref.GetGitiles().GetProject(),
			Ref:     primaryFailure.Ref.GetGitiles().GetRef(),
			Id:      commitHash,
		}
		build, err := projectBisector.TriggerRerun(ctx, tfa, tfs, gitilesCommit)
		if err != nil {
			return errors.Annotate(err, "trigger rerun for commit %s", commitHash).Err()
		}
		err = createTestRerunModel(ctx, tfa, nsa, tfs, build)
		if err != nil {
			return errors.Annotate(err, "create test rerun model for build %d", build.GetId()).Err()
		}
	}
	return nil
}

func createTestRerunModel(ctx context.Context, tfa *model.TestFailureAnalysis, nsa *model.TestNthSectionAnalysis, tfs []*model.TestFailure, build *bbpb.Build) error {
	dimensions, err := buildbucket.GetBuildTaskDimension(ctx, build.GetId())
	if err != nil {
		return errors.Annotate(err, "get build task dimension bbid %v", build.GetId()).Err()
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
				Host:     build.Input.GitilesCommit.Host,
				Project:  build.Input.GitilesCommit.Project,
				Id:       build.Input.GitilesCommit.Id,
				Ref:      build.Input.GitilesCommit.Ref,
				Position: build.Input.GitilesCommit.Position,
			},
			Status:     build.Status,
			CreateTime: build.CreateTime.AsTime(),
			StartTime:  build.StartTime.AsTime(),
		},
		Type:                  model.RerunBuildType_NthSection,
		AnalysisKey:           datastore.KeyForObj(ctx, tfa),
		NthSectionAnalysisKey: datastore.KeyForObj(ctx, nsa),
		Status:                pb.RerunStatus_RERUN_STATUS_IN_PROGRESS,
		Dimensions:            dimensions,
		Priority:              tfa.Priority,
	}
	return datastore.Put(ctx, rerun)
}

func CreateSnapshot(ctx context.Context, nsa *model.TestNthSectionAnalysis) (*nthsectionsnapshot.Snapshot, error) {
	// Get all reruns for the current analysis.
	// We sort by create_time to make sure the latest rerun for a commit position
	// is considered.
	// If we retry INFRA_FAILURE rerun, we want to take the status of the retry,
	// instead of the INFRA_FAILURE status.
	q := datastore.NewQuery("TestSingleRerun").Eq("nthsection_analysis_key", datastore.KeyForObj(ctx, nsa)).Order("luci_build.create_time")
	reruns := []*model.TestSingleRerun{}
	err := datastore.GetAll(ctx, q, &reruns)
	if err != nil {
		return nil, errors.Annotate(err, "getting all reruns").Err()
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

func getProjectBisector(ctx context.Context, tfa *model.TestFailureAnalysis, luciAnalysis analysis.AnalysisClient) (projectbisector.ProjectBisector, error) {
	switch tfa.Project {
	case "chromium":
		bisector := &chromium.Bisector{
			LuciAnalysis: luciAnalysis,
		}
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
	changeLogs, err := changelogutil.GetChangeLogs(ctx, regressionRange)
	if err != nil {
		return nil, errors.Annotate(err, "couldn't fetch changelog").Err()
	}
	blameList := changelogutil.ChangeLogsToBlamelist(ctx, changeLogs)

	nsa := &model.TestNthSectionAnalysis{
		ParentAnalysis: datastore.KeyForObj(ctx, tfa),
		StartTime:      clock.Now(ctx),
		Status:         pb.AnalysisStatus_RUNNING,
		RunStatus:      pb.AnalysisRunStatus_STARTED,
		BlameList:      blameList,
	}

	err = datastore.Put(ctx, nsa)
	if err != nil {
		return nil, errors.Annotate(err, "save nthsection").Err()
	}
	return nsa, nil
}

func isEnabled(ctx context.Context) (bool, error) {
	cfg, err := config.Get(ctx)
	if err != nil {
		return false, err
	}
	return cfg.TestAnalysisConfig.GetBisectorEnabled(), nil
}
