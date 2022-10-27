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

// package culpritverification verifies if a suspect is a culprit.
package culpritverification

import (
	"context"
	"fmt"

	"go.chromium.org/luci/bisection/compilefailureanalysis/heuristic"
	"go.chromium.org/luci/bisection/internal/gitiles"
	gfim "go.chromium.org/luci/bisection/model"
	"go.chromium.org/luci/bisection/rerun"
	"go.chromium.org/luci/bisection/util/datastoreutil"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
)

// VerifySuspect verifies if a suspect is indeed the culprit.
// analysisID is CompileFailureAnalysis ID. It is meant to be propagated all the way to the
// recipe, so we can identify the analysis in buildbucket.
func VerifySuspect(c context.Context, suspect *gfim.Suspect, failedBuildID int64, analysisID int64) error {
	logging.Infof(c, "Verifying suspect %d for build %d", datastore.KeyForObj(c, suspect).IntID(), failedBuildID)

	// Get failed compile targets
	compileFailure, err := datastoreutil.GetCompileFailureForAnalysis(c, analysisID)
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

	// Get rerun build property
	props := map[string]interface{}{
		"analysis_id":    analysisID,
		"bisection_host": fmt.Sprintf("%s.appspot.com", info.AppID(c)),
	}
	if len(failedTargets) > 0 {
		props["compile_targets"] = failedTargets
	}

	// Verify the suspect
	suspectBuild, parentBuild, err := VerifyCommit(c, &suspect.GitilesCommit, failedBuildID, props)
	if err != nil {
		logging.Errorf(c, "Error triggering rerun for build %d: %s", failedBuildID, err)
		return err
	}
	suspectRerunBuildModel, err := rerun.CreateRerunBuildModel(c, suspectBuild, gfim.RerunBuildType_CulpritVerification, suspect, nil)
	if err != nil {
		return err
	}

	parentRerunBuildModel, err := rerun.CreateRerunBuildModel(c, parentBuild, gfim.RerunBuildType_CulpritVerification, suspect, nil)
	if err != nil {
		return err
	}

	suspect.VerificationStatus = gfim.SuspectVerificationStatus_UnderVerification
	suspect.SuspectRerunBuild = datastore.KeyForObj(c, suspectRerunBuildModel)
	suspect.ParentRerunBuild = datastore.KeyForObj(c, parentRerunBuildModel)
	err = datastore.Put(c, suspect)
	if err != nil {
		return err
	}
	return nil
}

func hasNewTarget(c context.Context, failedFiles []string, changelog *gfim.ChangeLog) bool {
	for _, file := range failedFiles {
		for _, diff := range changelog.ChangeLogDiffs {
			if diff.Type == gfim.ChangeType_ADD || diff.Type == gfim.ChangeType_COPY || diff.Type == gfim.ChangeType_RENAME {
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
func VerifyCommit(c context.Context, commit *buildbucketpb.GitilesCommit, failedBuildID int64, props map[string]interface{}) (*buildbucketpb.Build, *buildbucketpb.Build, error) {
	// Query Gitiles to get parent commit
	repoUrl := gitiles.GetRepoUrl(c, commit)
	p, err := gitiles.GetParentCommit(c, repoUrl, commit.Id)
	if err != nil {
		return nil, nil, err
	}
	parentCommit := &buildbucketpb.GitilesCommit{
		Host:    commit.Host,
		Project: commit.Project,
		Ref:     commit.Ref,
		Id:      p,
	}

	// Trigger a rerun with commit and parent commit
	build1, err := rerun.TriggerRerun(c, commit, failedBuildID, props)
	if err != nil {
		return nil, nil, err
	}

	build2, err := rerun.TriggerRerun(c, parentCommit, failedBuildID, props)
	if err != nil {
		return nil, nil, err
	}

	return build1, build2, nil
}
