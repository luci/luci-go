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

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"
	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/memory"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/config/validation"
	api "go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/utils/tasktest"

	. "github.com/smartystreets/goconvey/convey"
)

var _ task.Manager = (*TaskManager)(nil)

func TestTriggerBuild(t *testing.T) {
	t.Parallel()

	Convey("LaunchTask Triggers Jobs", t, func() {
		c := memory.Use(context.Background())
		cfg := &messages.GitilesTask{
			Repo: "https://a.googlesource.com/b.git",
			Refs: []string{"refs/heads/master", "refs/heads/branch", "refs/branch-heads/*"},
		}
		jobID := "proj/gitiles"
		parsedUrl, err := url.Parse(cfg.Repo)
		So(err, ShouldBeNil)

		type strmap map[string]string

		loadNoError := func() strmap {
			state, err := loadState(c, jobID, parsedUrl)
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

		Convey("new refs are discovered", func() {
			expectRefs("refs/heads", strmap{"refs/heads/master": "deadbeef00"})
			expectRefs("refs/branch-heads", strmap{"refs/weird": "1234567890"})
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

		Convey("do not trigger if there are no new commits", func() {
			So(saveState(c, jobID, parsedUrl, strmap{
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
			So(saveState(c, jobID, parsedUrl, strmap{
				"refs/heads/master":   "deadbeef00",
				"refs/branch-heads/x": "1234567890",
				"refs/was/watched":    "0987654321",
			}), ShouldBeNil)
			expectRefs("refs/heads", strmap{
				"refs/heads/master": "deadbeef01",
			})
			expectRefs("refs/branch-heads", strmap{
				"refs/branch-heads/1.2.3": "baadcafe00",
			})

			So(m.LaunchTask(c, ctl), ShouldBeNil)
			So(loadNoError(), ShouldResemble, strmap{
				"refs/heads/master":       "deadbeef01",
				"refs/branch-heads/1.2.3": "baadcafe00",
			})
			So(ctl.Triggers, ShouldHaveLength, 2)
			So(ctl.Triggers[0].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/branch-heads/1.2.3@baadcafe00")
			So(ctl.Triggers[1].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/heads/master@deadbeef01")
		})

		Convey("Refglobs: new, updated, and deleted refs", func() {
			So(saveState(c, jobID, parsedUrl, strmap{
				"refs/branch-heads/1.2.3": "deadbeef00",
				"refs/branch-heads/4.5":   "beefcafe",
				"refs/branch-heads/6.7":   "deadcafe",
			}), ShouldBeNil)
			expectRefs("refs/heads", nil)
			expectRefs("refs/branch-heads", strmap{
				"refs/branch-heads/1.2.3":          "deadbeef01",
				"refs/branch-heads/6.7":            "deadcafe",
				"refs/branch-heads/8.9.0":          "beef44dead",
				"refs/branch-heads/must/not/match": "deaddead",
			})

			So(m.LaunchTask(c, ctl), ShouldBeNil)
			So(loadNoError(), ShouldResemble, strmap{
				"refs/branch-heads/1.2.3": "deadbeef01", // updated.
				"refs/branch-heads/6.7":   "deadcafe",   // same.
				"refs/branch-heads/8.9.0": "beef44dead", // new.
			})
			So(ctl.Triggers, ShouldHaveLength, 2)
			So(ctl.Triggers[0].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/branch-heads/1.2.3@deadbeef01")
			So(ctl.Triggers[1].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/branch-heads/8.9.0@beef44dead")
		})

		Convey("do nothing at all if there are no changes", func() {
			So(saveState(c, jobID, parsedUrl, strmap{
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
			So(saveState(c, jobID, parsedUrl, strmap{
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
			m.maxTriggersPerInvocation = 2
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
				Refs: []string{"refs/heads/master", "refs/heads/branch", "refs/branch-heads/*"},
			}
			Convey("proper refs", func() {
				So(validate(cfg), ShouldBeNil)
			})
			Convey("invalid ref", func() {
				cfg.Refs = []string{"wtf/not/a/ref"}
				So(validate(cfg), ShouldNotBeNil)
			})
		})

		Convey("trailing refGlobs work", func() {
			cfg := &messages.GitilesTask{
				Repo: "https://a.googlesource.com/b.git",
				Refs: []string{"refs/*", "refs/heads/*", "refs/other/something"},
			}
			Convey("valid refGlobs", func() {
				So(validate(cfg), ShouldBeNil)
			})
			Convey("invalid refGlob", func() {
				cfg.Refs = []string{"refs/*/*"}
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
