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

package notify

import (
	"context"
	"testing"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"
	"go.chromium.org/luci/common/data/stringset"
	gitpb "go.chromium.org/luci/common/proto/git"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	notifypb "go.chromium.org/luci/luci_notify/api/config"
)

var (
	// These defaults match up with the "basic" project config.
	defaultGitilesHost    = "chromium.googlesource.com"
	defaultGitilesProject = "chromium/src"

	// Test revisions.
	rev1 = "deadbeef"
	rev2 = "badcoffe"

	// Test email addresses for commits.
	commitEmail1 = "example1@google.com"
	commitEmail2 = "example2@google.com"

	// Test commits, ordered from new to old.
	testCommits = []*gitpb.Commit{
		{
			Author: &gitpb.Commit_User{
				Email: commitEmail2,
			},
			Id: rev2,
		},
		{
			Author: &gitpb.Commit_User{
				Email: commitEmail1,
			},
			Id: rev1,
		},
	}
	revTestCommits = []*gitpb.Commit{
		{
			Author: &gitpb.Commit_User{
				Email: commitEmail1,
			},
			Id: rev2,
		},
		{
			Author: &gitpb.Commit_User{
				Email: commitEmail2,
			},
			Id: rev1,
		},
	}
)

func TestCheckout(t *testing.T) {
	gitilesCheckout := &notifypb.GitilesCommits{
		Commits: []*buildbucketpb.GitilesCommit{
			{
				Host:    defaultGitilesHost,
				Project: defaultGitilesProject,
				Id:      rev1,
			},
			{
				Host:    defaultGitilesHost,
				Project: "third_party/hello",
				Id:      rev2,
			},
		},
	}
	ftt.Run(`Conversion with GitilesCommits Empty`, t, func(t *ftt.Test) {
		checkout := NewCheckout(&notifypb.GitilesCommits{})
		assert.Loosely(t, checkout, should.HaveLength(0))
		result := checkout.ToGitilesCommits()
		assert.Loosely(t, result, should.BeNil)
	})

	ftt.Run(`Conversion with GitilesCommits Non-Empty`, t, func(t *ftt.Test) {
		checkout := NewCheckout(gitilesCheckout)
		assert.Loosely(t, checkout, should.Match(Checkout{
			protoutil.GitilesRepoURL(gitilesCheckout.Commits[0]): rev1,
			protoutil.GitilesRepoURL(gitilesCheckout.Commits[1]): rev2,
		}))
		result := checkout.ToGitilesCommits()
		assert.Loosely(t, result, should.Match(gitilesCheckout))
	})

	ftt.Run(`Filter repositories from allowlist`, t, func(t *ftt.Test) {
		checkout := NewCheckout(gitilesCheckout)
		repoURL := protoutil.GitilesRepoURL(gitilesCheckout.Commits[0])
		filteredCheckout := checkout.Filter(stringset.NewFromSlice([]string{repoURL}...))
		assert.Loosely(t, filteredCheckout, should.Match(Checkout{repoURL: rev1}))
	})
}

func TestLogs(t *testing.T) {
	ftt.Run(`ComputeLogs`, t, func(t *ftt.Test) {
		ctx := context.Background()
		history := mockHistoryFunc(map[string][]*gitpb.Commit{
			"chromium/src":      testCommits,
			"third_party/hello": testCommits,
			"third_party/what":  testCommits,
		})
		checkout1Old := Checkout{
			"https://chromium.googlesource.com/chromium/src":      rev1,
			"https://chromium.googlesource.com/third_party/hello": rev1,
		}
		checkout1New := Checkout{
			"https://chromium.googlesource.com/chromium/src":      rev2,
			"https://chromium.googlesource.com/third_party/hello": rev2,
		}
		checkout2 := Checkout{
			"https://chromium.googlesource.com/chromium/src":     rev2,
			"https://chromium.googlesource.com/third_party/what": rev2,
		}
		checkout3 := Checkout{
			"https://chromium.googlesource.com/third_party/what": rev2,
		}
		t.Run(`Both empty`, func(t *ftt.Test) {
			logs, err := ComputeLogs(ctx, "luci-proj", nil, nil, history)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, logs, should.HaveLength(0))
		})
		t.Run(`One empty`, func(t *ftt.Test) {
			logs1, err := ComputeLogs(ctx, "luci-proj", checkout1Old, nil, history)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, logs1, should.HaveLength(0))

			logs2, err := ComputeLogs(ctx, "luci-proj", nil, checkout1Old, history)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, logs2, should.HaveLength(0))
		})
		t.Run(`Both valid, full overlap`, func(t *ftt.Test) {
			logs, err := ComputeLogs(ctx, "luci-proj", checkout1Old, checkout1New, history)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, logs["https://chromium.googlesource.com/chromium/src"], should.Match(testCommits[:1]))
			assert.Loosely(t, logs["https://chromium.googlesource.com/third_party/hello"], should.Match(testCommits[:1]))
		})
		t.Run(`Both valid, partial overlap`, func(t *ftt.Test) {
			logs, err := ComputeLogs(ctx, "luci-proj", checkout1Old, checkout2, history)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, logs["https://chromium.googlesource.com/chromium/src"], should.Match(testCommits[:1]))
		})
		t.Run(`Both valid, no overlap`, func(t *ftt.Test) {
			logs, err := ComputeLogs(ctx, "luci-proj", checkout1Old, checkout3, history)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, logs, should.HaveLength(0))
		})
	})

	testLogs := Logs{
		"https://chromium.googlesource.com/chromium/src":      testCommits,
		"https://chromium.googlesource.com/third_party/hello": testCommits,
	}

	ftt.Run(`Filter repositories from allowlist`, t, func(t *ftt.Test) {
		filteredLogs := testLogs.Filter(stringset.NewFromSlice([]string{
			"https://chromium.googlesource.com/chromium/src",
		}...))
		assert.Loosely(t, filteredLogs["https://chromium.googlesource.com/chromium/src"], should.Match(testCommits))
	})

	ftt.Run(`Blamelist`, t, func(t *ftt.Test) {
		blamelist := testLogs.Blamelist("default")
		assert.Loosely(t, blamelist, should.Match([]EmailNotify{
			{
				Email:    "example1@google.com",
				Template: "default",
			},
			{
				Email:    "example2@google.com",
				Template: "default",
			},
		}))
	})
}

// mockHistoryFunc returns a mock HistoryFunc that gets its history from
// a given list of gitpb.Commit.
//
// mockCommits should be ordered from newest to oldest, to mimic the ordering
// returned by Gitiles.
func mockHistoryFunc(projectCommits map[string][]*gitpb.Commit) HistoryFunc {
	return func(_ context.Context, _, _, project, oldRevision, newRevision string) ([]*gitpb.Commit, error) {
		mockCommits := projectCommits[project]
		oldCommit := -1
		newCommit := -1
		for i, c := range mockCommits {
			if c.Id == oldRevision {
				oldCommit = i
			}
			if c.Id == newRevision {
				newCommit = i
			}
		}
		if oldCommit == -1 || newCommit == -1 || newCommit > oldCommit {
			return nil, nil
		}
		commits := make([]*gitpb.Commit, oldCommit-newCommit+1)
		copy(commits, mockCommits[newCommit:oldCommit+1])
		return commits, nil
	}
}
