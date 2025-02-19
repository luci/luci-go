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
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	commonpb "go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/common/proto/git"
	gitilespb "go.chromium.org/luci/common/proto/gitiles"
	"go.chromium.org/luci/common/proto/gitiles/mock_gitiles"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/config/validation"
	"go.chromium.org/luci/gae/impl/memory"

	api "go.chromium.org/luci/scheduler/api/scheduler/v1"
	"go.chromium.org/luci/scheduler/appengine/messages"
	"go.chromium.org/luci/scheduler/appengine/task"
	"go.chromium.org/luci/scheduler/appengine/task/utils/tasktest"
)

var _ task.Manager = (*TaskManager)(nil)

func TestTriggerBuild(t *testing.T) {
	t.Parallel()

	ftt.Run("LaunchTask Triggers Jobs", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		cfg := &messages.GitilesTask{
			Repo: "https://a.googlesource.com/b.git",
		}
		jobID := "proj/gitiles"

		type strmap map[string]string

		loadNoError := func() strmap {
			state, _, err := loadState(c, jobID, cfg.Repo)
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

		mockCtrl := gomock.NewController(t)
		gitilesMock := mock_gitiles.NewMockGitilesClient(mockCtrl)

		m := TaskManager{mockGitilesClient: gitilesMock}

		expectRefs := func(refsPath string, tips strmap) *gomock.Call {
			req := &gitilespb.RefsRequest{
				Project:  "b",
				RefsPath: refsPath,
			}
			res := &gitilespb.RefsResponse{
				Revisions: tips,
			}
			return gitilesMock.EXPECT().Refs(gomock.Any(), commonpb.MatcherEqual(req)).Return(res, nil)
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
				return gitilesMock.EXPECT().Log(gomock.Any(), commonpb.MatcherEqual(req)).Return(nil, errs[0])
			}
			res := &gitilespb.LogResponse{}
			committedAt := epoch
			for _, id := range ids {
				// Ids go backwards in time, just as in `git log`.
				committedAt = committedAt.Add(-time.Minute)
				res.Log = append(res.Log, &git.Commit{
					Id:        id,
					Committer: &git.Commit_User{Time: timestamppb.New(committedAt)},
				})
			}
			return gitilesMock.EXPECT().Log(gomock.Any(), commonpb.MatcherEqual(req)).Return(res, nil)
		}

		// expectLogWithDiff mocks Log call with result containing Tree Diff.
		// commitsWithFiles must be in the form of "sha1:comma,separated,files" and
		// if several must go backwards in time, just like git log.
		expectLogWithDiff := func(new, old string, pageSize int, project string, commitsWithFiles ...string) *gomock.Call {
			req := &gitilespb.LogRequest{
				Project:            project,
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
					Committer: &git.Commit_User{Time: timestamppb.New(committedAt)},
					TreeDiff:  diff,
				})
			}
			return gitilesMock.EXPECT().Log(gomock.Any(), commonpb.MatcherEqual(req)).Return(res, nil)
		}

		t.Run("each configured ref must match resolved ref", func(t *ftt.Test) {
			cfg.Refs = []string{"refs/heads/master", `regexp:refs/branch-heads/\d+`}
			expectRefs("refs/heads", strmap{"refs/heads/not-master": "deadbeef00"})
			expectRefs("refs/branch-heads", strmap{"refs/branch-heads/not-digits": "deadbeef00"})
			assert.Loosely(t, m.LaunchTask(c, ctl), should.ErrLike("2 unresolved refs"))
			assert.Loosely(t, ctl.Triggers, should.HaveLength(0))
			assert.Loosely(t, ctl.Log[len(ctl.Log)-2], should.ContainSubstring(
				"following configured refs didn't match a single actual ref:"))
		})

		t.Run("new refs are discovered", func(t *ftt.Test) {
			cfg.Refs = []string{"refs/heads/master"}
			expectRefs("refs/heads", strmap{"refs/heads/master": "deadbeef00", "refs/weird": "123456"})
			expectLog("deadbeef00", "", 1, log("deadbeef00"))
			assert.Loosely(t, m.LaunchTask(c, ctl), should.BeNil)
			assert.Loosely(t, loadNoError(), should.Resemble(strmap{
				"refs/heads/master": "deadbeef00",
			}))
			assert.Loosely(t, ctl.Triggers, should.HaveLength(1))
			assert.Loosely(t, ctl.Triggers[0].Id, should.Equal("https://a.googlesource.com/b.git/+/refs/heads/master@deadbeef00"))
			assert.Loosely(t, ctl.Triggers[0].GetGitiles(), should.Resemble(&api.GitilesTrigger{
				Repo:     "https://a.googlesource.com/b.git",
				Ref:      "refs/heads/master",
				Revision: "deadbeef00",
			}))
		})

		t.Run("regexp refs are matched correctly", func(t *ftt.Test) {
			cfg.Refs = []string{`regexp:refs/branch-heads/1\.\d+`}
			assert.Loosely(t, saveState(c, jobID, cfg, strmap{
				"refs/branch-heads/1.0": "deadcafe00",
				"refs/branch-heads/1.1": "beefcafe02",
			}), should.BeNil)
			expectRefs("refs/branch-heads", strmap{
				"refs/branch-heads/1.1":   "beefcafe00",
				"refs/branch-heads/1.2":   "deadbeef00",
				"refs/branch-heads/1.2.3": "deadbeef01",
			})
			expectLog("beefcafe00", "beefcafe02", 50, log("beefcafe00", "beefcafe01"))
			expectLog("deadbeef00", "", 1, log("deadbeef00"))
			assert.Loosely(t, m.LaunchTask(c, ctl), should.BeNil)

			assert.Loosely(t, loadNoError(), should.Resemble(strmap{
				"refs/branch-heads/1.2": "deadbeef00",
				"refs/branch-heads/1.1": "beefcafe00",
			}))
			assert.Loosely(t, ctl.Triggers, should.HaveLength(3))
			assert.Loosely(t, ctl.Triggers[0].Id, should.Equal("https://a.googlesource.com/b.git/+/refs/branch-heads/1.1@beefcafe01"))
			assert.Loosely(t, ctl.Triggers[0].GetGitiles(), should.Resemble(&api.GitilesTrigger{
				Repo:     "https://a.googlesource.com/b.git",
				Ref:      "refs/branch-heads/1.1",
				Revision: "beefcafe01",
			}))
			assert.Loosely(t, ctl.Triggers[1].Id, should.Equal("https://a.googlesource.com/b.git/+/refs/branch-heads/1.1@beefcafe00"))
			assert.Loosely(t, ctl.Triggers[2].Id, should.Equal("https://a.googlesource.com/b.git/+/refs/branch-heads/1.2@deadbeef00"))
		})

		t.Run("do not trigger if there are no new commits", func(t *ftt.Test) {
			cfg.Refs = []string{"regexp:refs/branch-heads/[^/]+"}
			assert.Loosely(t, saveState(c, jobID, cfg, strmap{
				"refs/branch-heads/beta": "deadbeef00",
			}), should.BeNil)
			expectRefs("refs/branch-heads", strmap{"refs/branch-heads/beta": "deadbeef00"})
			assert.Loosely(t, m.LaunchTask(c, ctl), should.BeNil)
			assert.Loosely(t, ctl.Triggers, should.BeNil)
			assert.Loosely(t, loadNoError(), should.Resemble(strmap{
				"refs/branch-heads/beta": "deadbeef00",
			}))
		})

		t.Run("New, updated, and deleted refs", func(t *ftt.Test) {
			cfg.Refs = []string{"refs/heads/master", "regexp:refs/branch-heads/[^/]+"}
			assert.Loosely(t, saveState(c, jobID, cfg, strmap{
				"refs/heads/master":   "deadbeef03",
				"refs/branch-heads/x": "1234567890",
				"refs/was/watched":    "0987654321",
			}), should.BeNil)
			expectRefs("refs/heads", strmap{
				"refs/heads/master": "deadbeef00",
			})
			expectRefs("refs/branch-heads", strmap{
				"refs/branch-heads/1.2.3": "baadcafe00",
			})
			expectLog("deadbeef00", "deadbeef03", 50, log("deadbeef00", "deadbeef01", "deadbeef02"))
			expectLog("baadcafe00", "", 1, log("baadcafe00"))

			assert.Loosely(t, m.LaunchTask(c, ctl), should.BeNil)
			assert.Loosely(t, loadNoError(), should.Resemble(strmap{
				"refs/heads/master":       "deadbeef00",
				"refs/branch-heads/1.2.3": "baadcafe00",
			}))
			assert.Loosely(t, ctl.Triggers, should.HaveLength(4))
			// Ordered by ref, then by timestamp.
			assert.Loosely(t, ctl.Triggers[0].Id, should.Equal("https://a.googlesource.com/b.git/+/refs/branch-heads/1.2.3@baadcafe00"))
			assert.Loosely(t, ctl.Triggers[1].Id, should.Equal("https://a.googlesource.com/b.git/+/refs/heads/master@deadbeef02"))
			assert.Loosely(t, ctl.Triggers[2].Id, should.Equal("https://a.googlesource.com/b.git/+/refs/heads/master@deadbeef01"))
			assert.Loosely(t, ctl.Triggers[3].Id, should.Equal("https://a.googlesource.com/b.git/+/refs/heads/master@deadbeef00"))
			for i, trig := range ctl.Triggers {
				assert.Loosely(t, trig.OrderInBatch, should.Equal(i))
			}
			assert.Loosely(t, ctl.Triggers[0].Created.AsTime(), should.Match(epoch.Add(-1*time.Minute)))
			assert.Loosely(t, ctl.Triggers[1].Created.AsTime(), should.Match(epoch.Add(-3*time.Minute))) // oldest on master
			assert.Loosely(t, ctl.Triggers[2].Created.AsTime(), should.Match(epoch.Add(-2*time.Minute)))
			assert.Loosely(t, ctl.Triggers[3].Created.AsTime(), should.Match(epoch.Add(-1*time.Minute))) // newest on master
		})

		t.Run("Updated ref with pathfilters", func(t *ftt.Test) {
			cfg.Refs = []string{"refs/heads/master"}
			cfg.PathRegexps = []string{`.+\.emit`}
			cfg.PathRegexpsExclude = []string{`skip/.+`}
			assert.Loosely(t, saveState(c, jobID, cfg, strmap{"refs/heads/master": "deadbeef04"}), should.BeNil)
			expectRefs("refs/heads", strmap{"refs/heads/master": "deadbeef00"})
			expectLogWithDiff("deadbeef00", "deadbeef04", 50, "b",
				"deadbeef00:skip/commit",
				"deadbeef01:yup.emit",
				"deadbeef02:skip/this-file,not-matched-file,but-still.emit",
				"deadbeef03:nothing-matched-means-skipped")

			assert.Loosely(t, m.LaunchTask(c, ctl), should.BeNil)
			assert.Loosely(t, loadNoError(), should.Resemble(strmap{
				"refs/heads/master": "deadbeef00",
			}))
			assert.Loosely(t, ctl.Triggers, should.HaveLength(2))
			assert.Loosely(t, ctl.Triggers[0].Id, should.Equal("https://a.googlesource.com/b.git/+/refs/heads/master@deadbeef02"))
			assert.Loosely(t, ctl.Triggers[1].Id, should.Equal("https://a.googlesource.com/b.git/+/refs/heads/master@deadbeef01"))
		})

		t.Run("Updated ref without matched commits", func(t *ftt.Test) {
			cfg.Refs = []string{"refs/heads/master"}
			cfg.PathRegexps = []string{`must-match`}
			assert.Loosely(t, saveState(c, jobID, cfg, strmap{"refs/heads/master": "deadbeef04"}), should.BeNil)
			expectRefs("refs/heads", strmap{"refs/heads/master": "deadbeef00"})

			expectLogWithDiff("deadbeef00", "deadbeef04", 50, "b",
				"deadbeef00:nope0",
				"deadbeef01:nope1",
				"deadbeef02:nope2",
				"deadbeef03:nope3")
			assert.Loosely(t, m.LaunchTask(c, ctl), should.BeNil)
			assert.Loosely(t, loadNoError(), should.Resemble(strmap{
				"refs/heads/master": "deadbeef00",
			}))
			assert.Loosely(t, ctl.Triggers, should.HaveLength(0))
		})

		t.Run("do nothing at all if there are no changes", func(t *ftt.Test) {
			cfg.Refs = []string{"refs/heads/master"}
			assert.Loosely(t, saveState(c, jobID, cfg, strmap{
				"refs/heads/master": "deadbeef",
			}), should.BeNil)
			expectRefs("refs/heads", strmap{
				"refs/heads/master": "deadbeef",
			})
			assert.Loosely(t, m.LaunchTask(c, ctl), should.BeNil)
			assert.Loosely(t, ctl.Triggers, should.BeNil)
			assert.Loosely(t, ctl.Log, should.NotContain("Saved 1 known refs"))
			assert.Loosely(t, ctl.Log, should.Contain("No changes detected"))
			assert.Loosely(t, loadNoError(), should.Resemble(strmap{
				"refs/heads/master": "deadbeef",
			}))
		})

		t.Run("skip pre-existing refs when ref patterns change", func(t *ftt.Test) {
			alreadyProcessedRef := "refs/tags/foobar"
			newlyMatchedRef := "refs/tags/baz"
			newRef := "refs/tags/foobaz"

			cfg.Refs = []string{"regexp:refs/tags/foo.*"}
			assert.Loosely(t, saveState(c, jobID, cfg, strmap{
				alreadyProcessedRef: "deadbeef",
			}), should.BeNil)

			// Change the filter so it matches a different set of tags than what
			// was previously matched.
			cfg.Refs = []string{"regexp:refs/tags/.*"}

			expectRefs("refs/tags", strmap{
				alreadyProcessedRef: "deadbeef",
				// A newly appearing ref that doesn't match the previous filter,
				// so should be assumed to not actually be new.
				newlyMatchedRef: "def456",
				// A ref that also matches the previous filter but wasn't seen
				// before, so can be assumed to actually be new.
				newRef: "abc123",
			})
			expectLog("abc123", "", 1, log("abc123"))
			expectLog("def456", "", 1, log("def456"))

			m.maxCommitsPerRefUpdate = 1
			assert.Loosely(t, m.LaunchTask(c, ctl), should.BeNil)
			assert.Loosely(t, loadNoError(), should.Resemble(strmap{
				alreadyProcessedRef: "deadbeef",
				newRef:              "abc123",
				newlyMatchedRef:     "def456",
			}))
			assert.Loosely(t, ctl.Triggers, should.HaveLength(1))
			assert.Loosely(t, ctl.Triggers[0].Id, should.Equal("https://a.googlesource.com/b.git/+/refs/tags/foobaz@abc123"))
			assert.Loosely(t, ctl.Triggers[0].GetGitiles(), should.Resemble(&api.GitilesTrigger{
				Repo:     "https://a.googlesource.com/b.git",
				Ref:      newRef,
				Revision: "abc123",
			}))
		})

		t.Run("skip pre-existing refs for new trigger", func(t *ftt.Test) {
			ref := "refs/tags/foobaz"

			cfg.Refs = []string{"regexp:refs/tags/.*"}

			expectRefs("refs/tags", strmap{
				ref: "abc123",
			})
			expectLog("abc123", "", 1, log("abc123"))

			m.maxCommitsPerRefUpdate = 1
			assert.Loosely(t, m.LaunchTask(c, ctl), should.BeNil)
			assert.Loosely(t, loadNoError(), should.Resemble(strmap{
				ref: "abc123",
			}))
			assert.Loosely(t, ctl.Triggers, should.HaveLength(0))
		})

		t.Run("Avoid choking on too many refs", func(t *ftt.Test) {
			cfg.Refs = []string{"refs/heads/master", "regexp:refs/branch-heads/[^/]+"}
			assert.Loosely(t, saveState(c, jobID, cfg, strmap{
				"refs/heads/master": "deadbeef",
			}), should.BeNil)
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
			assert.Loosely(t, m.LaunchTask(c, ctl), should.BeNil)
			assert.Loosely(t, ctl.Triggers, should.HaveLength(2))
			assert.Loosely(t, loadNoError(), should.Resemble(strmap{
				"refs/heads/master":   "deadbeef",
				"refs/branch-heads/1": "cafee1",
				"refs/branch-heads/2": "cafee2",
			}))
			ctl.Triggers = nil

			// Second run, refs/branch-heads/{3,4} updated.
			assert.Loosely(t, m.LaunchTask(c, ctl), should.BeNil)
			assert.Loosely(t, ctl.Triggers, should.HaveLength(2))
			assert.Loosely(t, loadNoError(), should.Resemble(strmap{
				"refs/heads/master":   "deadbeef",
				"refs/branch-heads/1": "cafee1",
				"refs/branch-heads/2": "cafee2",
				"refs/branch-heads/3": "cafee3",
				"refs/branch-heads/4": "cafee4",
			}))
			ctl.Triggers = nil

			// Final run, refs/branch-heads/5 updated.
			assert.Loosely(t, m.LaunchTask(c, ctl), should.BeNil)
			assert.Loosely(t, ctl.Triggers, should.HaveLength(1))
			assert.Loosely(t, loadNoError(), should.Resemble(strmap{
				"refs/heads/master":   "deadbeef",
				"refs/branch-heads/1": "cafee1",
				"refs/branch-heads/2": "cafee2",
				"refs/branch-heads/3": "cafee3",
				"refs/branch-heads/4": "cafee4",
				"refs/branch-heads/5": "cafee5",
			}))
		})

		t.Run("Ensure progress", func(t *ftt.Test) {
			cfg.Refs = []string{"regexp:refs/branch-heads/[^/]+"}
			expectRefs("refs/branch-heads", strmap{
				"refs/branch-heads/1": "cafee1",
				"refs/branch-heads/2": "cafee2",
				"refs/branch-heads/3": "cafee3",
			}).AnyTimes()
			m.maxTriggersPerInvocation = 2
			m.maxCommitsPerRefUpdate = 1

			t.Run("no progress is an error", func(t *ftt.Test) {
				expectLog("cafee1", "", 1, log(), errors.New("flake"))
				assert.Loosely(t, m.LaunchTask(c, ctl), should.ErrLike("flake"))
			})

			expectLog("cafee1", "", 1, log("cafee1"))
			expectLog("cafee2", "", 1, log(), errors.New("flake"))
			assert.Loosely(t, m.LaunchTask(c, ctl), should.BeNil)
			assert.Loosely(t, ctl.Triggers, should.HaveLength(1))
			assert.Loosely(t, loadNoError(), should.Resemble(strmap{
				"refs/branch-heads/1": "cafee1",
			}))
			ctl.Triggers = nil
			assert.Loosely(t, loadNoError(), should.Resemble(strmap{
				"refs/branch-heads/1": "cafee1",
			}))

			// Second run.
			expectLog("cafee2", "", 1, log("cafee2"))
			expectLog("cafee3", "", 1, log("cafee3"))
			assert.Loosely(t, m.LaunchTask(c, ctl), should.BeNil)
			assert.Loosely(t, ctl.Triggers, should.HaveLength(2))
			assert.Loosely(t, loadNoError(), should.Resemble(strmap{
				"refs/branch-heads/1": "cafee1",
				"refs/branch-heads/2": "cafee2",
				"refs/branch-heads/3": "cafee3",
			}))
		})

		t.Run("distinguish force push from transient weirdness", func(t *ftt.Test) {
			cfg.Refs = []string{"refs/heads/master"}
			assert.Loosely(t, saveState(c, jobID, cfg, strmap{
				"refs/heads/master": "001d", // old.
			}), should.BeNil)

			t.Run("force push going backwards", func(t *ftt.Test) {
				expectRefs("refs/heads", strmap{"refs/heads/master": "1111"})
				expectLog("1111", "001d", 50, log())
				assert.Loosely(t, m.LaunchTask(c, ctl), should.BeNil)
				// Changes state
				assert.Loosely(t, loadNoError(), should.Resemble(strmap{
					"refs/heads/master": "1111",
				}))
				// .. but no triggers, since there are no new commits.
				assert.Loosely(t, ctl.Triggers, should.HaveLength(0))
			})

			t.Run("force push wiping out prior HEAD", func(t *ftt.Test) {
				expectRefs("refs/heads", strmap{"refs/heads/master": "1111"})
				expectLog("1111", "001d", 50, nil, status.Errorf(codes.NotFound, "not found"))
				expectLog("1111", "", 1, log("1111"))
				expectLog("001d", "", 1, nil, status.Errorf(codes.NotFound, "not found"))
				assert.Loosely(t, m.LaunchTask(c, ctl), should.BeNil)
				assert.Loosely(t, loadNoError(), should.Resemble(strmap{
					"refs/heads/master": "1111",
				}))
				assert.Loosely(t, ctl.Triggers, should.HaveLength(1))
			})

			t.Run("race 1", func(t *ftt.Test) {
				expectRefs("refs/heads", strmap{"refs/heads/master": "1111"})
				expectLog("1111", "001d", 50, nil, status.Errorf(codes.NotFound, "not found"))
				expectLog("1111", "", 1, nil, status.Errorf(codes.NotFound, "not found"))
				assert.Loosely(t, transient.Tag.In(m.LaunchTask(c, ctl)), should.BeTrue)
				assert.Loosely(t, loadNoError(), should.Resemble(strmap{
					"refs/heads/master": "001d", // no change.
				}))
			})

			t.Run("race or fluke", func(t *ftt.Test) {
				expectRefs("refs/heads", strmap{"refs/heads/master": "1111"})
				expectLog("1111", "001d", 50, nil, status.Errorf(codes.NotFound, "not found"))
				expectLog("1111", "", 1, nil, status.Errorf(codes.NotFound, "not found"))
				assert.Loosely(t, m.LaunchTask(c, ctl), should.NotBeNil)
				assert.Loosely(t, loadNoError(), should.Resemble(strmap{
					"refs/heads/master": "001d",
				}))
			})
		})
	})
}

