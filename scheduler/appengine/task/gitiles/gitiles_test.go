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
	"strings"
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/scheduler/appengine/internal"
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

		loadNoError := func() map[string]string {
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
		gitilesMock := &mockGitilesClient{}
		m := TaskManager{mockGitilesClient: gitilesMock}

		Convey("new refs are discovered", func() {
			gitilesMock.allRefs = func() (map[string]string, error) {
				return map[string]string{
					"refs/heads/master": "deadbeef00",
					"refs/weird":        "1234567890",
				}, nil
			}
			So(m.LaunchTask(c, ctl, nil), ShouldBeNil)
			So(loadNoError(), ShouldResemble, map[string]string{
				"refs/heads/master": "deadbeef00",
			})
			So(ctl.Triggers, ShouldHaveLength, 1)
			So(ctl.Triggers[0].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/heads/master@deadbeef00")
			So(ctl.Triggers[0].GetGitiles(), ShouldResemble, &internal.GitilesTriggerData{
				Repo:     "https://a.googlesource.com/b.git",
				Ref:      "refs/heads/master",
				Revision: "deadbeef00",
			})
		})

		Convey("do not trigger if there are no new commits", func() {
			So(saveState(c, jobID, parsedUrl, map[string]string{
				"refs/heads/master": "deadbeef00",
			}), ShouldBeNil)
			gitilesMock.allRefs = func() (map[string]string, error) {
				return map[string]string{
					"refs/heads/master": "deadbeef00",
					"refs/weird":        "1234567890",
				}, nil
			}
			So(m.LaunchTask(c, ctl, nil), ShouldBeNil)
			So(ctl.Triggers, ShouldBeNil)
			So(loadNoError(), ShouldResemble, map[string]string{
				"refs/heads/master": "deadbeef00",
			})
		})

		Convey("New, updated, and deleted refs", func() {
			So(saveState(c, jobID, parsedUrl, map[string]string{
				"refs/heads/master": "deadbeef00",
				"refs/heads/branch": "1234567890",
				"refs/was/watched":  "0987654321",
			}), ShouldBeNil)
			gitilesMock.allRefs = func() (map[string]string, error) {
				return map[string]string{
					"refs/heads/master":       "deadbeef01",
					"refs/branch-heads/1.2.3": "baadcafe00",
				}, nil
			}
			So(m.LaunchTask(c, ctl, nil), ShouldBeNil)
			So(loadNoError(), ShouldResemble, map[string]string{
				"refs/heads/master":       "deadbeef01",
				"refs/branch-heads/1.2.3": "baadcafe00",
			})
			So(ctl.Triggers, ShouldHaveLength, 2)
			So(ctl.Triggers[0].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/branch-heads/1.2.3@baadcafe00")
			So(ctl.Triggers[1].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/heads/master@deadbeef01")
		})

		Convey("Refglobs: new, updated, and deleted refs", func() {
			So(saveState(c, jobID, parsedUrl, map[string]string{
				"refs/branch-heads/1.2.3": "deadbeef00",
				"refs/branch-heads/4.5":   "beefcafe",
				"refs/branch-heads/6.7":   "deadcafe",
			}), ShouldBeNil)
			gitilesMock.allRefs = func() (map[string]string, error) {
				return map[string]string{
					"refs/branch-heads/1.2.3":          "deadbeef01",
					"refs/branch-heads/6.7":            "deadcafe",
					"refs/branch-heads/8.9.0":          "beef44dead",
					"refs/branch-heads/must/not/match": "deaddead",
				}, nil
			}
			So(m.LaunchTask(c, ctl, nil), ShouldBeNil)
			So(loadNoError(), ShouldResemble, map[string]string{
				"refs/branch-heads/1.2.3": "deadbeef01", // updated.
				"refs/branch-heads/6.7":   "deadcafe",   // same.
				"refs/branch-heads/8.9.0": "beef44dead", // new.
			})
			So(ctl.Triggers, ShouldHaveLength, 2)
			So(ctl.Triggers[0].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/branch-heads/1.2.3@deadbeef01")
			So(ctl.Triggers[1].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/branch-heads/8.9.0@beef44dead")
		})

		Convey("do nothing at all if there are no changes", func() {
			So(saveState(c, jobID, parsedUrl, map[string]string{
				"refs/heads/master": "deadbeef",
			}), ShouldBeNil)
			gitilesMock.allRefs = func() (map[string]string, error) {
				return map[string]string{
					"refs/heads/master": "deadbeef",
				}, nil
			}
			So(m.LaunchTask(c, ctl, nil), ShouldBeNil)
			So(ctl.Triggers, ShouldBeNil)
			So(ctl.Log, ShouldNotContain, "Saved 1 known refs")
			So(ctl.Log, ShouldContain, "No changes detected")
			So(loadNoError(), ShouldResemble, map[string]string{
				"refs/heads/master": "deadbeef",
			})
		})

		Convey("Avoid choking on too many refs", func() {
			So(saveState(c, jobID, parsedUrl, map[string]string{
				"refs/heads/master": "deadbeef",
			}), ShouldBeNil)
			gitilesMock.allRefs = func() (map[string]string, error) {
				return map[string]string{
					"refs/branch-heads/1": "cafee1",
					"refs/branch-heads/2": "cafee2",
					"refs/branch-heads/3": "cafee3",
					"refs/branch-heads/4": "cafee4",
					"refs/branch-heads/5": "cafee5",
				}, nil
			}

			m.maxTriggersPerInvocation = 2
			// First run, refs/branch-heads/{1,2} updated, refs/heads/master removed.
			So(m.LaunchTask(c, ctl, nil), ShouldBeNil)
			So(ctl.Triggers, ShouldHaveLength, 2)
			So(loadNoError(), ShouldResemble, map[string]string{
				"refs/branch-heads/1": "cafee1",
				"refs/branch-heads/2": "cafee2",
			})
			ctl.Triggers = nil

			// Second run, refs/branch-heads/{3,4} updated.
			So(m.LaunchTask(c, ctl, nil), ShouldBeNil)
			So(ctl.Triggers, ShouldHaveLength, 2)
			So(loadNoError(), ShouldResemble, map[string]string{
				"refs/branch-heads/1": "cafee1",
				"refs/branch-heads/2": "cafee2",
				"refs/branch-heads/3": "cafee3",
				"refs/branch-heads/4": "cafee4",
			})
			ctl.Triggers = nil

			// Final run, refs/branch-heads/5 updated.
			So(m.LaunchTask(c, ctl, nil), ShouldBeNil)
			So(ctl.Triggers, ShouldHaveLength, 1)
			So(loadNoError(), ShouldResemble, map[string]string{
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

	Convey("refNamespace works", t, func() {
		cfg := &messages.GitilesTask{
			Repo: "https://a.googlesource.com/b.git",
			Refs: []string{"refs/heads/master", "refs/heads/branch", "refs/branch-heads/*"},
		}
		m := TaskManager{}
		So(m.ValidateProtoMessage(c, cfg), ShouldBeNil)

		cfg.Refs = []string{"wtf/not/a/ref"}
		So(m.ValidateProtoMessage(c, cfg), ShouldNotBeNil)
	})

	Convey("trailing refGlobs work", t, func() {
		cfg := &messages.GitilesTask{
			Repo: "https://a.googlesource.com/b.git",
			Refs: []string{"refs/*", "refs/heads/*", "refs/other/something"},
		}
		m := TaskManager{}
		So(m.ValidateProtoMessage(c, cfg), ShouldBeNil)

		cfg.Refs = []string{"refs/*/*"}
		So(m.ValidateProtoMessage(c, cfg), ShouldNotBeNil)
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

type mockGitilesClient struct {
	allRefs func() (map[string]string, error)
}

func (m *mockGitilesClient) Refs(c context.Context, repo string, path string) (map[string]string, error) {
	refs, err := m.allRefs()
	if err != nil {
		return nil, err
	}
	// Returns only those refs which reside in given path namespace.
	for ref := range refs {
		if ref == path || strings.HasPrefix(ref, path+"/") {
			continue
		}
		delete(refs, ref)
	}
	return refs, err
}

func TestRefsMock(t *testing.T) {
	t.Parallel()

	Convey("Refs Mock works", t, func() {
		c := context.Background()
		repo := "https://repo/whatever.git"
		m := mockGitilesClient{}
		Convey("Error propagation", func() {
			m.allRefs = func() (map[string]string, error) { return nil, errors.New("wtf") }
			_, err := m.Refs(c, repo, "refs")
			So(err, ShouldNotBeNil)
		})
		Convey("Correct namespacing", func() {
			m.allRefs = func() (map[string]string, error) {
				return map[string]string{
					"refs/heads/master":       "1",
					"refs/heads/infra/config": "2",
					"refs/wtf/bar":            "3",
					"refs/wtf/foo":            "4",
					"refs/wtf2":               "5",
				}, nil
			}

			refs, err := m.Refs(c, repo, "refs")
			So(err, ShouldBeNil)
			So(refs, ShouldResemble, map[string]string{
				"refs/heads/master":       "1",
				"refs/heads/infra/config": "2",
				"refs/wtf/bar":            "3",
				"refs/wtf/foo":            "4",
				"refs/wtf2":               "5",
			})

			refs, err = m.Refs(c, repo, "refs/heads")
			So(err, ShouldBeNil)
			So(refs, ShouldResemble, map[string]string{
				"refs/heads/master":       "1",
				"refs/heads/infra/config": "2",
			})

			refs, err = m.Refs(c, repo, "refs/heads/infra")
			So(err, ShouldBeNil)
			So(refs, ShouldResemble, map[string]string{
				"refs/heads/infra/config": "2",
			})

			refs, err = m.Refs(c, repo, "refs/wtf")
			So(err, ShouldBeNil)
			So(refs, ShouldResemble, map[string]string{
				"refs/wtf/bar": "3",
				"refs/wtf/foo": "4",
			})
		})
	})
}
