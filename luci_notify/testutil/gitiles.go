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
	"fmt"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/api/gitiles"
)

// GitilesMockClient is a mock Gitiles client used for testing.
//
// It always uses the same set of commits and ignores repo URLs.
type GitilesMockClient struct {
	Commits []gitiles.Commit
}

// History implements the GitilesClient interface for GitilesMockClient.
//
// It simply returns a range of commits from Commits, assuming that the
// commits and ordering is valid.
func (g *GitilesMockClient) History(_ context.Context, oldRevision, newRevision string) ([]gitiles.Commit, error) {
	oldCommit := -1
	newCommit := -1
	for i, c := range g.Commits {
		if c.Commit == oldRevision {
			oldCommit = i
		}
		if c.Commit == newRevision {
			newCommit = i
		}
	}
	fmt.Println(oldCommit, newCommit)
	if oldCommit == -1 || newCommit == -1 || newCommit < oldCommit {
		return []gitiles.Commit{}, nil
	}
	commits := make([]gitiles.Commit, newCommit - oldCommit + 1)
	for i, j := newCommit, 0; i >= oldCommit; i, j = i-1, j+1 {
		commits[j] = g.Commits[i]
	}
	return commits, nil
}
