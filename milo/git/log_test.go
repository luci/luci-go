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
	"go.chromium.org/gae/service/info"
	"go.chromium.org/gae/service/memcache"
	gitpb "go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"

	. "github.com/smartystreets/goconvey/convey"
)

func TestLog(t *testing.T) {
	t.Parallel()

	Convey("Log", t, func() {
		c := memory.Use(context.Background())

		ctl := gomock.NewController(t)
		defer ctl.Finish()
		gitilesMock := gitilespb.NewMockGitilesClient(ctl)
		c = UseFactory(c, func(c context.Context, host string) (gitilespb.GitilesClient, error) {
			return gitilesMock, nil
		})

		Convey("cold cache", func() {
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

			req := &gitilespb.LogRequest{
				Project:  "project",
				Treeish:  "refs/heads/master",
				PageSize: 100,
			}
			res := &gitilespb.LogResponse{
				Log: fakeCommits[1:101], // return 100 commits
			}
			gitilesMock.EXPECT().Log(gomock.Any(), req).Return(res, nil)
			commits, err := Log(c, "host", "project", "refs/heads/master", 0)
			So(err, ShouldBeNil)
			So(commits, ShouldResemble, res.Log)

			// Now that we have something in cache, call Log with cached commits.
			// gitiles.Log was already called maximum number of times, which is 1,
			// so another call with cause a test failure.

			Convey("with ref in cache", func() {
				commits, err := Log(c, "host", "project", "refs/heads/master", 0)
				So(err, ShouldBeNil)
				So(commits, ShouldResemble, res.Log)
			})

			Convey("with top commit in cache", func() {
				commits, err := Log(c, "host", "project", fakeCommits[1].Id, 0)
				So(err, ShouldBeNil)
				So(commits, ShouldResemble, res.Log)
			})

			Convey("with ancestor commit in cache", func() {
				commits, err := Log(c, "host", "project", fakeCommits[2].Id, 0)
				So(err, ShouldBeNil)
				So(commits, ShouldResemble, res.Log[1:])
			})

			Convey("with second ancestor commit in cache", func() {
				commits, err := Log(c, "host", "project", fakeCommits[3].Id, 0)
				So(err, ShouldBeNil)
				So(commits, ShouldResemble, res.Log[2:])
			})

			Convey("min is honored", func() {
				req2 := &gitilespb.LogRequest{
					Project:  "project",
					Treeish:  fakeCommits[2].Id,
					PageSize: 100,
				}
				res2 := &gitilespb.LogResponse{
					Log: fakeCommits[2:102],
				}
				gitilesMock.EXPECT().Log(gomock.Any(), req2).Return(res2, nil)

				commits, err := Log(c, "host", "project", fakeCommits[2].Id, 100)
				So(err, ShouldBeNil)
				So(commits, ShouldHaveLength, 100)
				So(commits, ShouldResemble, res2.Log)
			})

			Convey("request of item not in cache", func() {
				req2 := &gitilespb.LogRequest{
					Project:  "project",
					Treeish:  fakeCommits[101].Id,
					PageSize: 100,
				}
				res2 := &gitilespb.LogResponse{
					Log: fakeCommits[101:201],
				}
				gitilesMock.EXPECT().Log(gomock.Any(), req2).Return(res2, nil)
				commits, err := Log(c, "host", "project", fakeCommits[101].Id, 0)
				So(err, ShouldBeNil)
				So(commits, ShouldHaveLength, 100)
				So(commits, ShouldResemble, res2.Log)
			})

			Convey("do not update cache entries that have more info", func() {
				refCache := (&logReq{
					host:    "host",
					project: "project",
				}).mkCache(c, "refs/heads/master")
				c, err := info.Namespace(c, "git-log-v2")
				So(err, ShouldBeNil)
				err = memcache.Delete(c, refCache.Key())
				So(err, ShouldBeNil)

				req2 := &gitilespb.LogRequest{
					Project:  "project",
					Treeish:  "refs/heads/master",
					PageSize: 100,
				}
				res2 := &gitilespb.LogResponse{
					Log: fakeCommits[:100],
				}
				gitilesMock.EXPECT().Log(gomock.Any(), req2).Return(res2, nil)
				commits, err := Log(c, "host", "project", "refs/heads/master", 0)
				So(err, ShouldBeNil)
				So(commits, ShouldResemble, res2.Log)

				// ensure we still get 100-len log for second commit
				commits, err = Log(c, "host", "project", fakeCommits[1].Id, 1)
				So(err, ShouldBeNil)
				So(commits, ShouldHaveLength, 100)
			})
		})
	})
}
