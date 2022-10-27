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

	lbm "go.chromium.org/luci/bisection/model"
	lbpb "go.chromium.org/luci/bisection/proto"
	"go.chromium.org/luci/bisection/rerun"
	"go.chromium.org/luci/bisection/util/changelogutil"
	"go.chromium.org/luci/bisection/util/datastoreutil"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
)

func Analyze(
	c context.Context,
	cfa *lbm.CompileFailureAnalysis) (*lbm.CompileNthSectionAnalysis, error) {
	// Create a new CompileNthSectionAnalysis Entity
	nthSectionAnalysis := &lbm.CompileNthSectionAnalysis{
		ParentAnalysis: datastore.KeyForObj(c, cfa),
		StartTime:      clock.Now(c),
		Status:         lbpb.AnalysisStatus_CREATED,
	}

	changeLogs, err := changelogutil.GetChangeLogs(c, cfa.InitialRegressionRange)
	if err != nil {
		logging.Infof(c, "Cannot fetch changelog for analysis %d", cfa.Id)
	}

	err = updateBlameList(c, nthSectionAnalysis, changeLogs)
	if err != nil {
		return nil, err
	}

	err = startAnalysis(c, nthSectionAnalysis, cfa)
	if err != nil {
		return nil, errors.Annotate(err, "couldn't start analysis %d", cfa.Id).Err()
	}
	return nthSectionAnalysis, nil
}

// startAnalysis will based on find next commit(s) for rerun and schedule them
func startAnalysis(c context.Context, nsa *lbm.CompileNthSectionAnalysis, cfa *lbm.CompileFailureAnalysis) error {
	snapshot, err := CreateSnapshot(c, nsa)
	if err != nil {
		return err
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
		gitilesCommit := &buildbucketpb.GitilesCommit{
			Host:    cfa.InitialRegressionRange.FirstFailed.Host,
			Project: cfa.InitialRegressionRange.FirstFailed.Project,
			Ref:     cfa.InitialRegressionRange.FirstFailed.Ref,
			Id:      commit,
		}
		build, err := rerunCommit(c, nsa, gitilesCommit, cfa.FirstFailedBuildId)
		if err != nil {
			return errors.Annotate(err, "rerunCommit for %s", commit).Err()
		}
		_, err = rerun.CreateRerunBuildModel(c, build, lbm.RerunBuildType_NthSection, nil, nsa)
		if err != nil {
			return errors.Annotate(err, "createRerunBuildModel for %s", commit).Err()
		}
	}

	return nil
}

func rerunCommit(c context.Context, nthSectionAnalysis *lbm.CompileNthSectionAnalysis, commit *buildbucketpb.GitilesCommit, failedBuildID int64) (*buildbucketpb.Build, error) {
	props, err := getRerunProps(c, nthSectionAnalysis)
	if err != nil {
		return nil, errors.Annotate(err, "failed getting rerun props").Err()
	}

	// TODO(nqmtuan): Support priority here
	build, err := rerun.TriggerRerun(c, commit, failedBuildID, props)
	if err != nil {
		return nil, errors.Annotate(err, "couldn't trigger rerun").Err()
	}
	return build, nil
}

func getRerunProps(c context.Context, nthSectionAnalysis *lbm.CompileNthSectionAnalysis) (map[string]interface{}, error) {
	analysisID := nthSectionAnalysis.ParentAnalysis.IntID()
	compileFailure, err := datastoreutil.GetCompileFailureForAnalysis(c, analysisID)
	if err != nil {
		return nil, errors.Annotate(err, "get compile failure for analysis %d", analysisID).Err()
	}
	// TODO (nqmtuan): Handle the case where the failed compile targets are newly added.
	// So any commits before the culprits cannot run the targets.
	// In such cases, we should detect from the recipe side
	failedTargets := compileFailure.OutputTargets

	props := map[string]interface{}{
		"analysis_id":    analysisID,
		"bisection_host": fmt.Sprintf("%s.appspot.com", info.AppID(c)),
	}
	if len(failedTargets) > 0 {
		props["compile_targets"] = failedTargets
	}

	return props, nil
}

func updateBlameList(c context.Context, nthSectionAnalysis *lbm.CompileNthSectionAnalysis, changeLogs []*lbm.ChangeLog) error {
	commits := []*lbpb.BlameListSingleCommit{}
	for _, cl := range changeLogs {
		reviewURL, err := cl.GetReviewUrl()
		if err != nil {
			// Just log, this is not important for nth-section analysis
			logging.Errorf(c, "Error getting review URL: %s", err)
		}

		reviewTitle, err := cl.GetReviewTitle()
		if err != nil {
			// Just log, this is not important for nth-section analysis
			logging.Errorf(c, "Error getting review title: %s", err)
		}

		commits = append(commits, &lbpb.BlameListSingleCommit{
			Commit:      cl.Commit,
			ReviewUrl:   reviewURL,
			ReviewTitle: reviewTitle,
		})
	}
	nthSectionAnalysis.BlameList = &lbpb.BlameList{
		Commits: commits,
	}
	return datastore.Put(c, nthSectionAnalysis)
}
