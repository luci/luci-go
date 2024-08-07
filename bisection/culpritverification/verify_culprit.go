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

// Package culpritverification verifies if a suspect is a culprit.
package culpritverification

import (
	"context"

	"google.golang.org/protobuf/proto"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/compilefailureanalysis/heuristic"
	"go.chromium.org/luci/bisection/compilefailureanalysis/statusupdater"
	cpvt "go.chromium.org/luci/bisection/culpritverification/task"
	"go.chromium.org/luci/bisection/hosts"
	"go.chromium.org/luci/bisection/internal/config"
	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/bisection/rerun"
	taskpb "go.chromium.org/luci/bisection/task/proto"
	"go.chromium.org/luci/bisection/util"
	"go.chromium.org/luci/bisection/util/datastoreutil"
	"go.chromium.org/luci/bisection/util/loggingutil"
)

// RegisterTaskClass registers the task class for tq dispatcher
func RegisterTaskClass() {
	compileHandler := func(ctx context.Context, payload proto.Message) error {
		task := payload.(*taskpb.CulpritVerificationTask)
		analysisID := task.GetAnalysisId()
		suspectID := task.GetSuspectId()
		parentKey := task.GetParentKey()
		return handleTQError(ctx, processCulpritVerificationTask(ctx, analysisID, suspectID, parentKey))
	}
	testHandler := func(ctx context.Context, payload proto.Message) error {
		task := payload.(*taskpb.TestFailureCulpritVerificationTask)
		return handleTQError(ctx, processTestFailureTask(ctx, task))
	}
	cpvt.RegisterTaskClass(compileHandler, testHandler)
}

func handleTQError(ctx context.Context, err error) error {
	if err != nil {
		err := errors.Annotate(err, "run culprit verification").Err()
		logging.Errorf(ctx, err.Error())
		// If the error is transient, return err to retry
		if transient.Tag.In(err) {
			return err
		}
		return nil
	}
	return nil
}

func processCulpritVerificationTask(c context.Context, analysisID int64, suspectID int64, parentKeyStr string) error {
	c, err := loggingutil.UpdateLoggingWithAnalysisID(c, analysisID)
	if err != nil {
		// not critical, just log
		err := errors.Annotate(err, "failed UpdateLoggingWithAnalysisID %d", analysisID)
		logging.Errorf(c, "%v", err)
	}

	cfa, err := datastoreutil.GetCompileFailureAnalysis(c, analysisID)
	if err != nil {
		return errors.Annotate(err, "failed getting CompileFailureAnalysis").Err()
	}

	parentKey, err := datastore.NewKeyEncoded(parentKeyStr)
	if err != nil {
		return errors.Annotate(err, "couldn't decode parent key for suspect").Err()
	}

	suspect, err := datastoreutil.GetSuspect(c, suspectID, parentKey)
	if err != nil {
		return errors.Annotate(err, "couldn't get suspect").Err()
	}
	return VerifySuspect(c, suspect, cfa.FirstFailedBuildId, analysisID)
}

