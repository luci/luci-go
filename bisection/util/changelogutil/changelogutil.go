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

// Package changelogutil contains utility functions for changelogs.
package changelogutil

import (
	"context"
	"fmt"

	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
	"go.chromium.org/luci/common/logging"
)

// GetChangeLogs queries Gitiles for changelogs in the regression range.
// If shouldIncludeLastPass is true, the result should also include the last pass revision.
// The result will be in descending order of recency (i.e. first failed revision at index 0).
func GetChangeLogs(c context.Context, rr *pb.RegressionRange, shouldIncludeLastPass bool) ([]*model.ChangeLog, error) {
	if rr.LastPassed.Host != rr.FirstFailed.Host || rr.LastPassed.Project != rr.FirstFailed.Project {
		return nil, fmt.Errorf("RepoURL for last pass and first failed commits must be same, but aren't: %v and %v", rr.LastPassed, rr.FirstFailed)
	}
	repoURL := gitiles.GetRepoUrl(c, rr.FirstFailed)
	lastPassID := rr.LastPassed.Id
	if shouldIncludeLastPass {
		lastPassID = fmt.Sprintf("%s^1", lastPassID)
	}
	return gitiles.GetChangeLogs(c, repoURL, lastPassID, rr.FirstFailed.Id)
}

func ChangeLogsToBlamelist(ctx context.Context, changeLogs []*model.ChangeLog) *pb.BlameList {
	if len(changeLogs) == 0 {
		return &pb.BlameList{}
	}
	commits := []*pb.BlameListSingleCommit{}
	for i := 0; i < len(changeLogs)-1; i++ {
		cl := changeLogs[i]
		commits = append(commits, changelogToCommit(ctx, cl))
	}
	return &pb.BlameList{
		Commits:        commits,
		LastPassCommit: changelogToCommit(ctx, changeLogs[len(changeLogs)-1]),
	}
}

func changelogToCommit(ctx context.Context, cl *model.ChangeLog) *pb.BlameListSingleCommit {
	reviewURL, err := cl.GetReviewUrl()
	if err != nil {
		// Just log, this is not important for nth-section analysis
		logging.Errorf(ctx, "Error getting review URL: %s", err)
	}

	reviewTitle, err := cl.GetReviewTitle()
	if err != nil {
		// Just log, this is not important for nth-section analysis
		logging.Errorf(ctx, "Error getting review title: %s", err)
	}

	return &pb.BlameListSingleCommit{
		Commit:      cl.Commit,
		ReviewUrl:   reviewURL,
		ReviewTitle: reviewTitle,
	}
}
