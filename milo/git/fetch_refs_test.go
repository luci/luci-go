// Copyright 2018 The LUCI Authors.
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

package git

import (
	"encoding/hex"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/auth/identity"
	gitpb "go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/milo/api/config"
	"go.chromium.org/luci/milo/git/gitacls"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFetchRefs(t *testing.T) {
	t.Parallel()

	Convey("FetchRefs", t, func() {
		c := memory.Use(context.Background())

		ctl := gomock.NewController(t)
		defer ctl.Finish()
		gitilesMock := gitilespb.NewMockGitilesClient(ctl)

		host := "limited.googlesource.com"
		acls, err := gitacls.FromConfig(c, []*config.Settings_SourceAcls{
			{Hosts: []string{host}, Readers: []string{"allowed@example.com"}},
		})
		So(err, ShouldBeNil)
		impl := implementation{mockGitiles: gitilesMock, acls: acls}
		c = Use(c, &impl)
		cAllowed := auth.WithState(c, &authtest.FakeState{Identity: "user:allowed@example.com"})
		cDenied := auth.WithState(c, &authtest.FakeState{Identity: identity.AnonymousIdentity})

		fakeCommits := make([]*gitpb.Commit, 255)
		commitID := make([]byte, 20)
		commitID[0] = 255
		referenceTime, err := time.Parse(time.RFC3339, "2018-06-22T19:34:06Z")
		So(err, ShouldBeNil)
		for i := range fakeCommits {
			fakeCommits[i] = &gitpb.Commit{
				Id: hex.EncodeToString(commitID),
				Committer: &gitpb.Commit_User{
					Time: google.NewTimestamp( // each next commit is 1 minute older
						referenceTime.Add(-time.Duration(i) * time.Minute)),
				},
			}
			if i%10 > 0 { // every 10th commit has no parent (another branch)
				fakeCommits[i-1].Parents = []string{fakeCommits[i].Id}
			}

			commitID[0]--
		}

		Convey("ACLs respected", func() {
			_, err := impl.FetchRefs(
				cDenied, host, "project", "refs/heads/master",
				[]string{`regexp:refs/branch-heads/\d+\.\d+`}, 50)
			So(err.Error(), ShouldContainSubstring, "not found")
		})

		Convey("no refs match", func() {
			gitilesMock.EXPECT().Refs(gomock.Any(), &gitilespb.RefsRequest{
				Project: "project", RefsPath: "refs/branch-heads",
			}).Return(&gitilespb.RefsResponse{Revisions: map[string]string{}}, nil)

			commits, err := impl.FetchRefs(
				cAllowed, host, "project", "refs/heads/master",
				[]string{`regexp:refs/branch-heads/\d+\.\d+`}, 50)
			So(err, ShouldBeNil)
			So(len(commits), ShouldEqual, 0)
		})

		Convey("one ref matches", func() {
			gitilesMock.EXPECT().Refs(gomock.Any(), &gitilespb.RefsRequest{
				Project: "project", RefsPath: "refs/branch-heads",
			}).Return(&gitilespb.RefsResponse{
				Revisions: map[string]string{"refs/branch-heads/1.1": fakeCommits[0].Id},
			}, nil)

			gitilesMock.EXPECT().Log(gomock.Any(), &gitilespb.LogRequest{
				Project: "project", Treeish: fakeCommits[0].Id,
				PageSize: 100, Ancestor: "refs/heads/master",
			}).Return(&gitilespb.LogResponse{
				Log: fakeCommits[0:5],
			}, nil)

			commits, err := impl.FetchRefs(
				cAllowed, host, "project", "refs/heads/master",
				[]string{`regexp:refs/branch-heads/\d+\.\d+`}, 50)
			So(err, ShouldBeNil)
			So(commits, ShouldResemble, fakeCommits[0:5])
		})

		Convey("multiple refs match and commits are merged correctly", func() {
			gitilesMock.EXPECT().Refs(gomock.Any(), &gitilespb.RefsRequest{
				Project: "project", RefsPath: "refs/branch-heads",
			}).Return(&gitilespb.RefsResponse{
				Revisions: map[string]string{
					"refs/branch-heads/1.1": fakeCommits[0].Id,
					"refs/branch-heads/1.2": fakeCommits[10].Id,
				},
			}, nil)

			gitilesMock.EXPECT().Refs(gomock.Any(), &gitilespb.RefsRequest{
				Project: "project", RefsPath: "refs/heads",
			}).Return(&gitilespb.RefsResponse{
				Revisions: map[string]string{
					"refs/heads/1.3.195": fakeCommits[20].Id,
				},
			}, nil)

			// Change commit times in order to test merging logic. This still keeps
			// the order of commits on each ref, but should change the order in the
			// merged list by moving:
			//  - commit 2 back in time between 22 and 23,
			//  - commit 3 back in time past 23 (should be truncated by limit) and
			//  - commit 20 forward in time between 0 and 1.
			fakeCommits[2].Committer.Time = google.NewTimestamp(
				referenceTime.Add(-time.Duration(22)*time.Minute - time.Second))
			fakeCommits[3].Committer.Time = google.NewTimestamp(
				referenceTime.Add(-time.Duration(23)*time.Minute - time.Second))
			fakeCommits[20].Committer.Time = google.NewTimestamp(
				referenceTime.Add(-time.Duration(0)*time.Minute - time.Second))

			gitilesMock.EXPECT().Log(gomock.Any(), &gitilespb.LogRequest{
				Project: "project", Treeish: fakeCommits[0].Id,
				PageSize: 100, Ancestor: "refs/heads/master",
			}).Return(&gitilespb.LogResponse{
				Log: fakeCommits[0:4],
			}, nil)

			gitilesMock.EXPECT().Log(gomock.Any(), &gitilespb.LogRequest{
				Project: "project", Treeish: fakeCommits[10].Id,
				PageSize: 100, Ancestor: "refs/heads/master",
			}).Return(&gitilespb.LogResponse{
				Log: []*gitpb.Commit{},
			}, nil)

			gitilesMock.EXPECT().Log(gomock.Any(), &gitilespb.LogRequest{
				Project: "project", Treeish: fakeCommits[20].Id,
				PageSize: 100, Ancestor: "refs/heads/master",
			}).Return(&gitilespb.LogResponse{
				Log: fakeCommits[20:30],
			}, nil)

			commits, err := impl.FetchRefs(
				cAllowed, host, "project", "refs/heads/master", []string{
					`regexp:refs/branch-heads/\d+\.\d+`,
					`regexp:refs/heads/\d+\.\d+\.\d+`,
				}, 7)
			So(err, ShouldBeNil)
			So(commits, ShouldResemble, []*gitpb.Commit{
				fakeCommits[0], fakeCommits[20], fakeCommits[1], fakeCommits[21],
				fakeCommits[22], fakeCommits[2], fakeCommits[23],
			})
		})
	})
}
