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
	"errors"
	"fmt"
	"regexp"
	"strings"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/bisection/internal/gitiles"
	"go.chromium.org/luci/bisection/model"
	pb "go.chromium.org/luci/bisection/proto/v1"
)

var rollURLRe = regexp.MustCompile(`https://([a-zA-Z0-9.\-]+)/([^/\s]+?(?:\/[^/\s]+?)*?)(?:\.git)?/\+log/([a-f0-9]{7,40})\.\.([a-f0-9]{7,40})`)
var rollTitleRe = regexp.MustCompile(`(?i)^Roll\s+([a-zA-Z0-9_\-/.]+)(?:\s+|:)`)
var repoURLRe = regexp.MustCompile(`https://([a-zA-Z0-9.\-]+)/(.+)`)

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

	commitTime, err := cl.GetCommitTime()
	if err != nil {
		// Just log, this is informational.
		logging.Errorf(ctx, "Error getting commit time: %s", err)
	}

	return &pb.BlameListSingleCommit{
		Commit:      cl.Commit,
		ReviewUrl:   reviewURL,
		ReviewTitle: reviewTitle,
		CommitTime:  commitTime,
	}
}

// SetCommitPositionInBlamelist sets the position field in BlameListSingleCommits of this blamelist.
// Commits in the blamelist are ordered by commit position in descending order.
// Index 0 refers to the highest-position commit in the regression range, with has the commit position same as regression end position.
// Index n-1 refers to the lowest-position commit in the regression range.
// We can find the commit position of all commits in between.
func SetCommitPositionInBlamelist(blamelist *pb.BlameList, regressionStartPosition, regressionEndPosition int64) error {
	if int(regressionEndPosition-regressionStartPosition) != len(blamelist.Commits) {
		msg := fmt.Sprintf("Number of changelog in the regression range (%d) doesn't match the regression commit position (%d)-(%d)",
			len(blamelist.Commits), regressionStartPosition, regressionEndPosition)
		return errors.New(msg)
	}
	for i, c := range blamelist.Commits {
		c.Position = regressionEndPosition - int64(i)
	}
	blamelist.LastPassCommit.Position = regressionStartPosition
	return nil
}

// FindCommitIndexInBlameList find the index of the gitiles commit in blamelist.
// It returns -1 if it couldn't find.
func FindCommitIndexInBlameList(gitilesCommit *bbpb.GitilesCommit, blamelist *pb.BlameList) int {
	for i, commit := range blamelist.Commits {
		if commit.Commit == gitilesCommit.Id {
			return i
		}
	}
	return -1
}

// ParseRollURL parses a Gitiles log URL from a commit message.
// It returns the host, project, start revision, end revision, and true if successful.
func ParseRollURL(msg string) (string, string, string, string, bool) {
	matches := rollURLRe.FindStringSubmatch(msg)
	if len(matches) < 5 {
		return "", "", "", "", false
	}
	return matches[1], matches[2], matches[3], matches[4], true
}

// ParseRollPrefix parses the prefix path (dependency location) from the roll title/message.
// It ensures the prefix ends with a slash.
func ParseRollPrefix(msg string) string {
	matches := rollTitleRe.FindStringSubmatch(msg)
	if len(matches) < 2 {
		return ""
	}
	prefix := matches[1]
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}
	return prefix
}

// ExpandChangeLogs identifies roll CLs in the blamelist and expands them by querying
// Gitiles for the sub-commits that were rolled, prepend their changed file paths
// with the rolled prefix, and inserting them into the blamelist.
// ExpandedChangeLog wraps model.ChangeLog with additional metadata
// populated during roll expansion (e.g. tracking sub-repo URLs and parent roll links).
type ExpandedChangeLog struct {
	*model.ChangeLog
	RepoURL    string
	RollParent string
}

const maxExpansionDepth = 5

func ExpandChangeLogs(c context.Context, changelogs []*model.ChangeLog, mainRepoURL string) ([]*ExpandedChangeLog, error) {
	var expanded []*ExpandedChangeLog
	for _, cl := range changelogs {
		ecl := &ExpandedChangeLog{
			ChangeLog: cl,
			RepoURL:   mainRepoURL,
		}
		expanded = append(expanded, ecl)
		if IsRevert(cl.Message) {
			continue
		}
		nested, err := expandChangeLogRecursive(c, ecl, "", 0)
		if err != nil {
			return nil, err
		}
		expanded = append(expanded, nested...)
	}
	return expanded, nil
}

func expandChangeLogRecursive(c context.Context, cl *ExpandedChangeLog, currentPrefix string, depth int) ([]*ExpandedChangeLog, error) {
	if depth >= maxExpansionDepth {
		logging.Warningf(c, "Exceeded max roll expansion depth %d for commit %s, stopping expansion", maxExpansionDepth, cl.Commit)
		return nil, nil
	}

	host, project, start, end, ok := ParseRollURL(cl.Message)
	if !ok {
		return nil, nil
	}

	prefix := ParseRollPrefix(cl.Message)
	combinedPrefix := currentPrefix + prefix
	repoURL := fmt.Sprintf("https://%s/%s", host, project)
	logging.Infof(c, "Found roll CL %s, expanding sub-commits in %s from %s to %s with combined prefix %s", cl.Commit, repoURL, start, end, combinedPrefix)

	subChangelogs, err := gitiles.GetChangeLogs(c, repoURL, start, end)
	if err != nil {
		logging.Errorf(c, "Failed to fetch sub-commits for roll %s: %v", cl.Commit, err)
		return nil, nil
	}

	var result []*ExpandedChangeLog
	for _, scl := range subChangelogs {
		escl := &ExpandedChangeLog{
			ChangeLog:  scl,
			RollParent: cl.Commit,
			RepoURL:    repoURL,
		}
		// Prepend the combined prefix path to the changed file paths in the sub-changelogs.
		// This translates the sub-repo relative file paths (e.g. "src/core/SkCanvas.cpp")
		// to main-repo relative file paths (e.g. "src/third_party/skia/src/core/SkCanvas.cpp")
		// so that they can be matched against build failure log file paths.
		if combinedPrefix != "" {
			for i := range escl.ChangeLogDiffs {
				diff := &escl.ChangeLogDiffs[i]
				if diff.OldPath != "" && diff.OldPath != "/dev/null" {
					diff.OldPath = combinedPrefix + diff.OldPath
				}
				if diff.NewPath != "" && diff.NewPath != "/dev/null" {
					diff.NewPath = combinedPrefix + diff.NewPath
				}
			}
		}
		result = append(result, escl)

		nested, err := expandChangeLogRecursive(c, escl, combinedPrefix, depth+1)
		if err != nil {
			return nil, err
		}
		result = append(result, nested...)
	}
	return result, nil
}

// ParseRepoURL parses a Gitiles repository URL into host and project.
func ParseRepoURL(repoURL string) (string, string, error) {
	matches := repoURLRe.FindStringSubmatch(repoURL)
	if len(matches) < 3 {
		return "", "", fmt.Errorf("invalid repo URL: %s", repoURL)
	}
	return matches[1], matches[2], nil
}

// IsRevert returns true if the commit message looks like a revert.
func IsRevert(msg string) bool {
	return strings.HasPrefix(msg, "Revert ") || strings.HasPrefix(msg, "Revert:")
}
