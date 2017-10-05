// Copyright 2017 The LUCI Authors.
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

package testutil

import (
	"golang.org/x/net/context"

	"go.chromium.org/luci/common/api/gitiles"
)

// HistoryFunc is a function that gets a list of commits from Gitiles for a
// specific repository at repoURL, between revisions oldRevision and newRevision.
type HistoryFunc func(c context.Context, repoURL, oldRevision, newRevision string) ([]gitiles.Commit, error)

// NewMockHistoryFunc generates a new mock HistoryFunc that gets its history from
// a given list of gitiles.Commit.
func NewMockHistoryFunc(mockCommits []gitiles.Commit) HistoryFunc {
	return func(_ context.Context, _, oldRevision, newRevision string) ([]gitiles.Commit, error) {
		oldCommit := -1
		newCommit := -1
		for i, c := range mockCommits {
			if c.Commit == oldRevision {
				oldCommit = i
			}
			if c.Commit == newRevision {
				newCommit = i
			}
		}
		if oldCommit == -1 || newCommit == -1 || newCommit < oldCommit {
			return []gitiles.Commit{}, nil
		}
		commits := make([]gitiles.Commit, newCommit-oldCommit+1)
		for i, j := newCommit, 0; i >= oldCommit; i, j = i-1, j+1 {
			commits[j] = mockCommits[i]
		}
		return commits, nil
	}
}
