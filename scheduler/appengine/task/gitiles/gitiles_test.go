// Copyright 2016 The LUCI Authors.
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

package gitiles

import (
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/config/validation"
	api "go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/utils/tasktest"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

var _ task.Manager = (*TaskManager)(nil)

func TestTriggerBuild(t *testing.T) {
	t.Parallel()

	Convey("LaunchTask Triggers Jobs", t, func() {
		c := memory.Use(context.Background())
		cfg := &messages.GitilesTask{
			Repo: "https://a.googlesource.com/b.git",
			Refs: []string{"refs/heads/master", "refs/heads/branch", "regexp:refs/branch-heads/[^/]+"},
		}
		jobID := "proj/gitiles"
		parsedURL, err := url.Parse(cfg.Repo)
		So(err, ShouldBeNil)

		type strmap map[string]string

		loadNoError := func() strmap {
			state, err := loadState(c, jobID, parsedURL)
			if err != nil {
				panic(err)
			}
			return state
		}

		ctl := &tasktest.TestController{
			TaskMessage:   cfg,
			Client:        http.DefaultClient,
			SaveCallback:  func() error { return nil },
			OverrideJobID: jobID,
		}
		// TODO(nodir): use goconvey.C instead of t in NewController.
		gitilesMock := gitilespb.NewMockGitilesClient(gomock.NewController(t))

		m := TaskManager{mockGitilesClient: gitilesMock}

		expectRefs := func(refsPath string, tips strmap) *gomock.Call {
			req := &gitilespb.RefsRequest{
				Project:  "b",
				RefsPath: refsPath,
			}
			res := &gitilespb.RefsResponse{
				Revisions: tips,
			}
			return gitilesMock.EXPECT().Refs(gomock.Any(), req).Return(res, nil)
		}
		// log is for readability of expectLog calls.
		log := func(ids ...string) []string { return ids }
		var epoch = time.Unix(1442270520, 0).UTC()
		expectLog := func(from, to string, pageSize int, ids []string, errs ...error) *gomock.Call {
			req := &gitilespb.LogRequest{
				Project:  "b",
				Treeish:  from,
				Ancestor: to,
				PageSize: int32(pageSize),
			}
			if len(errs) > 0 {
				return gitilesMock.EXPECT().Log(gomock.Any(), req).Return(nil, errs[0])
			}
			res := &gitilespb.LogResponse{}
			committedAt := epoch
			for _, id := range ids {
				// Ids go backwards in time, just as in `git log`.
				committedAt = committedAt.Add(-time.Minute)
				res.Log = append(res.Log, &git.Commit{
					Id:        id,
					Committer: &git.Commit_User{Time: google.NewTimestamp(committedAt)},
				})
			}
			return gitilesMock.EXPECT().Log(gomock.Any(), req).Return(res, nil)
		}

		Convey("new refs are discovered", func() {
			expectRefs("refs/heads", strmap{"refs/heads/master": "deadbeef00"})
			expectRefs("refs/branch-heads", strmap{"refs/weird": "1234567890"})
			expectLog("deadbeef00", "", 1, log("deadbeef00"))
			So(m.LaunchTask(c, ctl), ShouldBeNil)
			So(loadNoError(), ShouldResemble, strmap{
				"refs/heads/master": "deadbeef00",
			})
			So(ctl.Triggers, ShouldHaveLength, 1)
			So(ctl.Triggers[0].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/heads/master@deadbeef00")
			So(ctl.Triggers[0].GetGitiles(), ShouldResemble, &api.GitilesTrigger{
				Repo:     "https://a.googlesource.com/b.git",
				Ref:      "refs/heads/master",
				Revision: "deadbeef00",
			})
		})

		Convey("regexp refs are matched correctly", func() {
			cfg.Refs = []string{`regexp:refs/branch-heads/1\.\d+`}
			So(saveState(c, jobID, parsedURL, strmap{
				"refs/branch-heads/1.0": "deadcafe00",
				"refs/branch-heads/1.1": "beefcafe02",
			}), ShouldBeNil)
			expectRefs("refs/branch-heads", strmap{
				"refs/branch-heads/1.1":   "beefcafe00",
				"refs/branch-heads/1.2":   "deadbeef00",
				"refs/branch-heads/1.2.3": "deadbeef01",
			})
			expectLog("beefcafe00", "beefcafe02", 50, log("beefcafe00", "beefcafe01"))
			expectLog("deadbeef00", "", 1, log("deadbeef00"))
			So(m.LaunchTask(c, ctl), ShouldBeNil)

			So(loadNoError(), ShouldResemble, strmap{
				"refs/branch-heads/1.2": "deadbeef00",
				"refs/branch-heads/1.1": "beefcafe00",
			})
			So(ctl.Triggers, ShouldHaveLength, 3)
			So(ctl.Triggers[0].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/branch-heads/1.1@beefcafe01")
			So(ctl.Triggers[0].GetGitiles(), ShouldResemble, &api.GitilesTrigger{
				Repo:     "https://a.googlesource.com/b.git",
				Ref:      "refs/branch-heads/1.1",
				Revision: "beefcafe01",
			})
			So(ctl.Triggers[1].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/branch-heads/1.1@beefcafe00")
			So(ctl.Triggers[2].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/branch-heads/1.2@deadbeef00")
		})

		Convey("do not trigger if there are no new commits", func() {
			So(saveState(c, jobID, parsedURL, strmap{
				"refs/heads/master": "deadbeef00",
			}), ShouldBeNil)
			expectRefs("refs/heads", strmap{"refs/heads/master": "deadbeef00"})
			expectRefs("refs/branch-heads", strmap{"refs/weird": "1234567890"})
			So(m.LaunchTask(c, ctl), ShouldBeNil)
			So(ctl.Triggers, ShouldBeNil)
			So(loadNoError(), ShouldResemble, strmap{
				"refs/heads/master": "deadbeef00",
			})
		})

		Convey("New, updated, and deleted refs", func() {
			So(saveState(c, jobID, parsedURL, strmap{
				"refs/heads/master":   "deadbeef03",
				"refs/branch-heads/x": "1234567890",
				"refs/was/watched":    "0987654321",
			}), ShouldBeNil)
			expectRefs("refs/heads", strmap{
				"refs/heads/master": "deadbeef00",
			})
			expectRefs("refs/branch-heads", strmap{
				"refs/branch-heads/1.2.3": "baadcafe00",
			})
			expectLog("deadbeef00", "deadbeef03", 50, log("deadbeef00", "deadbeef01", "deadbeef02"))
			expectLog("baadcafe00", "", 1, log("baadcafe00"))

			So(m.LaunchTask(c, ctl), ShouldBeNil)
			So(loadNoError(), ShouldResemble, strmap{
				"refs/heads/master":       "deadbeef00",
				"refs/branch-heads/1.2.3": "baadcafe00",
			})
			So(ctl.Triggers, ShouldHaveLength, 4)
			// Ordered by ref, then by timestamp.
			So(ctl.Triggers[0].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/branch-heads/1.2.3@baadcafe00")
			So(ctl.Triggers[1].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/heads/master@deadbeef02")
			So(ctl.Triggers[2].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/heads/master@deadbeef01")
			So(ctl.Triggers[3].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/heads/master@deadbeef00")
			for i, t := range ctl.Triggers {
				So(t.OrderInBatch, ShouldEqual, i)
			}
			So(google.TimeFromProto(ctl.Triggers[0].Created), ShouldEqual, epoch.Add(-1*time.Minute))
			So(google.TimeFromProto(ctl.Triggers[1].Created), ShouldEqual, epoch.Add(-3*time.Minute)) // oldest on master
			So(google.TimeFromProto(ctl.Triggers[2].Created), ShouldEqual, epoch.Add(-2*time.Minute))
			So(google.TimeFromProto(ctl.Triggers[3].Created), ShouldEqual, epoch.Add(-1*time.Minute)) // newest on master
		})

		Convey("do nothing at all if there are no changes", func() {
			So(saveState(c, jobID, parsedURL, strmap{
				"refs/heads/master": "deadbeef",
			}), ShouldBeNil)
			expectRefs("refs/heads", strmap{
				"refs/heads/master": "deadbeef",
			})
			expectRefs("refs/branch-heads", nil)
			So(m.LaunchTask(c, ctl), ShouldBeNil)
			So(ctl.Triggers, ShouldBeNil)
			So(ctl.Log, ShouldNotContain, "Saved 1 known refs")
			So(ctl.Log, ShouldContain, "No changes detected")
			So(loadNoError(), ShouldResemble, strmap{
				"refs/heads/master": "deadbeef",
			})
		})

		Convey("Avoid choking on too many refs", func() {
			So(saveState(c, jobID, parsedURL, strmap{
				"refs/heads/master": "deadbeef",
			}), ShouldBeNil)
			expectRefs("refs/heads", nil).AnyTimes()
			expectRefs("refs/branch-heads", strmap{
				"refs/branch-heads/1": "cafee1",
				"refs/branch-heads/2": "cafee2",
				"refs/branch-heads/3": "cafee3",
				"refs/branch-heads/4": "cafee4",
				"refs/branch-heads/5": "cafee5",
			}).AnyTimes()
			expectLog("cafee1", "", 1, log("cafee1"))
			expectLog("cafee2", "", 1, log("cafee2"))
			expectLog("cafee3", "", 1, log("cafee3"))
			expectLog("cafee4", "", 1, log("cafee4"))
			expectLog("cafee5", "", 1, log("cafee5"))
			m.maxTriggersPerInvocation = 2
			m.maxCommitsPerRefUpdate = 1
			// First run, refs/branch-heads/{1,2} updated, refs/heads/master removed.
			So(m.LaunchTask(c, ctl), ShouldBeNil)
			So(ctl.Triggers, ShouldHaveLength, 2)
			So(loadNoError(), ShouldResemble, strmap{
				"refs/branch-heads/1": "cafee1",
				"refs/branch-heads/2": "cafee2",
			})
			ctl.Triggers = nil

			// Second run, refs/branch-heads/{3,4} updated.
			So(m.LaunchTask(c, ctl), ShouldBeNil)
			So(ctl.Triggers, ShouldHaveLength, 2)
			So(loadNoError(), ShouldResemble, strmap{
				"refs/branch-heads/1": "cafee1",
				"refs/branch-heads/2": "cafee2",
				"refs/branch-heads/3": "cafee3",
				"refs/branch-heads/4": "cafee4",
			})
			ctl.Triggers = nil

			// Final run, refs/branch-heads/5 updated.
			So(m.LaunchTask(c, ctl), ShouldBeNil)
			So(ctl.Triggers, ShouldHaveLength, 1)
			So(loadNoError(), ShouldResemble, strmap{
				"refs/branch-heads/1": "cafee1",
				"refs/branch-heads/2": "cafee2",
				"refs/branch-heads/3": "cafee3",
				"refs/branch-heads/4": "cafee4",
				"refs/branch-heads/5": "cafee5",
			})
		})

		Convey("Ensure progress", func() {
			expectRefs("refs/heads", nil).AnyTimes()
			expectRefs("refs/branch-heads", strmap{
				"refs/branch-heads/1": "cafee1",
				"refs/branch-heads/2": "cafee2",
				"refs/branch-heads/3": "cafee3",
			}).AnyTimes()
			m.maxTriggersPerInvocation = 2
			m.maxCommitsPerRefUpdate = 1

			Convey("no progress is an error", func() {
				expectLog("cafee1", "", 1, log(), errors.New("flake"))
				So(m.LaunchTask(c, ctl), ShouldErrLike, "flake")
			})

			expectLog("cafee1", "", 1, log("cafee1"))
			expectLog("cafee2", "", 1, log(), errors.New("flake"))
			So(m.LaunchTask(c, ctl), ShouldBeNil)
			So(ctl.Triggers, ShouldHaveLength, 1)
			So(loadNoError(), ShouldResemble, strmap{
				"refs/branch-heads/1": "cafee1",
			})
			ctl.Triggers = nil
			So(loadNoError(), ShouldResemble, strmap{
				"refs/branch-heads/1": "cafee1",
			})

			// Second run.
			expectLog("cafee2", "", 1, log("cafee2"))
			expectLog("cafee3", "", 1, log("cafee3"))
			So(m.LaunchTask(c, ctl), ShouldBeNil)
			So(ctl.Triggers, ShouldHaveLength, 2)
			So(loadNoError(), ShouldResemble, strmap{
				"refs/branch-heads/1": "cafee1",
				"refs/branch-heads/2": "cafee2",
				"refs/branch-heads/3": "cafee3",
			})
		})
	})
}

func TestValidateConfig(t *testing.T) {
	t.Parallel()
	c := context.Background()

	Convey("ValidateProtoMessage works", t, func() {
		ctx := &validation.Context{Context: c}
		m := TaskManager{}
		validate := func(msg proto.Message) error {
			m.ValidateProtoMessage(ctx, msg)
			return ctx.Finalize()
		}
		Convey("refNamespace works", func() {
			cfg := &messages.GitilesTask{
				Repo: "https://a.googlesource.com/b.git",
				Refs: []string{"refs/heads/master", "refs/heads/branch", "regexp:refs/branch-heads/[^/]+"},
			}
			Convey("proper refs", func() {
				So(validate(cfg), ShouldBeNil)
			})
			Convey("invalid ref", func() {
				cfg.Refs = []string{"wtf/not/a/ref"}
				So(validate(cfg), ShouldNotBeNil)
			})
		})

		Convey("refGlobs are rejected", func() {
			cfg := &messages.GitilesTask{
				Repo: "https://a.googlesource.com/b.git",
				Refs: []string{"refs/*"},
			}
			So(validate(cfg), ShouldNotBeNil)
		})

		Convey("refRegexp works", func() {
			cfg := &messages.GitilesTask{
				Repo: "https://a.googlesource.com/b.git",
				Refs: []string{`regexp:refs/heads/\d+`},
			}
			Convey("valid", func() {
				So(validate(cfg), ShouldBeNil)
			})
			Convey("^ and $ are not allowed", func() {
				cfg.Refs = []string{`regexp:^refs/heads/\d+$`}
				So(validate(cfg), ShouldNotBeNil)
			})
			Convey("invalid regexp", func() {
				cfg.Refs = []string{`regexp:a++`}
				So(validate(cfg), ShouldNotBeNil)
			})
			Convey("regexp matches single ref only", func() {
				cfg.Refs = []string{`regexp:refs/heads/master`}
				So(validate(cfg), ShouldNotBeNil)
			})
			Convey("regexp does not start with its own literal prefix", func() {
				cfg.Refs = []string{`regexp:re()fs/heads/\d+`}
				So(validate(cfg), ShouldNotBeNil)
			})
			Convey("too few slashes in literal prefix", func() {
				cfg.Refs = []string{`regexp:refs/release-\d+/foo`}
				So(validate(cfg), ShouldNotBeNil)
			})
			Convey("literal prefix does not start with refs/", func() {
				cfg.Refs = []string{`regexp:/refs/heads/\d+`}
				So(validate(cfg), ShouldNotBeNil)
			})
		})
	})
}

func TestRefNamespace(t *testing.T) {
	t.Parallel()

	Convey("splitRef works", t, func() {
		p, s := splitRef("refs/heads/master")
		So(p, ShouldEqual, "refs/heads")
		So(s, ShouldEqual, "master")
		p, s = splitRef("refs/weird/")
		So(p, ShouldEqual, "refs/weird")
		So(s, ShouldEqual, "")
	})
}
