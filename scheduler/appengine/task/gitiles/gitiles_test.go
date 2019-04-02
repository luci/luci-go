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
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/golang/protobuf/proto"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/retry/transient"
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
		}
		jobID := "proj/gitiles"

		type strmap map[string]string

		loadNoError := func() strmap {
			state, err := loadState(c, jobID, cfg.Repo)
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
		// expCommits is for readability of expectLog calls.
		log := func(ids ...string) []string { return ids }
		var epoch = time.Unix(1442270520, 0).UTC()
		expectLog := func(new, old string, pageSize int, ids []string, errs ...error) *gomock.Call {
			req := &gitilespb.LogRequest{
				Project:            "b",
				Committish:         new,
				ExcludeAncestorsOf: old,
				PageSize:           int32(pageSize),
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

		// expectLogWithDiff mocks Log call with result containing Tree Diff.
		// commitsWithFiles must be in the form of "sha1:comma,separated,files" and
		// if several must go backwards in time, just like git log.
		expectLogWithDiff := func(new, old string, pageSize int, commitsWithFiles ...string) *gomock.Call {
			req := &gitilespb.LogRequest{
				Project:            "b",
				Committish:         new,
				ExcludeAncestorsOf: old,
				PageSize:           int32(pageSize),
				TreeDiff:           true,
			}
			res := &gitilespb.LogResponse{}
			committedAt := epoch
			for _, cfs := range commitsWithFiles {
				parts := strings.SplitN(cfs, ":", 2)
				if len(parts) != 2 {
					panic(fmt.Errorf(`commitWithFiles must be in the form of "sha1:comma,separated,files", but given %q`, cfs))
				}
				id := parts[0]
				fileNames := strings.Split(parts[1], ",")
				diff := make([]*git.Commit_TreeDiff, len(fileNames))
				for i, f := range fileNames {
					diff[i] = &git.Commit_TreeDiff{NewPath: f}
				}
				committedAt = committedAt.Add(-time.Minute)
				res.Log = append(res.Log, &git.Commit{
					Id:        id,
					Committer: &git.Commit_User{Time: google.NewTimestamp(committedAt)},
					TreeDiff:  diff,
				})
			}
			return gitilesMock.EXPECT().Log(gomock.Any(), req).Return(res, nil)
		}

		Convey("each configured ref must match resolved ref", func() {
			cfg.Refs = []string{"refs/heads/master", `regexp:refs/branch-heads/\d+`}
			expectRefs("refs/heads", strmap{"refs/heads/not-master": "deadbeef00"})
			expectRefs("refs/branch-heads", strmap{"refs/branch-heads/not-digits": "deadbeef00"})
			So(m.LaunchTask(c, ctl), ShouldErrLike, "2 unresolved refs")
			So(ctl.Triggers, ShouldHaveLength, 0)
			So(ctl.Log[len(ctl.Log)-2], ShouldContainSubstring,
				"following configured refs didn't match a single actual ref:")
		})

		Convey("new refs are discovered", func() {
			cfg.Refs = []string{"refs/heads/master"}
			expectRefs("refs/heads", strmap{"refs/heads/master": "deadbeef00", "refs/weird": "123456"})
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

		Convey("newly discovered ref ignores pathFilter", func() {
			cfg.Refs = []string{"refs/heads/master"}
			cfg.PathRegexps = []string{"only.this.file"}
			expectRefs("refs/heads", strmap{"refs/heads/master": "deadbeef00"})
			expectLogWithDiff("deadbeef00", "", 1, "deadbeef00:some,files")
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
			So(saveState(c, jobID, cfg.Repo, strmap{
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
			cfg.Refs = []string{"regexp:refs/branch-heads/[^/]+"}
			So(saveState(c, jobID, cfg.Repo, strmap{
				"refs/branch-heads/beta": "deadbeef00",
			}), ShouldBeNil)
			expectRefs("refs/branch-heads", strmap{"refs/branch-heads/beta": "deadbeef00"})
			So(m.LaunchTask(c, ctl), ShouldBeNil)
			So(ctl.Triggers, ShouldBeNil)
			So(loadNoError(), ShouldResemble, strmap{
				"refs/branch-heads/beta": "deadbeef00",
			})
		})

		Convey("New, updated, and deleted refs", func() {
			cfg.Refs = []string{"refs/heads/master", "regexp:refs/branch-heads/[^/]+"}
			So(saveState(c, jobID, cfg.Repo, strmap{
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

		Convey("Updated ref with pathfilters", func() {
			cfg.Refs = []string{"refs/heads/master"}
			cfg.PathRegexps = []string{`.+\.emit`}
			cfg.PathRegexpsExclude = []string{`skip/.+`}
			So(saveState(c, jobID, cfg.Repo, strmap{"refs/heads/master": "deadbeef04"}), ShouldBeNil)
			expectRefs("refs/heads", strmap{"refs/heads/master": "deadbeef00"})
			expectLogWithDiff("deadbeef00", "deadbeef04", 50,
				"deadbeef00:skip/commit",
				"deadbeef01:yup.emit",
				"deadbeef02:skip/this-file,not-matched-file,but-still.emit",
				"deadbeef03:nothing-matched-means-skipped")

			So(m.LaunchTask(c, ctl), ShouldBeNil)
			So(loadNoError(), ShouldResemble, strmap{
				"refs/heads/master": "deadbeef00",
			})
			So(ctl.Triggers, ShouldHaveLength, 2)
			So(ctl.Triggers[0].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/heads/master@deadbeef02")
			So(ctl.Triggers[1].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/heads/master@deadbeef01")
		})

		Convey("Updated ref without matched commits", func() {
			cfg.Refs = []string{"refs/heads/master"}
			cfg.PathRegexps = []string{`must-match`}
			So(saveState(c, jobID, cfg.Repo, strmap{"refs/heads/master": "deadbeef04"}), ShouldBeNil)
			expectRefs("refs/heads", strmap{"refs/heads/master": "deadbeef00"})

			Convey("results in no emitted triggers if we examine each commit", func() {
				expectLogWithDiff("deadbeef00", "deadbeef04", 50,
					"deadbeef00:nope0",
					"deadbeef01:nope1",
					"deadbeef02:nope2",
					"deadbeef03:nope3")
				So(m.LaunchTask(c, ctl), ShouldBeNil)
				So(loadNoError(), ShouldResemble, strmap{
					"refs/heads/master": "deadbeef00",
				})
				So(ctl.Triggers, ShouldHaveLength, 0)
			})
			Convey("results in last commit emitted if we don't examine all commits", func() {
				m.maxCommitsPerRefUpdate = 2
				expectLogWithDiff("deadbeef00", "deadbeef04", 2,
					"deadbeef00:nope",
					"deadbeef01:nope1") // note deadbeef02 and 03 weren't examined.
				So(m.LaunchTask(c, ctl), ShouldBeNil)
				So(loadNoError(), ShouldResemble, strmap{
					"refs/heads/master": "deadbeef00",
				})
				So(ctl.Triggers, ShouldHaveLength, 1)
				So(ctl.Triggers[0].Id, ShouldEqual, "https://a.googlesource.com/b.git/+/refs/heads/master@deadbeef00")
			})
		})

		Convey("do nothing at all if there are no changes", func() {
			cfg.Refs = []string{"refs/heads/master"}
			So(saveState(c, jobID, cfg.Repo, strmap{
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
			cfg.Refs = []string{"refs/heads/master", "regexp:refs/branch-heads/[^/]+"}
			So(saveState(c, jobID, cfg.Repo, strmap{
				"refs/heads/master": "deadbeef",
			}), ShouldBeNil)
			expectRefs("refs/heads", strmap{"refs/heads/master": "deadbeef"}).AnyTimes()
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
			// First run, refs/branch-heads/{1,2} updated, refs/heads/master preserved.
			So(m.LaunchTask(c, ctl), ShouldBeNil)
			So(ctl.Triggers, ShouldHaveLength, 2)
			So(loadNoError(), ShouldResemble, strmap{
				"refs/heads/master":   "deadbeef",
				"refs/branch-heads/1": "cafee1",
				"refs/branch-heads/2": "cafee2",
			})
			ctl.Triggers = nil

			// Second run, refs/branch-heads/{3,4} updated.
			So(m.LaunchTask(c, ctl), ShouldBeNil)
			So(ctl.Triggers, ShouldHaveLength, 2)
			So(loadNoError(), ShouldResemble, strmap{
				"refs/heads/master":   "deadbeef",
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
				"refs/heads/master":   "deadbeef",
				"refs/branch-heads/1": "cafee1",
				"refs/branch-heads/2": "cafee2",
				"refs/branch-heads/3": "cafee3",
				"refs/branch-heads/4": "cafee4",
				"refs/branch-heads/5": "cafee5",
			})
		})

		Convey("Ensure progress", func() {
			cfg.Refs = []string{"regexp:refs/branch-heads/[^/]+"}
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

		Convey("distinguish force push from transient weirdness", func() {
			cfg.Refs = []string{"refs/heads/master"}
			So(saveState(c, jobID, cfg.Repo, strmap{
				"refs/heads/master": "001d", // old.
			}), ShouldBeNil)
			expectRefs("refs/heads", strmap{"refs/heads/master": "1111"})

			Convey("force push going backwards", func() {
				expectLog("1111", "001d", 50, log())
				So(m.LaunchTask(c, ctl), ShouldBeNil)
				// Changes state
				So(loadNoError(), ShouldResemble, strmap{
					"refs/heads/master": "1111",
				})
				// .. but no triggers, since there are no new commits.
				So(ctl.Triggers, ShouldHaveLength, 0)
			})

			Convey("force push wiping out prior HEAD", func() {
				expectLog("1111", "001d", 50, nil, grpc.Errorf(codes.NotFound, "not found"))
				expectLog("1111", "", 1, log("1111"))
				expectLog("001d", "", 1, nil, grpc.Errorf(codes.NotFound, "not found"))
				So(m.LaunchTask(c, ctl), ShouldBeNil)
				So(loadNoError(), ShouldResemble, strmap{
					"refs/heads/master": "1111",
				})
				So(ctl.Triggers, ShouldHaveLength, 1)
			})

			Convey("race 1", func() {
				expectLog("1111", "001d", 50, nil, grpc.Errorf(codes.NotFound, "not found"))
				expectLog("1111", "", 1, nil, grpc.Errorf(codes.NotFound, "not found"))
				So(transient.Tag.In(m.LaunchTask(c, ctl)), ShouldBeTrue)
				So(loadNoError(), ShouldResemble, strmap{
					"refs/heads/master": "001d", // no change.
				})
			})

			Convey("race or fluke", func() {
				expectLog("1111", "001d", 50, nil, grpc.Errorf(codes.NotFound, "not found"))
				expectLog("1111", "", 1, nil, grpc.Errorf(codes.NotFound, "not found"))
				expectLog("001d", "", 1, nil, grpc.Errorf(codes.NotFound, "not found"))
				So(m.LaunchTask(c, ctl), ShouldNotBeNil)
				So(loadNoError(), ShouldResemble, strmap{
					"refs/heads/master": "001d",
				})
			})
		})
	})
}

func TestPathFilterHelpers(t *testing.T) {
	t.Parallel()

	Convey("PathFilter helpers work", t, func() {
		Convey("disjunctiveOfRegexps works", func() {
			So(disjunctiveOfRegexps([]string{`.+\.cpp`}), ShouldEqual, `^((.+\.cpp))$`)
			So(disjunctiveOfRegexps([]string{`.+\.cpp`, `?a`}), ShouldEqual, `^((.+\.cpp)|(?a))$`)
		})
		Convey("pathFilter works", func() {
			Convey("simple", func() {
				empty, err := newPathFilter(&messages.GitilesTask{})
				So(err, ShouldBeNil)
				So(empty.active(), ShouldBeFalse)
				_, err = newPathFilter(&messages.GitilesTask{PathRegexps: []string{`\K`}})
				So(err, ShouldNotBeNil)
				_, err = newPathFilter(&messages.GitilesTask{PathRegexps: []string{`a?`}, PathRegexpsExclude: []string{`\K`}})
				So(err, ShouldNotBeNil)

			})
			Convey("just negative ignored", func() {
				v, err := newPathFilter(&messages.GitilesTask{PathRegexpsExclude: []string{`.+\.cpp`}})
				So(err, ShouldBeNil)
				So(v.active(), ShouldBeFalse)
			})

			Convey("just positive", func() {
				v, err := newPathFilter(&messages.GitilesTask{PathRegexps: []string{`.+`}})
				So(err, ShouldBeNil)
				So(v.active(), ShouldBeTrue)
				Convey("empty commit is not interesting", func() {
					So(v.isInteresting([]*git.Commit_TreeDiff{}), ShouldBeFalse)
				})
				Convey("new or old paths are taken into account", func() {
					So(v.isInteresting([]*git.Commit_TreeDiff{{OldPath: "old"}}), ShouldBeTrue)
					So(v.isInteresting([]*git.Commit_TreeDiff{{NewPath: "new"}}), ShouldBeTrue)
				})
			})

			genDiff := func(files ...string) []*git.Commit_TreeDiff {
				r := make([]*git.Commit_TreeDiff, len(files))
				for i, f := range files {
					if i&1 == 0 {
						r[i] = &git.Commit_TreeDiff{OldPath: f}
					} else {
						r[i] = &git.Commit_TreeDiff{NewPath: f}
					}
				}
				return r
			}

			Convey("many positives", func() {
				v, err := newPathFilter(&messages.GitilesTask{PathRegexps: []string{`.+\.cpp`, "exact"}})
				So(err, ShouldBeNil)
				So(v.isInteresting(genDiff("not.matched")), ShouldBeFalse)

				So(v.isInteresting(genDiff("matched.cpp")), ShouldBeTrue)
				So(v.isInteresting(genDiff("exact")), ShouldBeTrue)
				So(v.isInteresting(genDiff("at least", "one", "matched.cpp")), ShouldBeTrue)
			})

			Convey("many negatives", func() {
				v, err := newPathFilter(&messages.GitilesTask{
					PathRegexps:        []string{`.+`},
					PathRegexpsExclude: []string{`.+\.cpp`, `excluded`},
				})
				So(err, ShouldBeNil)
				So(v.isInteresting(genDiff("not excluded")), ShouldBeTrue)
				So(v.isInteresting(genDiff("excluded/is/a/dir/not/a/file")), ShouldBeTrue)
				So(v.isInteresting(genDiff("excluded", "also.excluded.cpp", "but this file isn't")), ShouldBeTrue)

				So(v.isInteresting(genDiff("excluded.cpp")), ShouldBeFalse)
				So(v.isInteresting(genDiff("excluded")), ShouldBeFalse)
				So(v.isInteresting(genDiff()), ShouldBeFalse)
			})

			Convey("smoke test for complexity", func() {
				v, err := newPathFilter(&messages.GitilesTask{
					PathRegexps:        []string{`.+/\d\.py`, `included/.+`},
					PathRegexpsExclude: []string{`.+\.cpp`, `excluded/.*`},
				})
				So(err, ShouldBeNil)
				So(v.isInteresting(genDiff("excluded/1", "also.cpp", "included/one-is-enough")), ShouldBeTrue)
				So(v.isInteresting(genDiff("included/but-also-excluded.cpp", "one-still-enough/1.py")), ShouldBeTrue)

				So(v.isInteresting(genDiff("included/but-also-excluded.cpp", "excluded/2.py")), ShouldBeFalse)
				So(v.isInteresting(genDiff("matches nothing")), ShouldBeFalse)
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

		Convey("refRegexp works", func() {
			cfg := &messages.GitilesTask{
				Repo: "https://a.googlesource.com/b.git",
				Refs: []string{
					`regexp:refs/heads/\d+`,
					`regexp:refs/actually/exact`,
					`refs/heads/master`,
				},
			}
			Convey("valid", func() {
				So(validate(cfg), ShouldBeNil)
			})
			Convey("invalid regexp", func() {
				cfg.Refs = []string{`regexp:a++`}
				So(validate(cfg), ShouldNotBeNil)
			})
		})

		Convey("pathRegexs works", func() {
			cfg := &messages.GitilesTask{
				Repo:               "https://a.googlesource.com/b.git",
				Refs:               []string{"refs/heads/master"},
				PathRegexps:        []string{`.+\.cpp`},
				PathRegexpsExclude: []string{`.+\.py`},
			}
			Convey("valid", func() {
				So(validate(cfg), ShouldBeNil)
			})
			Convey("can't even parse", func() {
				cfg.PathRegexpsExclude = []string{`\K`}
				So(validate(cfg), ShouldNotBeNil)
			})
			Convey("redundant", func() {
				cfg.PathRegexps = []string{``}
				So(validate(cfg), ShouldNotBeNil)
				cfg.PathRegexps = []string{`^file`}
				So(validate(cfg), ShouldNotBeNil)
				cfg.PathRegexps = []string{`file$`}
				So(validate(cfg), ShouldNotBeNil)
			})
			Convey("excludes require includes", func() {
				cfg.PathRegexps = nil
				So(validate(cfg), ShouldNotBeNil)
			})
		})
	})
}
