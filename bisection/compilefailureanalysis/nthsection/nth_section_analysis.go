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

// Package nthsection performs nthsection analysis.
package nthsection

import (
	"context"
	"fmt"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/bisection/compilefailureanalysis/statusupdater"
	"go.chromium.org/luci/bisection/culpritverification"
	"go.chromium.org/luci/bisection/hosts"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/nthsectionsnapshot"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/rerun"
	taskpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/changelogutil"
	"go.chromium.org/luci/bisection/util/datastoreutil"
)

func Analyze(
	c context.Context,
	cfa *model.CompileFailureAnalysis) (*model.CompileNthSectionAnalysis, error) {
	logging.Infof(c, "Starting nthsection analysis.")
	// Create a new CompileNthSectionAnalysis Entity
	nsa := &model.CompileNthSectionAnalysis{
		ParentAnalysis: datastore.KeyForObj(c, cfa),
		StartTime:      clock.Now(c),
		Status:         pb.AnalysisStatus_RUNNING,
		RunStatus:      pb.AnalysisRunStatus_STARTED,
	}

	// We save the nthSectionAnalysis in updateBlameList below
	// but we save it here because we need the object in datastore for setStatusError
	err := datastore.Put(c, nsa)
	if err != nil {
		return nil, errors.Annotate(err, "couldn't save nthsection model").Err()
	}

	changeLogs, err := changelogutil.GetChangeLogs(c, cfa.InitialRegressionRange, true)
	if err != nil {
		setStatusError(c, nsa)
		return nil, errors.Annotate(err, "couldn't fetch changelog").Err()
	}

	err = updateBlameList(c, nsa, changeLogs)
	if err != nil {
		setStatusError(c, nsa)
		return nil, errors.Annotate(err, "couldn't update blamelist").Err()
	}

	err = startAnalysis(c, nsa, cfa)
	if err != nil {
		setStatusError(c, nsa)
		return nil, errors.Annotate(err, "couldn't start analysis %d", cfa.Id).Err()
	}
	return nsa, nil
}

// startAnalysis will based on find next commit(s) for rerun and schedule them
func startAnalysis(c context.Context, nsa *model.CompileNthSectionAnalysis, cfa *model.CompileFailureAnalysis) error {
	logging.Infof(c, "Starting nthsection reruns.")
	snapshot, err := CreateSnapshot(c, nsa)
	if err != nil {
		return err
	}

	// When there is only 1 commit in the blamelist, no reruns are triggered.
	// In this case, we should save the culprit and trigger culprit verification.
	ok, cul := snapshot.GetCulprit()

	if ok {
		err := SaveSuspectAndTriggerCulpritVerification(c, nsa, cfa, snapshot.BlameList.Commits[cul])
		if err != nil {
			return errors.Annotate(err, "save suspect and trigger culprit verification").Err()
		}
		return nil
	}

	// maxRerun controls how many rerun we can run at a time
	// Its value depends on the importance of the analysis, availability of bots etc...
	// For now, hard-code it to run bisection
	maxRerun := 1 // bisection
	commits, err := snapshot.FindNextCommitsToRun(maxRerun)
	if err != nil {
		return errors.Annotate(err, "couldn't find commits to run").Err()
	}

	for _, commit := range commits {
		gitilesCommit := &bbpb.GitilesCommit{
			Host:    cfa.InitialRegressionRange.FirstFailed.Host,
			Project: cfa.InitialRegressionRange.FirstFailed.Project,
			Ref:     cfa.InitialRegressionRange.FirstFailed.Ref,
			Id:      commit,
		}
		err := RerunCommit(c, nsa, gitilesCommit, cfa.FirstFailedBuildId, nil)
		if err != nil {
			return errors.Annotate(err, "rerunCommit for %s", commit).Err()
		}
	}

	return nil
}

func SaveSuspectAndTriggerCulpritVerification(c context.Context, nsa *model.CompileNthSectionAnalysis, cfa *model.CompileFailureAnalysis, commit *pb.BlameListSingleCommit) error {
	suspect, err := storeNthSectionResultToDatastore(c, cfa, nsa, commit)
	if err != nil {
		return errors.Annotate(err, "storeNthSectionResultToDatastore").Err()
	}

	// Run culprit verification
	shouldRunCulpritVerification, err := culpritverification.ShouldRunCulpritVerification(c, cfa)
	if err != nil {
		return errors.Annotate(err, "couldn't fetch shouldRunCulpritVerification config").Err()
	}
	if shouldRunCulpritVerification {
		suspectID := nsa.Suspect.IntID()
		err = tq.AddTask(c, &tq.Task{
			Title: fmt.Sprintf("culprit_verification_%d_%d", cfa.Id, suspectID),
			Payload: &taskpb.CulpritVerificationTask{
				SuspectId:  suspectID,
				AnalysisId: cfa.Id,
				ParentKey:  nsa.Suspect.Parent().Encode(),
			},
		})
		if err != nil {
			// Non-critical, just log the error
			// TODO (nqmtuan): Update the analysis to ended, and update the suspect.
			err := errors.Annotate(err, "schedule culprit verification task %d_%d", cfa.Id, suspectID).Err()
			logging.Errorf(c, err.Error())
		}
		// Update suspect verification status
		err = datastore.RunInTransaction(c, func(c context.Context) error {
			e := datastore.Get(c, suspect)
			if e != nil {
				return e
			}
			suspect.VerificationStatus = model.SuspectVerificationStatus_VerificationScheduled
			return datastore.Put(c, suspect)
		}, nil)
		if err != nil {
			// Non-critical, just log the error
			err := errors.Annotate(err, "saving suspect").Err()
			logging.Errorf(c, err.Error())
		}
	}
	return nil
}