// VerifySuspect verifies if a suspect is indeed the culprit.
// analysisID is CompileFailureAnalysis ID. It is meant to be propagated all the way to the
// recipe, so we can identify the analysis in buildbucket.
func VerifySuspect(c context.Context, suspect *model.Suspect, failedBuildID int64, analysisID int64) error {
	logging.Infof(c, "Verifying suspect %d for build %d", datastore.KeyForObj(c, suspect).IntID(), failedBuildID)

	// Check if the analysis has found any culprits, if yes, exit early
	cfa, err := datastoreutil.GetCompileFailureAnalysis(c, analysisID)
	if err != nil {
		return err
	}

	defer updateSuspectStatus(c, suspect, cfa)

	if len(cfa.VerifiedCulprits) > 0 {
		logging.Infof(c, "culprit found for analysis %d, no need to trigger any verification runs", analysisID)
		return nil
	}

	// Check if there is any suspect with the same commit being verified
	// If yes, we don't run verification for this suspect anymore
	suspectExist, err := checkSuspectWithSameCommitExist(c, cfa, suspect)
	if err != nil {
		return errors.Annotate(err, "checkSuspectWithSameCommitExist").Err()
	}
	if suspectExist {
		return nil
	}

	// Get failed compile targets
	compileFailure, err := datastoreutil.GetCompileFailureForAnalysisID(c, analysisID)
	if err != nil {
		return err
	}
	failedTargets := compileFailure.OutputTargets

	// Get the changelog for the suspect
	repoURL := gitiles.GetRepoUrl(c, &suspect.GitilesCommit)
	changeLogs, err := gitiles.GetChangeLogsForSingleRevision(c, repoURL, suspect.GitilesCommit.Id)
	if err != nil {
		// This is non-critical, we just log and continue
		logging.Errorf(c, "Cannot get changelog for revision %s: %s", suspect.GitilesCommit.Id, err)
	} else {
		// Check if any failed files is newly added in the change log.
		// If it is the case, the parent revision cannot compile failed targets.
		// In such cases, we do not pass the failed targets to recipe, instead
		// we will compile all targets.
		if hasNewTarget(c, compileFailure.FailedFiles, changeLogs) {
			failedTargets = []string{}
		}
	}

	host, err := hosts.APIHost(c)
	if err != nil {
		return errors.Annotate(err, "get bisection API Host").Err()
	}

	// Get rerun build property
	props := map[string]any{
		"analysis_id":    analysisID,
		"bisection_host": host,
		// For culprit verification, we should remove builder cache
		"should_clobber": true,
	}
	if len(failedTargets) > 0 {
		props["compile_targets"] = failedTargets
	}

	// Verify the suspect
	priority, err := getSuspectPriority(c, suspect)
	if err != nil {
		return errors.Annotate(err, "failed getting priority").Err()
	}

	// TODO(nqmtuan): Pass in the project.
	// For now, hardcode to "chromium", since we only support chromium for compile failure.
	suspectBuild, parentBuild, err := VerifySuspectCommit(c, "chromium", suspect, failedBuildID, props, priority)
	if err != nil {
		logging.Errorf(c, "Error triggering rerun for build %d: %s", failedBuildID, err)
		return err
	}
	suspectRerunBuildModel, err := rerun.CreateRerunBuildModel(c, suspectBuild, model.RerunBuildType_CulpritVerification, suspect, nil, priority)
	if err != nil {
		return err
	}

	parentRerunBuildModel, err := rerun.CreateRerunBuildModel(c, parentBuild, model.RerunBuildType_CulpritVerification, suspect, nil, priority)
	if err != nil {
		return err
	}

	err = datastore.RunInTransaction(c, func(ctx context.Context) error {
		e := datastore.Get(c, suspect)
		if e != nil {
			return e
		}
		suspect.VerificationStatus = model.SuspectVerificationStatus_UnderVerification
		suspect.SuspectRerunBuild = datastore.KeyForObj(c, suspectRerunBuildModel)
		suspect.ParentRerunBuild = datastore.KeyForObj(c, parentRerunBuildModel)
		return datastore.Put(c, suspect)
	}, nil)

	if err != nil {
		return err
	}
	return nil
}

func checkSuspectWithSameCommitExist(c context.Context, cfa *model.CompileFailureAnalysis, suspect *model.Suspect) (bool, error) {
	suspects, err := datastoreutil.FetchSuspectsForAnalysis(c, cfa)
	if err != nil {
		return false, errors.Annotate(err, "fetchSuspectsForAnalysis").Err()
	}
	for _, s := range suspects {
		// Need to be of different suspect
		if s.Id != suspect.Id {
			if s.GitilesCommit.Id == suspect.GitilesCommit.Id {
				if s.VerificationStatus != model.SuspectVerificationStatus_Unverified {
					return true, nil
				}
			}
		}
	}
	return false, nil
}

func hasNewTarget(c context.Context, failedFiles []string, changelog *model.ChangeLog) bool {
	for _, file := range failedFiles {
		for _, diff := range changelog.ChangeLogDiffs {
			if diff.Type == model.ChangeType_ADD || diff.Type == model.ChangeType_COPY || diff.Type == model.ChangeType_RENAME {
				if heuristic.IsSameFile(diff.NewPath, file) {
					return true
				}
			}
		}
	}
	return false
}

// VerifyCommit checks if a commit is the culprit of a build failure.
// Returns 2 builds:
// - The 1st build is the rerun build for the commit
// - The 2nd build is the rerun build for the parent commit
func VerifySuspectCommit(c context.Context, project string, suspect *model.Suspect, failedBuildID int64, props map[string]any, priority int32) (*buildbucketpb.Build, *buildbucketpb.Build, error) {
	commit := &suspect.GitilesCommit

	// Query Gitiles to get parent commit
	parentCommit, err := getParentCommit(c, commit)
	if err != nil {
		return nil, nil, errors.Annotate(err, "get parent commit for commit %s", commit.Id).Err()
	}
	builder, err := config.GetCompileBuilder(c, project)
	if err != nil {
		return nil, nil, errors.Annotate(err, "get compile builder").Err()
	}
	options := &rerun.TriggerOptions{
		Builder:         util.BuilderFromConfigBuilder(builder),
		GitilesCommit:   commit,
		SampleBuildID:   failedBuildID,
		ExtraProperties: props,
		ExtraDimensions: nil,
		Priority:        priority,
	}
	// Trigger a rerun with commit and parent commit
	build1, err := rerun.TriggerRerun(c, options)
	if err != nil {
		return nil, nil, err
	}

	options.GitilesCommit = parentCommit
	build2, err := rerun.TriggerRerun(c, options)
	if err != nil {
		return nil, nil, err
	}

	return build1, build2, nil
}