func TestPathFilterHelpers(t *testing.T) {
	t.Parallel()

	ftt.Run("PathFilter helpers work", t, func(t *ftt.Test) {
		t.Run("disjunctiveOfRegexps works", func(t *ftt.Test) {
			assert.Loosely(t, disjunctiveOfRegexps([]string{`.+\.cpp`}), should.Equal(`^((.+\.cpp))$`))
			assert.Loosely(t, disjunctiveOfRegexps([]string{`.+\.cpp`, `?a`}), should.Equal(`^((.+\.cpp)|(?a))$`))
		})
		t.Run("pathFilter works", func(t *ftt.Test) {
			t.Run("simple", func(t *ftt.Test) {
				empty, err := newPathFilter(&messages.GitilesTask{})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, empty.active(), should.BeFalse)
				_, err = newPathFilter(&messages.GitilesTask{PathRegexps: []string{`\K`}})
				assert.Loosely(t, err, should.NotBeNil)
				_, err = newPathFilter(&messages.GitilesTask{PathRegexps: []string{`a?`}, PathRegexpsExclude: []string{`\K`}})
				assert.Loosely(t, err, should.NotBeNil)

			})
			t.Run("just negative ignored", func(t *ftt.Test) {
				v, err := newPathFilter(&messages.GitilesTask{PathRegexpsExclude: []string{`.+\.cpp`}})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, v.active(), should.BeFalse)
			})

			t.Run("just positive", func(t *ftt.Test) {
				v, err := newPathFilter(&messages.GitilesTask{PathRegexps: []string{`.+`}})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, v.active(), should.BeTrue)
				t.Run("empty commit is not interesting", func(t *ftt.Test) {
					assert.Loosely(t, v.isInteresting([]*git.Commit_TreeDiff{}), should.BeFalse)
				})
				t.Run("new or old paths are taken into account", func(t *ftt.Test) {
					assert.Loosely(t, v.isInteresting([]*git.Commit_TreeDiff{{OldPath: "old"}}), should.BeTrue)
					assert.Loosely(t, v.isInteresting([]*git.Commit_TreeDiff{{NewPath: "new"}}), should.BeTrue)
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

			t.Run("many positives", func(t *ftt.Test) {
				v, err := newPathFilter(&messages.GitilesTask{PathRegexps: []string{`.+\.cpp`, "exact"}})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, v.isInteresting(genDiff("not.matched")), should.BeFalse)

				assert.Loosely(t, v.isInteresting(genDiff("matched.cpp")), should.BeTrue)
				assert.Loosely(t, v.isInteresting(genDiff("exact")), should.BeTrue)
				assert.Loosely(t, v.isInteresting(genDiff("at least", "one", "matched.cpp")), should.BeTrue)
			})

			t.Run("many negatives", func(t *ftt.Test) {
				v, err := newPathFilter(&messages.GitilesTask{
					PathRegexps:        []string{`.+`},
					PathRegexpsExclude: []string{`.+\.cpp`, `excluded`},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, v.isInteresting(genDiff("not excluded")), should.BeTrue)
				assert.Loosely(t, v.isInteresting(genDiff("excluded/is/a/dir/not/a/file")), should.BeTrue)
				assert.Loosely(t, v.isInteresting(genDiff("excluded", "also.excluded.cpp", "but this file isn't")), should.BeTrue)

				assert.Loosely(t, v.isInteresting(genDiff("excluded.cpp")), should.BeFalse)
				assert.Loosely(t, v.isInteresting(genDiff("excluded")), should.BeFalse)
				assert.Loosely(t, v.isInteresting(genDiff()), should.BeFalse)
			})

			t.Run("smoke test for complexity", func(t *ftt.Test) {
				v, err := newPathFilter(&messages.GitilesTask{
					PathRegexps:        []string{`.+/\d\.py`, `included/.+`},
					PathRegexpsExclude: []string{`.+\.cpp`, `excluded/.*`},
				})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, v.isInteresting(genDiff("excluded/1", "also.cpp", "included/one-is-enough")), should.BeTrue)
				assert.Loosely(t, v.isInteresting(genDiff("included/but-also-excluded.cpp", "one-still-enough/1.py")), should.BeTrue)

				assert.Loosely(t, v.isInteresting(genDiff("included/but-also-excluded.cpp", "excluded/2.py")), should.BeFalse)
				assert.Loosely(t, v.isInteresting(genDiff("matches nothing", "")), should.BeFalse)
			})
		})
	})
}

