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

	"github.com/alicebob/miniredis/v2"
	"github.com/golang/mock/gomock"
	"github.com/gomodule/redigo/redis"
	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/proto"
	gitpb "go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/proto/gitiles/mock_gitiles"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/milo/internal/git/gitacls"
	configpb "go.chromium.org/luci/milo/proto/config"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/redisconn"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestLog(t *testing.T) {
	t.Parallel()

	Convey("Log", t, func() {
		c := memory.Use(context.Background())

		// Set up a test redis server.
		s, err := miniredis.Run()
		So(err, ShouldBeNil)
		defer s.Close()
		c = redisconn.UsePool(c, &redis.Pool{
			Dial: func() (redis.Conn, error) {
				return redis.Dial("tcp", s.Addr())
			},
		})

		ctl := gomock.NewController(t)
		defer ctl.Finish()
		gitilesMock := mock_gitiles.NewMockGitilesClient(ctl)

		host := "limited.googlesource.com"
		acls, err := gitacls.FromConfig(c, []*configpb.Settings_SourceAcls{
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
			if i > 0 {
				fakeCommits[i-1].Parents = []string{fakeCommits[i].Id}
			}

			commitID[0]--
		}

		Convey("cold cache", func() {
			Convey("ACLs respected", func() {
				_, err := impl.Log(cDenied, host, "project", "refs/heads/main", &LogOptions{Limit: 50})
				So(err.Error(), ShouldContainSubstring, "not logged in")
			})

			req := &gitilespb.LogRequest{
				Project:    "project",
				Committish: "refs/heads/main",
				PageSize:   100,
			}
			res := &gitilespb.LogResponse{
				Log: fakeCommits[1:101], // return 100 commits
			}
			gitilesMock.EXPECT().Log(gomock.Any(), proto.MatcherEqual(req)).Return(res, nil)

			commits, err := impl.Log(cAllowed, host, "project", "refs/heads/main", &LogOptions{Limit: 100})
			So(err, ShouldBeNil)
			So(commits, ShouldResemble, res.Log)

			// Now that we have something in cache, call Log with cached commits.
			// gitiles.Log was already called maximum number of times, which is 1,
			// so another call with cause a test failure.

			Convey("ACLs respected even with cache", func() {
				_, err := impl.Log(cDenied, host, "project", "refs/heads/main", &LogOptions{Limit: 50})
				So(err.Error(), ShouldContainSubstring, "not logged in")
			})

			Convey("with exactly one last commit not in cache", func() {
				req2 := &gitilespb.LogRequest{
					Project:    "project",
					Committish: fakeCommits[100].Id,
					PageSize:   100, // we always fetch 100
				}
				res2 := &gitilespb.LogResponse{
					Log: fakeCommits[100:200],
				}
				gitilesMock.EXPECT().Log(gomock.Any(), proto.MatcherEqual(req2)).Return(res2, nil)
				commits, err := impl.Log(cAllowed, host, "project", "refs/heads/main", &LogOptions{Limit: 101})
				So(err, ShouldBeNil)
				So(commits, ShouldResembleProto, fakeCommits[1:102])
			})

			Convey("with exactly the proceeding commit not in cache", func() {
				req2 := &gitilespb.LogRequest{
					Project:    "project",
					Committish: fakeCommits[51].Id,
					PageSize:   100, // we always fetch 100
				}
				res2 := &gitilespb.LogResponse{
					Log: fakeCommits[51:150],
				}
				gitilesMock.EXPECT().Log(gomock.Any(), proto.MatcherEqual(req2)).Return(res2, nil).Times(0)
				commits, err := impl.Log(cAllowed, host, "project", fakeCommits[51].Id, &LogOptions{Limit: 50})
				So(err, ShouldBeNil)
				So(commits, ShouldResembleProto, fakeCommits[51:101])
			})

			Convey("with ref in cache", func() {
				commits, err := impl.Log(cAllowed, host, "project", "refs/heads/main", &LogOptions{Limit: 50})
				So(err, ShouldBeNil)
				So(commits, ShouldResembleProto, res.Log[:50])
			})

			Convey("with top commit in cache", func() {
				commits, err := impl.Log(cAllowed, host, "project", fakeCommits[1].Id, &LogOptions{Limit: 50})
				So(err, ShouldBeNil)
				So(commits, ShouldResembleProto, res.Log[:50])
			})

			Convey("with ancestor commit in cache", func() {
				commits, err := impl.Log(cAllowed, host, "project", fakeCommits[2].Id, &LogOptions{Limit: 50})
				So(err, ShouldBeNil)
				So(commits, ShouldResembleProto, res.Log[1:51])
			})

			Convey("with second ancestor commit in cache", func() {
				commits, err := impl.Log(cAllowed, host, "project", fakeCommits[3].Id, &LogOptions{Limit: 50})
				So(err, ShouldBeNil)
				So(commits, ShouldResembleProto, res.Log[2:52])
			})

			Convey("min is honored", func() {
				req2 := &gitilespb.LogRequest{
					Project:    "project",
					Committish: fakeCommits[2].Id,
					PageSize:   100,
				}
				res2 := &gitilespb.LogResponse{
					Log: fakeCommits[2:102],
				}
				gitilesMock.EXPECT().Log(gomock.Any(), proto.MatcherEqual(req2)).Return(res2, nil)

				commits, err := impl.Log(cAllowed, host, "project", fakeCommits[2].Id, &LogOptions{Limit: 100})
				So(err, ShouldBeNil)
				So(commits, ShouldHaveLength, 100)
				So(commits, ShouldResembleProto, res2.Log)
			})

			Convey("request of item not in cache", func() {
				req2 := &gitilespb.LogRequest{
					Project:    "project",
					Committish: fakeCommits[101].Id,
					PageSize:   100,
				}
				res2 := &gitilespb.LogResponse{
					Log: fakeCommits[101:201],
				}
				gitilesMock.EXPECT().Log(gomock.Any(), proto.MatcherEqual(req2)).Return(res2, nil)
				commits, err := impl.Log(cAllowed, host, "project", fakeCommits[101].Id, &LogOptions{Limit: 50})
				So(err, ShouldBeNil)
				So(commits, ShouldHaveLength, 50)
				So(commits, ShouldResemble, res2.Log[:50])
			})

			Convey("do not update cache entries that have more info", func() {
				refCacheKey := (&logReq{
					host:    host,
					project: "project",
				}).mkCacheKey(c, "refs/heads/main")

				conn, err := redisconn.Get(c)
				So(err, ShouldBeNil)
				defer conn.Close()
				_, err = conn.Do("DEL", refCacheKey)
				So(err, ShouldBeNil)

				req2 := &gitilespb.LogRequest{
					Project:    "project",
					Committish: "refs/heads/main",
					PageSize:   100,
				}
				res2 := &gitilespb.LogResponse{
					Log: fakeCommits[:100],
				}
				gitilesMock.EXPECT().Log(gomock.Any(), proto.MatcherEqual(req2)).Return(res2, nil)
				commits, err := impl.Log(cAllowed, host, "project", "refs/heads/main", &LogOptions{Limit: 50})
				So(err, ShouldBeNil)
				So(commits, ShouldResemble, res2.Log[:50])
			})
		})
		Convey("paging", func() {
			req1 := &gitilespb.LogRequest{
				Project:    "project",
				Committish: "refs/heads/main",
				PageSize:   100,
			}
			res1 := &gitilespb.LogResponse{
				Log: fakeCommits[:100],
			}
			req2 := &gitilespb.LogRequest{
				Project:    "project",
				Committish: res1.Log[len(res1.Log)-1].Id,
				PageSize:   100, // we always fetch 100
			}
			res2 := &gitilespb.LogResponse{
				Log: fakeCommits[99:199],
			}
			gitilesMock.EXPECT().Log(gomock.Any(), proto.MatcherEqual(req1)).Return(res1, nil)
			gitilesMock.EXPECT().Log(gomock.Any(), proto.MatcherEqual(req2)).Return(res2, nil)

			commits, err := impl.Log(cAllowed, host, "project", "refs/heads/main", &LogOptions{Limit: 150})
			So(err, ShouldBeNil)
			So(commits, ShouldResemble, fakeCommits[:150])
		})
	})
}
