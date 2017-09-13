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
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/utils/tasktest"

	"github.com/golang/protobuf/proto"
	. "github.com/smartystreets/goconvey/convey"
)

var _ task.Manager = (*TaskManager)(nil)

func TestTriggerBuild(t *testing.T) {

	Convey("LaunchTask Triggers Jobs", t, func() {
		c := memory.Use(context.Background())
		cfg := &messages.GitilesTask{
			Repo: "https://a.googlesource.com/b.git",
			Refs: []string{"refs/heads/master", "refs/heads/branch", "refs/branch-heads/1.2.3"},
		}
		loadSavedRepo := func() *Repository {
			r := &Repository{ID: "proj/gitiles:https://a.googlesource.com/b.git"}
			if err := ds.Get(c, r); err != nil {
				panic(err) // Stack is useful.
			}
			return r
		}
		ctl := &tasktest.TestController{
			TaskMessage:   cfg,
			Client:        http.DefaultClient,
			SaveCallback:  func() error { return nil },
			OverrideJobID: "proj/gitiles",
		}
		gitilesMock := &mockGitilesClient{}
		m := TaskManager{mockGitilesClient: gitilesMock}

		Convey("new refs are discovered", func() {
			gitilesMock.refs = func(ctx context.Context, repo string, path string) (map[string]string, error) {
				return map[string]string{
					"refs/heads/master": "deadbeef0",
					"refs/weird":        "123456789",
				}, nil
			}
			So(m.LaunchTask(c, ctl, nil), ShouldBeNil)
			So(loadSavedRepo(), ShouldResemble, &Repository{
				ID: "proj/gitiles:https://a.googlesource.com/b.git",
				References: []Reference{
					{Name: "refs/heads/master", Revision: "deadbeef0"},
				},
			})
			So(ctl.Triggers, ShouldHaveLength, 1)
			So(ctl.Triggers[0].ID, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/heads/master@deadbeef0")
			p := internal.TriggerPayload{}
			So(proto.Unmarshal(ctl.Triggers[0].Payload, &p), ShouldBeNil)
			So(p, ShouldResemble, internal.TriggerPayload{
				Gitiles: &internal.GitilesTrigger{
					Repo:     "https://a.googlesource.com/b.git",
					Ref:      "refs/heads/master",
					Revision: "deadbeef0",
				},
			})
		})

		Convey("do not trigger if there are no new commits", func() {
			ds.Put(c, &Repository{
				ID: "proj/gitiles:https://a.googlesource.com/b.git",
				References: []Reference{
					{Name: "refs/heads/master", Revision: "deadbeef0"},
				},
			})
			gitilesMock.refs = func(ctx context.Context, repo string, path string) (map[string]string, error) {
				return map[string]string{
					"refs/heads/master": "deadbeef0",
					"refs/weird":        "123456789",
				}, nil
			}
			So(m.LaunchTask(c, ctl, nil), ShouldBeNil)
			So(ctl.Triggers, ShouldBeNil)
			So(loadSavedRepo(), ShouldResemble, &Repository{
				ID: "proj/gitiles:https://a.googlesource.com/b.git",
				References: []Reference{
					{Name: "refs/heads/master", Revision: "deadbeef0"},
				},
			})
		})

		Convey("Updated, not changed and deleted refs", func() {
			ds.Put(c, &Repository{
				ID: "proj/gitiles:https://a.googlesource.com/b.git",
				References: []Reference{
					{Name: "refs/heads/master", Revision: "deadbeef0"},
					{Name: "refs/heads/branch", Revision: "123456789"},
					{Name: "refs/was/watched", Revision: "098765432"},
				},
			})
			gitilesMock.refs = func(ctx context.Context, repo string, path string) (map[string]string, error) {
				return map[string]string{
					"refs/heads/master":       "deadbeef1",
					"refs/branch-heads/1.2.3": "baadcafe0",
				}, nil
			}
			So(m.LaunchTask(c, ctl, nil), ShouldBeNil)
			So(loadSavedRepo(), ShouldResemble, &Repository{
				ID: "proj/gitiles:https://a.googlesource.com/b.git",
				References: []Reference{
					{Name: "refs/branch-heads/1.2.3", Revision: "baadcafe0"},
					{Name: "refs/heads/master", Revision: "deadbeef1"},
				},
			})
			So(ctl.Triggers, ShouldHaveLength, 2)
			So(ctl.Triggers[0].ID, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/branch-heads/1.2.3@baadcafe0")
			So(ctl.Triggers[1].ID, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/heads/master@deadbeef1")
		})

		Convey("do nothing at all if there are no changes", func() {
			ds.Put(c, &Repository{
				ID: "proj/gitiles:https://a.googlesource.com/b.git",
				References: []Reference{
					{Name: "refs/heads/master", Revision: "deadbeef0"},
				},
			})
			gitilesMock.refs = func(ctx context.Context, repo string, path string) (map[string]string, error) {
				return map[string]string{
					"refs/heads/master": "deadbeef0",
				}, nil
			}
			So(m.LaunchTask(c, ctl, nil), ShouldBeNil)
			So(ctl.Triggers, ShouldBeNil)
			So(ctl.Log, ShouldNotContain, "Saved 1 known refs")
			So(ctl.Log, ShouldContain, "No changes detected")
			So(loadSavedRepo(), ShouldResemble, &Repository{
				ID: "proj/gitiles:https://a.googlesource.com/b.git",
				References: []Reference{
					{Name: "refs/heads/master", Revision: "deadbeef0"},
				},
			})
		})
	})
}

type mockGitilesClient struct {
	refs func(ctx context.Context, repoURL, refsPath string) (map[string]string, error)
}

func (m *mockGitilesClient) Refs(c context.Context, repo string, path string) (map[string]string, error) {
	return m.refs(c, repo, path)
}
