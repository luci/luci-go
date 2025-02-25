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
	"context"
	"encoding/hex"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/proto"
	gitpb "go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/proto/gitiles/mock_gitiles"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/milo/internal/git/gitacls"
	configpb "go.chromium.org/luci/milo/proto/config"
)

func TestCombinedLogs(t *testing.T) {
	t.Parallel()

	ftt.Run("CombinedLogs", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())

		ctl := gomock.NewController(t)
		defer ctl.Finish()
		gitilesMock := mock_gitiles.NewMockGitilesClient(ctl)

		host := "limited.googlesource.com"
		acls, err := gitacls.FromConfig(c, []*configpb.Settings_SourceAcls{
			{Hosts: []string{host}, Readers: []string{"allowed@example.com"}},
		})
		assert.Loosely(t, err, should.BeNil)
		impl := implementation{mockGitiles: gitilesMock, acls: acls}
		c = Use(c, &impl)
		cAllowed := auth.WithState(c, &authtest.FakeState{Identity: "user:allowed@example.com"})
		cDenied := auth.WithState(c, &authtest.FakeState{Identity: identity.AnonymousIdentity})

		fakeCommits := make([]*gitpb.Commit, 30)
		commitID := make([]byte, 20)
		commitID[0] = 255
		epoch, err := time.Parse(time.RFC3339, "2018-06-22T19:34:06Z")
		assert.Loosely(t, err, should.BeNil)
		for i := range fakeCommits {
			fakeCommits[i] = &gitpb.Commit{
				Id: hex.EncodeToString(commitID),
				Committer: &gitpb.Commit_User{
					Time: timestamppb.New( // each next commit is 1 minute older
						epoch.Add(-time.Duration(i) * time.Minute)),
				},
			}
			commitID[0]--
		}

		type refTips map[string]string
		mockRefsCall := func(prefix string, tips refTips) *gomock.Call {
			return gitilesMock.EXPECT().Refs(gomock.Any(), proto.MatcherEqual(&gitilespb.RefsRequest{
				Project:  "project",
				RefsPath: prefix,
			})).Return(&gitilespb.RefsResponse{Revisions: tips}, nil)
		}

		mockLogCall := func(reqCommit string, respCommits []*gitpb.Commit) *gomock.Call {
			return gitilesMock.EXPECT().Log(gomock.Any(), proto.MatcherEqual(&gitilespb.LogRequest{
				Project: "project", Committish: reqCommit,
				PageSize: 100, ExcludeAncestorsOf: "refs/heads/main",
			})).Return(&gitilespb.LogResponse{Log: respCommits}, nil)
		}

		t.Run("ACLs respected", func(t *ftt.Test) {
			_, err := impl.CombinedLogs(
				cDenied, host, "project", "refs/heads/main",
				[]string{`regexp:refs/branch-heads/\d+\.\d+`}, 50)
			assert.Loosely(t, err.Error(), should.ContainSubstring("not logged in"))
		})

		t.Run("no refs match", func(t *ftt.Test) {
			mockRefsCall("refs/branch-heads", refTips{})
			commits, err := impl.CombinedLogs(
				cAllowed, host, "project", "refs/heads/main",
				[]string{`regexp:refs/branch-heads/\d+\.\d+`}, 50)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, len(commits), should.BeZero)
		})

		t.Run("one ref matches", func(t *ftt.Test) {
			mockRefsCall("refs/branch-heads", refTips{
				"refs/branch-heads/1.1": fakeCommits[0].Id,
			})

			mockLogCall(fakeCommits[0].Id, fakeCommits[0:5])

			commits, err := impl.CombinedLogs(
				cAllowed, host, "project", "refs/heads/main",
				[]string{`regexp:refs/branch-heads/\d+\.\d+`}, 50)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, commits, should.Match(fakeCommits[0:5]))
		})

		t.Run("multiple refs match and commits are merged correctly", func(t *ftt.Test) {
			mockRefsCall("refs/branch-heads", refTips{
				"refs/branch-heads/1.1": fakeCommits[0].Id,
				"refs/branch-heads/1.2": fakeCommits[10].Id,
			})
			mockRefsCall("refs/heads", refTips{
				"refs/heads/1.3.195": fakeCommits[20].Id,
			})

			// Change commit times in order to test merging logic. This still keeps
			// the order of commits on each ref, but should change the order in the
			// merged list by moving:
			//  - commit 2 back in time between 22 and 23,
			//  - commit 3 back in time past 23 (should be truncated by limit) and
			//  - commit 20 forward in time between 0 and 1.
			fakeCommits[2].Committer.Time = timestamppb.New(
				epoch.Add(-time.Duration(22)*time.Minute - time.Second))
			fakeCommits[3].Committer.Time = timestamppb.New(
				epoch.Add(-time.Duration(23)*time.Minute - time.Second))
			fakeCommits[20].Committer.Time = timestamppb.New(
				epoch.Add(-time.Duration(0)*time.Minute - time.Second))

			mockLogCall(fakeCommits[0].Id, fakeCommits[0:4])
			mockLogCall(fakeCommits[10].Id, fakeCommits[10:10]) // empty list
			mockLogCall(fakeCommits[20].Id, fakeCommits[20:30])

			commits, err := impl.CombinedLogs(
				cAllowed, host, "project", "refs/heads/main", []string{
					`regexp:refs/branch-heads/\d+\.\d+`,
					`regexp:refs/heads/\d+\.\d+\.\d+`,
				}, 7)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, commits, should.Match([]*gitpb.Commit{
				fakeCommits[0], fakeCommits[20], fakeCommits[1], fakeCommits[21],
				fakeCommits[22], fakeCommits[2], fakeCommits[23],
			}))
		})

		t.Run("multiple refs match and their commits deduped", func(t *ftt.Test) {
			mockRefsCall("refs/branch-heads", refTips{
				"refs/branch-heads/1.1": fakeCommits[0].Id,
				"refs/branch-heads/1.2": fakeCommits[5].Id,
			})

			mockLogCall(fakeCommits[0].Id, fakeCommits[0:10])
			mockLogCall(fakeCommits[5].Id, fakeCommits[5:10])

			commits, err := impl.CombinedLogs(
				cAllowed, host, "project", "refs/heads/main",
				[]string{`regexp:refs/branch-heads/\d+\.\d+`}, 50)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, commits, should.Match(fakeCommits[0:10]))
		})

		t.Run("use result from cache when available", func(t *ftt.Test) {
			mockRefsCall("refs/branch-heads", refTips{
				"refs/branch-heads/1.1": fakeCommits[0].Id,
				"refs/branch-heads/1.2": fakeCommits[10].Id,
			}).Times(2)

			mockLogCall(fakeCommits[0].Id, fakeCommits[0:10]).Times(1)
			mockLogCall(fakeCommits[10].Id, fakeCommits[10:20]).Times(1)

			commits, err := impl.CombinedLogs(
				cAllowed, host, "project", "refs/heads/main",
				[]string{`regexp:refs/branch-heads/\d+\.\d+`}, 50)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, commits, should.Match(fakeCommits[0:20]))

			// This call should use logs from cache.
			commits, err = impl.CombinedLogs(
				cAllowed, host, "project", "refs/heads/main",
				[]string{`regexp:refs/branch-heads/\d+\.\d+`}, 50)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, commits, should.Match(fakeCommits[0:20]))
		})

		t.Run("invalidate cache when ref moves", func(t *ftt.Test) {
			firstRefsCall := mockRefsCall("refs/branch-heads", refTips{
				"refs/branch-heads/1.1": fakeCommits[0].Id,
				"refs/branch-heads/1.2": fakeCommits[11].Id,
			})

			mockRefsCall("refs/branch-heads", refTips{
				"refs/branch-heads/1.1": fakeCommits[0].Id,
				"refs/branch-heads/1.2": fakeCommits[10].Id,
			}).After(firstRefsCall)

			mockLogCall(fakeCommits[0].Id, fakeCommits[0:2])
			mockLogCall(fakeCommits[11].Id, fakeCommits[11:13])

			// This call is required due to moved ref.
			mockLogCall(fakeCommits[10].Id, fakeCommits[10:13])

			commits, err := impl.CombinedLogs(
				cAllowed, host, "project", "refs/heads/main",
				[]string{`regexp:refs/branch-heads/\d+\.\d+`}, 50)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, commits, should.Match([]*gitpb.Commit{
				fakeCommits[0], fakeCommits[1], fakeCommits[11], fakeCommits[12]}))

			commits, err = impl.CombinedLogs(
				cAllowed, host, "project", "refs/heads/main",
				[]string{`regexp:refs/branch-heads/\d+\.\d+`}, 50)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, commits, should.Match([]*gitpb.Commit{
				fakeCommits[0], fakeCommits[1], fakeCommits[10], fakeCommits[11],
				fakeCommits[12]}))
		})
	})
}
