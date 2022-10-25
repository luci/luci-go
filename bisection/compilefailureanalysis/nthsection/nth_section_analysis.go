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

	lbm "go.chromium.org/luci/bisection/model"
	lbpb "go.chromium.org/luci/bisection/proto"
	"go.chromium.org/luci/bisection/util/changelogutil"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
)

func Analyze(
	c context.Context,
	cfa *lbm.CompileFailureAnalysis,
	rr *lbpb.RegressionRange) (*lbm.CompileNthSectionAnalysis, error) {
	// Create a new CompileNthSectionAnalysis Entity
	nthSectionAnalysis := &lbm.CompileNthSectionAnalysis{
		ParentAnalysis: datastore.KeyForObj(c, cfa),
		StartTime:      clock.Now(c),
		Status:         lbpb.AnalysisStatus_CREATED,
	}

	changeLogs, err := changelogutil.GetChangeLogs(c, rr)
	if err != nil {
		logging.Infof(c, "Cannot fetch changelog for analysis %d", cfa.Id)
	}

	err = updateBlameList(c, nthSectionAnalysis, changeLogs)
	if err != nil {
		return nil, err
	}

	err = startAnalysis(c, nthSectionAnalysis)
	if err != nil {
		return nil, err
	}
	return nthSectionAnalysis, nil
}

// startAnalysis will based on find next commit(s) for rerun and schedule them
func startAnalysis(c context.Context, nthSectionAnalysis *lbm.CompileNthSectionAnalysis) error {
	_, err := CreateSnapshot(c, nthSectionAnalysis)
	if err != nil {
		return err
	}

	// TODO (nqmtuan): find the commits to run and schedule the run
	// maxRerun = 1 // bisection
	// snapshot.FindNextIndicesToRun(3)

	return nil
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
