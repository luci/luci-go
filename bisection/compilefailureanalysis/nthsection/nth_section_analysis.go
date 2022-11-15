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

	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto"
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
	cfa *model.CompileFailureAnalysis) (*model.CompileNthSectionAnalysis, error) {
	// Create a new CompileNthSectionAnalysis Entity
	nthSectionAnalysis := &model.CompileNthSectionAnalysis{
		ParentAnalysis: datastore.KeyForObj(c, cfa),
		StartTime:      clock.Now(c),
		Status:         pb.AnalysisStatus_CREATED,
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
func startAnalysis(c context.Context, nsa *model.CompileNthSectionAnalysis, cfa *model.CompileFailureAnalysis) error {
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
		err := RerunCommit(c, nsa, gitilesCommit, cfa.FirstFailedBuildId, nil)
		if err != nil {
			return errors.Annotate(err, "rerunCommit for %s", commit).Err()
		}
	}

	return nil
}

func RerunCommit(c context.Context, nsa *model.CompileNthSectionAnalysis, commit *buildbucketpb.GitilesCommit, failedBuildID int64, dims map[string]string) error {
	props, err := getRerunProps(c, nsa)
	if err != nil {
		return errors.Annotate(err, "failed getting rerun props").Err()
	}

	priority := getRerunPriority(c, nsa, commit, dims)

	// TODO(nqmtuan): Support priority here
	build, err := rerun.TriggerRerun(c, commit, failedBuildID, props, dims, priority)
	if err != nil {
		return errors.Annotate(err, "couldn't trigger rerun").Err()
	}

	_, err = rerun.CreateRerunBuildModel(c, build, model.RerunBuildType_NthSection, nil, nsa, priority)
	if err != nil {
		return errors.Annotate(err, "createRerunBuildModel").Err()
	}

	return nil
}

func getRerunPriority(c context.Context, nsa *model.CompileNthSectionAnalysis, commit *buildbucketpb.GitilesCommit, dims map[string]string) int32 {
	// TODO (nqmtuan): Add other priority offset
	var pri int32 = rerun.PriorityNthSection
	// If targetting a particular bot
	if _, ok := dims["id"]; ok {
		pri += rerun.PriorityScheduleOnSameBotOffset
	}
	return rerun.CapPriority(pri)
}

func getRerunProps(c context.Context, nthSectionAnalysis *model.CompileNthSectionAnalysis) (map[string]interface{}, error) {
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

func updateBlameList(c context.Context, nthSectionAnalysis *model.CompileNthSectionAnalysis, changeLogs []*model.ChangeLog) error {
	commits := []*pb.BlameListSingleCommit{}
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

		commits = append(commits, &pb.BlameListSingleCommit{
			Commit:      cl.Commit,
			ReviewUrl:   reviewURL,
			ReviewTitle: reviewTitle,
		})
	}
	nthSectionAnalysis.BlameList = &pb.BlameList{
		Commits: commits,
	}
	return datastore.Put(c, nthSectionAnalysis)
}
