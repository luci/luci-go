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
	"testing"

	"golang.org/x/net/context"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	gitpb "go.chromium.org/luci/common/proto/git"

	notifypb "go.chromium.org/luci/luci_notify/api/config"

	. "github.com/smartystreets/goconvey/convey"
)

var (
	// These defaults match up with the "basic" project config.
	defaultGitilesHost = "chromium.googlesource.com"
	defaultGitilesProject = "chromium/src"

	// Test revisions.
	rev1 = "deadbeef"
	rev2 = "badcoffe"

	// Test commits, ordered from new to old.
	testCommits = []*gitpb.Commit{
		{
			Author: &gitpb.Commit_User{
				Email: "example2@google.com",
			},
			Id: rev2,
		},
		{
			Author: &gitpb.Commit_User{
				Email: "example1@google.com",
			},
			Id: rev1,
		},
	}
)

func TestCheckout(t *testing.T) {
	gitilesCheckout := notifypb.GitilesCommits{
		Commits: []*buildbucketpb.GitilesCommit{
			{
				Host: defaultGitilesHost,
				Project: defaultGitilesProject,
				Id: rev1,
			},
			{
				Host: defaultGitilesHost,
				Project: "third_party/hello",
				Id: rev2,
			},
		},
	}
	Convey(`Conversion with GitilesCommits Empty`, t, func() {
		checkout := NewCheckout(notifypb.GitilesCommits{})
		So(len(checkout), ShouldEqual, 0)
		So(len(checkout.ToGitilesCommits().Commits), ShouldEqual, 0)
	})

	Convey(`Conversion with GitilesCommits Non-Empty`, t, func() {
		checkout := NewCheckout(gitilesCheckout)
		So(checkout, ShouldResemble, Checkout{
			gitilesCheckout.Commits[0].RepoURL(): rev1,
			gitilesCheckout.Commits[1].RepoURL(): rev2,
		})
		So(checkout.ToGitilesCommits(), ShouldResemble, gitilesCheckout)
	})

	Convey(`Filter repositories from whitelist`, t, func() {
		checkout := NewCheckout(gitilesCheckout)
		repoURL := gitilesCheckout.Commits[0].RepoURL()
		filteredCheckout := checkout.Filter([]string{repoURL})
		So(filteredCheckout, ShouldResemble, Checkout{repoURL: rev1})
	})
}

func TestLogs(t *testing.T) {
	Convey(`ComputeLogs`, t, func() {
		ctx := context.Background()
		history := mockHistoryFunc(testCommits)
		checkout1Old := Checkout{
			"https://chromium.googlesource.com/chromium/src": rev1,
			"https://chromium.googlesource.com/third_party/hello": rev1,
		}
		checkout1New := Checkout{
			"https://chromium.googlesource.com/chromium/src": rev2,
			"https://chromium.googlesource.com/third_party/hello": rev2,
		}
		checkout2 := Checkout{
			"https://chromium.googlesource.com/chromium/src": rev2,
			"https://chromium.googlesource.com/third_party/what": rev2,
		}
		checkout3 := Checkout{
			"https://chromium.googlesource.com/third_party/what": rev2,
		}
		Convey(`Both empty`, func() {
			logs, err := ComputeLogs(ctx, nil, nil, history)
			So(err, ShouldBeNil)
			So(len(logs), ShouldEqual, 0)
		})
		Convey(`One empty`, func() {
			logs1, err := ComputeLogs(ctx, checkout1Old, nil, history)
			So(err, ShouldBeNil)
			So(len(logs1), ShouldEqual, 0)

			logs2, err := ComputeLogs(ctx, nil, checkout1Old, history)
			So(err, ShouldBeNil)
			So(len(logs2), ShouldEqual, 0)
		})
		Convey(`Both valid, full overlap`, func() {
			logs, err := ComputeLogs(ctx, checkout1Old, checkout1New, history)
			So(err, ShouldBeNil)
			So(logs, ShouldResemble, Logs{
				"https://chromium.googlesource.com/chromium/src": testCommits,
				"https://chromium.googlesource.com/third_party/hello": testCommits,
			})
		})
		Convey(`Both valid, partial overlap`, func() {
			logs, err := ComputeLogs(ctx, checkout1Old, checkout2, history)
			So(err, ShouldBeNil)
			So(logs, ShouldResemble, Logs{
				"https://chromium.googlesource.com/chromium/src": testCommits,
			})
		})
		Convey(`Both valid, no overlap`, func() {
			logs, err := ComputeLogs(ctx, checkout1Old, checkout3, history)
			So(err, ShouldBeNil)
			So(len(logs), ShouldEqual, 0)
		})
	})

	Convey(`Blamelist`, t, func() {
		logs := Logs{
			"https://chromium.googlesource.com/chromium/src": testCommits,
			"https://chromium.googlesource.com/third_party/hello": testCommits,
		}
		blamelist := logs.Blamelist("default")
		So(blamelist, ShouldResemble, []EmailNotify{
			{
				Email: "example1@google.com",
				Template: "default",
			},
			{
				Email: "example2@google.com",
				Template: "default",
			},
		})
	})
}

// mockHistoryFunc returns a mock HistoryFunc that gets its history from
// a given list of gitpb.Commit.
//
// mockCommits should be ordered from newest to oldest, to mimic the ordering
// returned by Gitiles.
func mockHistoryFunc(mockCommits []*gitpb.Commit) HistoryFunc {
	return func(_ context.Context, _, _, oldRevision, newRevision string) ([]*gitpb.Commit, error) {
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