func TestValidateConfig(t *testing.T) {
	t.Parallel()
	c := context.Background()

	ftt.Run("ValidateProtoMessage works", t, func(t *ftt.Test) {
		ctx := &validation.Context{Context: c}
		m := TaskManager{}
		validate := func(msg proto.Message) error {
			m.ValidateProtoMessage(ctx, msg, "some-project:some-realm")
			return ctx.Finalize()
		}
		t.Run("refNamespace works", func(t *ftt.Test) {
			cfg := &messages.GitilesTask{
				Repo: "https://a.googlesource.com/b.git",
				Refs: []string{"refs/heads/master", "refs/heads/branch", "regexp:refs/branch-heads/[^/]+"},
			}
			t.Run("proper refs", func(t *ftt.Test) {
				assert.Loosely(t, validate(cfg), should.BeNil)
			})
			t.Run("invalid ref", func(t *ftt.Test) {
				cfg.Refs = []string{"wtf/not/a/ref"}
				assert.Loosely(t, validate(cfg), should.NotBeNil)
			})
		})

		t.Run("refRegexp works", func(t *ftt.Test) {
			cfg := &messages.GitilesTask{
				Repo: "https://a.googlesource.com/b.git",
				Refs: []string{
					`regexp:refs/heads/\d+`,
					`regexp:refs/actually/exact`,
					`refs/heads/master`,
				},
			}
			t.Run("valid", func(t *ftt.Test) {
				assert.Loosely(t, validate(cfg), should.BeNil)
			})
			t.Run("invalid regexp", func(t *ftt.Test) {
				cfg.Refs = []string{`regexp:a++`}
				assert.Loosely(t, validate(cfg), should.NotBeNil)
			})
		})

		t.Run("pathRegexs works", func(t *ftt.Test) {
			cfg := &messages.GitilesTask{
				Repo:               "https://a.googlesource.com/b.git",
				Refs:               []string{"refs/heads/master"},
				PathRegexps:        []string{`.+\.cpp`},
				PathRegexpsExclude: []string{`.+\.py`},
			}
			t.Run("valid", func(t *ftt.Test) {
				assert.Loosely(t, validate(cfg), should.BeNil)
			})
			t.Run("can't even parse", func(t *ftt.Test) {
				cfg.PathRegexpsExclude = []string{`\K`}
				assert.Loosely(t, validate(cfg), should.NotBeNil)
			})
			t.Run("redundant", func(t *ftt.Test) {
				cfg.PathRegexps = []string{``}
				assert.Loosely(t, validate(cfg), should.NotBeNil)
				cfg.PathRegexps = []string{`^file`}
				assert.Loosely(t, validate(cfg), should.NotBeNil)
				cfg.PathRegexps = []string{`file$`}
				assert.Loosely(t, validate(cfg), should.NotBeNil)
			})
			t.Run("excludes require includes", func(t *ftt.Test) {
				cfg.PathRegexps = nil
				assert.Loosely(t, validate(cfg), should.NotBeNil)
			})
		})
	})
}