func storeNthSectionResultToDatastore(c context.Context, cfa *model.CompileFailureAnalysis, nsa *model.CompileNthSectionAnalysis, blCommit *pb.BlameListSingleCommit) (*model.Suspect, error) {
	suspect := &model.Suspect{
		Type: model.SuspectType_NthSection,
		GitilesCommit: bbpb.GitilesCommit{
			Host:    cfa.InitialRegressionRange.FirstFailed.Host,
			Project: cfa.InitialRegressionRange.FirstFailed.Project,
			Ref:     cfa.InitialRegressionRange.FirstFailed.Ref,
			Id:      blCommit.Commit,
		},
		ParentAnalysis:     datastore.KeyForObj(c, nsa),
		VerificationStatus: model.SuspectVerificationStatus_Unverified,
		ReviewUrl:          blCommit.ReviewUrl,
		ReviewTitle:        blCommit.ReviewTitle,
		AnalysisType:       pb.AnalysisType_COMPILE_FAILURE_ANALYSIS,
	}
	err := datastore.Put(c, suspect)
	if err != nil {
		return nil, errors.Annotate(err, "couldn't save suspect").Err()
	}

	err = datastore.RunInTransaction(c, func(ctx context.Context) error {
		e := datastore.Get(c, nsa)
		if e != nil {
			return e
		}
		nsa.Status = pb.AnalysisStatus_SUSPECTFOUND
		nsa.Suspect = datastore.KeyForObj(c, suspect)
		nsa.RunStatus = pb.AnalysisRunStatus_ENDED
		nsa.EndTime = clock.Now(c)
		return datastore.Put(c, nsa)
	}, nil)

	if err != nil {
		return nil, errors.Annotate(err, "couldn't save nthsection analysis").Err()
	}

	err = statusupdater.UpdateStatus(c, cfa, pb.AnalysisStatus_SUSPECTFOUND, pb.AnalysisRunStatus_STARTED)
	if err != nil {
		return nil, errors.Annotate(err, "couldn't save analysis").Err()
	}

	return suspect, nil
}

func RerunCommit(c context.Context, nsa *model.CompileNthSectionAnalysis, commit *bbpb.GitilesCommit, failedBuildID int64, dims map[string]string) error {
	props, err := getRerunProps(c, nsa)
	if err != nil {
		return errors.Annotate(err, "failed getting rerun props").Err()
	}

	priority, err := getRerunPriority(c, nsa, commit, dims)
	if err != nil {
		return errors.Annotate(err, "couldn't getRerunPriority").Err()
	}

	// TODO(nqmtuan): Pass in the project.
	// For now, hardcode to "chromium", since we only support chromium for compile failure.
	builder, err := config.GetCompileBuilder(c, "chromium")
	if err != nil {
		return errors.Annotate(err, "get compile builder").Err()
	}
	options := &rerun.TriggerOptions{
		Builder:         util.BuilderFromConfigBuilder(builder),
		GitilesCommit:   commit,
		SampleBuildID:   failedBuildID,
		ExtraProperties: props,
		ExtraDimensions: dims,
		Priority:        priority,
	}
	build, err := rerun.TriggerRerun(c, options)
	if err != nil {
		return errors.Annotate(err, "couldn't trigger rerun").Err()
	}

	_, err = rerun.CreateRerunBuildModel(c, build, model.RerunBuildType_NthSection, nil, nsa, priority)
	if err != nil {
		return errors.Annotate(err, "createRerunBuildModel").Err()
	}

	return nil
}

func getRerunPriority(c context.Context, nsa *model.CompileNthSectionAnalysis, commit *bbpb.GitilesCommit, dims map[string]string) (int32, error) {
	// TODO (nqmtuan): Add other priority offset
	var pri int32 = rerun.PriorityNthSection
	// If targetting a particular bot
	if _, ok := dims["id"]; ok {
		pri += rerun.PriorityScheduleOnSameBotOffset
	}

	// Offset the priority based on run duration
	cfa, err := datastoreutil.GetCompileFailureAnalysis(c, nsa.ParentAnalysis.IntID())
	if err != nil {
		return 0, errors.Annotate(err, "couldn't get analysis for nthsection %d", nsa.Id).Err()
	}
	pri, err = rerun.OffsetPriorityBasedOnRunDuration(c, pri, cfa)
	if err != nil {
		return 0, errors.Annotate(err, "couldn't OffsetPriorityBasedOnRunDuration analysis %d", cfa.Id).Err()
	}

	// Offset the priority if it is a tree closer
	if cfa.IsTreeCloser {
		pri += rerun.PriorityTreeClosureOffset
	}

	return rerun.CapPriority(pri), nil
}