func getSuspectPriority(c context.Context, suspect *model.Suspect) (int32, error) {
	// TODO (nqmtuan): Support priority for nth-section case
	// For now let's return the baseline for culprit verification
	// We can add offset later
	confidence := heuristic.GetConfidenceLevel(suspect.Score)
	var pri int32 = 0
	switch confidence {
	case pb.SuspectConfidenceLevel_HIGH:
		pri = rerun.PriorityCulpritVerificationHighConfidence
	case pb.SuspectConfidenceLevel_MEDIUM:
		pri = rerun.PriorityCulpritVerificationMediumConfidence
	case pb.SuspectConfidenceLevel_LOW:
		pri = rerun.PriorityCulpritVerificationLowConfidence
	}

	// Check if the same suspect has any running build
	otherSuspects, err := datastoreutil.GetOtherSuspectsWithSameCL(c, suspect)
	if err != nil {
		return 0, errors.Annotate(err, "failed GetOtherSuspectsWithSameCL %d", suspect.Id).Err()
	}

	// If there is a running/finished suspect run -> lower priority of this run
	for _, s := range otherSuspects {
		if s.VerificationStatus == model.SuspectVerificationStatus_UnderVerification || s.VerificationStatus == model.SuspectVerificationStatus_ConfirmedCulprit || s.VerificationStatus == model.SuspectVerificationStatus_Vindicated {
			pri += rerun.PriorityAnotherVerificationBuildExistOffset
			break
		}
	}

	// Offset the priority based on run duration
	cfa, err := datastoreutil.GetCompileFailureAnalysis(c, suspect.ParentAnalysis.Parent().IntID())
	if err != nil {
		return 0, errors.Annotate(err, "couldn't get analysis for suspect %d", suspect.Id).Err()
	}
	pri, err = rerun.OffsetPriorityBasedOnRunDuration(c, pri, cfa)
	if err != nil {
		return 0, errors.Annotate(err, "couldn't OffsetPriorityBasedOnRunDuration for suspect %d", suspect.Id).Err()
	}

	// Offset the priority if it is a tree closer
	if cfa.IsTreeCloser {
		pri += rerun.PriorityTreeClosureOffset
	}

	return rerun.CapPriority(pri), nil
}

func updateSuspectStatus(c context.Context, suspect *model.Suspect, cfa *model.CompileFailureAnalysis) {
	// If after VerifySuspect, the suspect verification status is not
	// SuspectVerificationStatus_UnderVerification, it means no reruns have been scheduled
	// so we should set the status back to SuspectVerificationStatus_Unverified
	if suspect.VerificationStatus != model.SuspectVerificationStatus_UnderVerification {
		err := datastore.RunInTransaction(c, func(c context.Context) error {
			// Update suspect status
			e := datastore.Get(c, suspect)
			if e != nil {
				return e
			}
			suspect.VerificationStatus = model.SuspectVerificationStatus_Unverified
			return datastore.Put(c, suspect)
		}, nil)

		if err != nil {
			logging.Errorf(c, errors.Annotate(err, "set suspect verification status").Err().Error())
		}
		// Also update the analysis status this case, because
		// the analysis may ended, given the suspect is no longer under verification
		err = statusupdater.UpdateAnalysisStatus(c, cfa)
		if err != nil {
			logging.Errorf(c, errors.Annotate(err, "set analysis status").Err().Error())
		}
	}
}

func ShouldRunCulpritVerification(c context.Context, cfa *model.CompileFailureAnalysis) (bool, error) {
	project, err := datastoreutil.GetProjectForCompileFailureAnalysis(c, cfa)
	if err != nil {
		return false, errors.Annotate(err, "get project for compile failure analysis").Err()
	}
	cfg, err := config.Project(c, project)
	if err != nil {
		return false, errors.Annotate(err, "config project").Err()
	}
	return cfg.CompileAnalysisConfig.CulpritVerificationEnabled, nil
}

func getParentCommit(ctx context.Context, commit *buildbucketpb.GitilesCommit) (*buildbucketpb.GitilesCommit, error) {
	repoURL := gitiles.GetRepoUrl(ctx, commit)
	p, err := gitiles.GetParentCommit(ctx, repoURL, commit.Id)
	if err != nil {
		return nil, err
	}
	return &buildbucketpb.GitilesCommit{
		Host:    commit.Host,
		Project: commit.Project,
		Ref:     commit.Ref,
		Id:      p,
	}, nil
}
