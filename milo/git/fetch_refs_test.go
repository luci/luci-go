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

	"github.com/golang/mock/gomock"
	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/auth/identity"
	gitpb "go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
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
		for i := range fakeCommits {
			fakeCommits[i] = &gitpb.Commit{Id: hex.EncodeToString(commitID)}
			if i%10 > 0 { // every 10th commit has no parent (another branch)
				fakeCommits[i-1].Parents = []string{fakeCommits[i].Id}
			}

			commitID[0]--
		}

		Convey("ACLs respected", func() {
			_, err := impl.FetchRefs(
				cDenied, host, "project", "refs/heads/master",
				[]string{`refs/branch-heads/\d+\.\d+`}, 50)
			So(err.Error(), ShouldContainSubstring, "not found")
		})

		Convey("one ref matches", func() {
			gitilesMock.EXPECT().Refs(gomock.Any(), &gitilespb.RefsRequest{
				Project: "project", RefsPath: "refs/branch-heads/",
			}).Return(&gitilespb.RefsResponse{
				Revisions: map[string]string{"refs/branch-heads/1.1": fakeCommits[0].Id},
			}, nil)

			gitilesMock.EXPECT().Log(gomock.Any(), &gitilespb.LogRequest{
				Project: "project", Treeish: "refs/branch-heads/1.1",
				PageSize: 100, Ancestor: "refs/heads/master", TreeDiff: false,
			}).Return(&gitilespb.LogResponse{
				Log: fakeCommits[0:5],
			}, nil)

			commits, err := impl.FetchRefs(
				cAllowed, host, "project", "refs/heads/master",
				[]string{`refs/branch-heads/\d+\.\d+`}, 50)
			So(err, ShouldBeNil)
			So(commits, ShouldResemble, fakeCommits[0:5])
		})
	})
}