func getRerunProps(c context.Context, nthSectionAnalysis *model.CompileNthSectionAnalysis) (map[string]any, error) {
	analysisID := nthSectionAnalysis.ParentAnalysis.IntID()
	compileFailure, err := datastoreutil.GetCompileFailureForAnalysisID(c, analysisID)
	if err != nil {
		return nil, errors.Annotate(err, "get compile failure for analysis %d", analysisID).Err()
	}
	// TODO (nqmtuan): Handle the case where the failed compile targets are newly added.
	// So any commits before the culprits cannot run the targets.
	// In such cases, we should detect from the recipe side
	failedTargets := compileFailure.OutputTargets

	host, err := hosts.APIHost(c)
	if err != nil {
		return nil, errors.Annotate(err, "get bisection API Host").Err()
	}

	props := map[string]any{
		"analysis_id":    analysisID,
		"bisection_host": host,
	}
	if len(failedTargets) > 0 {
		props["compile_targets"] = failedTargets
	}

	return props, nil
}

func updateBlameList(c context.Context, nthSectionAnalysis *model.CompileNthSectionAnalysis, changeLogs []*model.ChangeLog) error {
	logging.Infof(c, "Update blame list for nthsection. Change logs have %d items.", len(changeLogs))
	blameList := changelogutil.ChangeLogsToBlamelist(c, changeLogs)
	nthSectionAnalysis.BlameList = blameList
	return datastore.Put(c, nthSectionAnalysis)
}

func ShouldRunNthSectionAnalysis(c context.Context, cfa *model.CompileFailureAnalysis) (bool, error) {
	project, err := datastoreutil.GetProjectForCompileFailureAnalysis(c, cfa)
	if err != nil {
		return false, errors.Annotate(err, "get project for compile failure analysis").Err()
	}
	cfg, err := config.Project(c, project)
	if err != nil {
		return false, errors.Annotate(err, "config project").Err()
	}
	return cfg.CompileAnalysisConfig.NthsectionEnabled, nil
}

func setStatusError(c context.Context, nsa *model.CompileNthSectionAnalysis) {
	// if cannot set status error, just log the error here, because the calls
	// are made from an error block
	err := datastore.RunInTransaction(c, func(c context.Context) error {
		e := datastore.Get(c, nsa)
		if e != nil {
			return e
		}
		nsa.Status = pb.AnalysisStatus_ERROR
		nsa.EndTime = clock.Now(c)
		nsa.RunStatus = pb.AnalysisRunStatus_ENDED
		return datastore.Put(c, nsa)
	}, nil)

	if err != nil {
		err = errors.Annotate(err, "couldn't setStatusError for nthsection analysis %d", nsa.ParentAnalysis.IntID()).Err()
		logging.Errorf(c, err.Error())
	}
}

func CreateSnapshot(c context.Context, nthSectionAnalysis *model.CompileNthSectionAnalysis) (*nthsectionsnapshot.Snapshot, error) {
	// Get all reruns for the current analysis
	// This should contain all reruns for nth section and culprit verification
	q := datastore.NewQuery("SingleRerun").Eq("analysis", nthSectionAnalysis.ParentAnalysis).Order("start_time")
	reruns := []*model.SingleRerun{}
	err := datastore.GetAll(c, q, &reruns)
	if err != nil {
		return nil, errors.Annotate(err, "getting all reruns").Err()
	}

	snapshot := &nthsectionsnapshot.Snapshot{
		BlameList:      nthSectionAnalysis.BlameList,
		Runs:           []*nthsectionsnapshot.Run{},
		NumInfraFailed: 0,
	}

	statusMap := map[string]pb.RerunStatus{}
	typeMap := map[string]model.RerunBuildType{}
	for _, r := range reruns {
		statusMap[r.GitilesCommit.GetId()] = r.Status
		typeMap[r.GitilesCommit.GetId()] = r.Type
		if r.Status == pb.RerunStatus_RERUN_STATUS_INFRA_FAILED {
			snapshot.NumInfraFailed++
		}
		if r.Status == pb.RerunStatus_RERUN_STATUS_IN_PROGRESS {
			snapshot.NumInProgress++
		}
	}

	blamelist := nthSectionAnalysis.BlameList
	for index, cl := range blamelist.Commits {
		if stat, ok := statusMap[cl.Commit]; ok {
			snapshot.Runs = append(snapshot.Runs, &nthsectionsnapshot.Run{
				Index:  index,
				Commit: cl.Commit,
				Status: stat,
				Type:   typeMap[cl.Commit],
			})
		}
	}
	return snapshot, nil
}
