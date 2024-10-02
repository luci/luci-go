// Copyright 2021 The LUCI Authors.
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

package run

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp/cmpopts"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "go.chromium.org/luci/common/testing/assertions"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestLoadChildRuns(t *testing.T) {
	t.Parallel()

	ftt.Run("LoadChildRuns works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		put := func(runID common.RunID, depRuns common.RunIDs) {
			assert.Loosely(t, datastore.Put(ctx, &Run{
				ID:      runID,
				DepRuns: depRuns,
			}), should.BeNil)
		}

		const parentRun1 = common.RunID("parent/1-cow")

		const orphanRun = common.RunID("orphan/1-chicken")
		put(orphanRun, common.RunIDs{})
		out1, err := LoadChildRuns(ctx, parentRun1)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, out1, should.HaveLength(0))

		const pendingRun = common.RunID("child/1-pending")
		put(pendingRun, common.RunIDs{parentRun1})
		const runningRun = common.RunID("child/1-running")
		put(runningRun, common.RunIDs{parentRun1})

		out2, err := LoadChildRuns(ctx, parentRun1)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, out2, should.HaveLength(2))
	})
}

func TestLoadRunLogEntries(t *testing.T) {
	t.Parallel()

	ftt.Run("LoadRunLogEntries works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		ev := int64(1)
		put := func(runID common.RunID, entries ...*LogEntry) {
			assert.Loosely(t, datastore.Put(ctx, &RunLog{
				Run:     datastore.MakeKey(ctx, common.RunKind, string(runID)),
				ID:      ev,
				Entries: &LogEntries{Entries: entries},
			}), should.BeNil)
			ev += 1
		}

		const run1 = common.RunID("rust/123-1-beef")
		const run2 = common.RunID("dart/789-2-cafe")

		put(
			run1,
			&LogEntry{
				Time: timestamppb.New(ct.Clock.Now()),
				Kind: &LogEntry_Created_{Created: &LogEntry_Created{
					ConfigGroupId: "fi/rst",
				}},
			},
		)
		ct.Clock.Add(time.Minute)
		put(
			run1,
			&LogEntry{
				Time: timestamppb.New(ct.Clock.Now()),
				Kind: &LogEntry_ConfigChanged_{ConfigChanged: &LogEntry_ConfigChanged{
					ConfigGroupId: "se/cond",
				}},
			},
			&LogEntry{
				Time: timestamppb.New(ct.Clock.Now()),
				Kind: &LogEntry_TryjobsRequirementUpdated_{TryjobsRequirementUpdated: &LogEntry_TryjobsRequirementUpdated{}},
			},
		)

		ct.Clock.Add(time.Minute)
		put(
			run2,
			&LogEntry{
				Time: timestamppb.New(ct.Clock.Now()),
				Kind: &LogEntry_Created_{Created: &LogEntry_Created{
					ConfigGroupId: "fi/rst-but-run2",
				}},
			},
		)

		out1, err := LoadRunLogEntries(ctx, run1)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, out1, should.HaveLength(3))
		assert.Loosely(t, out1[0].GetCreated().GetConfigGroupId(), should.Match("fi/rst"))
		assert.Loosely(t, out1[1].GetConfigChanged().GetConfigGroupId(), should.Match("se/cond"))
		assert.Loosely(t, out1[2].GetTryjobsRequirementUpdated(), should.NotBeNil)

		out2, err := LoadRunLogEntries(ctx, run2)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, out2, should.HaveLength(1))
		assert.Loosely(t, out2[0].GetCreated().GetConfigGroupId(), should.Match("fi/rst-but-run2"))
	})
}

func TestLoadRunsBuilder(t *testing.T) {
	t.Parallel()

	ftt.Run("LoadRunsBuilder works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const lProject = "proj"
		// Run statuses are used in this test to ensure Runs were actually loaded.
		makeRun := func(id int, s Status) *Run {
			r := &Run{ID: common.RunID(fmt.Sprintf("%s/%03d", lProject, id)), Status: s}
			assert.Loosely(t, datastore.Put(ctx, r), should.BeNil)
			return r
		}

		r1 := makeRun(1, Status_RUNNING)
		r2 := makeRun(2, Status_CANCELLED)
		r3 := makeRun(3, Status_PENDING)
		r4 := makeRun(4, Status_SUCCEEDED)
		r201 := makeRun(201, Status_FAILED)
		r202 := makeRun(202, Status_FAILED)
		r404 := makeRun(404, Status_PENDING)
		r405 := makeRun(405, Status_PENDING)
		assert.Loosely(t, datastore.Delete(ctx, r404, r405), should.BeNil)

		t.Run("Without checker", func(t *ftt.Test) {
			t.Run("Every Run exists", func(t *ftt.Test) {
				verify := func(b LoadRunsBuilder) {
					runsA, errs := b.Do(ctx)
					assert.Loosely(t, errs, should.Resemble(make(errors.MultiError, 2)))
					assert.Loosely(t, runsA, should.Resemble([]*Run{r201, r202}))

					runsB, err := b.DoIgnoreNotFound(ctx)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, runsB, should.Resemble(runsA))
				}
				t.Run("IDs", func(t *ftt.Test) {
					verify(LoadRunsFromIDs(r201.ID, r202.ID))
				})
				t.Run("keys", func(t *ftt.Test) {
					verify(LoadRunsFromKeys(
						datastore.MakeKey(ctx, common.RunKind, string(r201.ID)),
						datastore.MakeKey(ctx, common.RunKind, string(r202.ID)),
					))
				})
			})

			t.Run("A missing Run", func(t *ftt.Test) {
				b := LoadRunsFromIDs(r404.ID)

				runsA, errs := b.Do(ctx)
				assert.Loosely(t, errs, should.Match([]error{datastore.ErrNoSuchEntity}, cmpopts.EquateErrors()))
				assert.Loosely(t, runsA, should.Resemble([]*Run{{ID: r404.ID}}))

				runsB, err := b.DoIgnoreNotFound(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, runsB, should.BeNil)
			})
			t.Run("Mix of existing and missing", func(t *ftt.Test) {
				b := LoadRunsFromIDs(r201.ID, r404.ID, r202.ID, r405.ID, r4.ID)

				runsA, errs := b.Do(ctx)
				assert.Loosely(t, errs, should.Match([]error{nil, datastore.ErrNoSuchEntity, nil, datastore.ErrNoSuchEntity, nil}, cmpopts.EquateErrors()))
				assert.Loosely(t, runsA, should.Resemble([]*Run{
					r201,
					{ID: r404.ID},
					r202,
					{ID: r405.ID},
					r4,
				}))

				runsB, err := b.DoIgnoreNotFound(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, runsB, should.Resemble([]*Run{r201, r202, r4}))
			})
		})

		t.Run("With checker", func(t *ftt.Test) {
			checker := fakeRunChecker{
				afterOnNotFound: appstatus.Error(codes.NotFound, "not-found-ds"),
			}

			t.Run("No errors of any kind", func(t *ftt.Test) {
				b := LoadRunsFromIDs(r201.ID, r202.ID, r4.ID).Checker(checker)

				runsA, errs := b.Do(ctx)
				assert.Loosely(t, errs, should.Resemble(make(errors.MultiError, 3)))
				assert.Loosely(t, runsA, should.Resemble([]*Run{r201, r202, r4}))

				runsB, err := b.DoIgnoreNotFound(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, runsB, should.Resemble(runsA))
			})

			t.Run("Missing in datastore", func(t *ftt.Test) {
				b := LoadRunsFromIDs(r404.ID).Checker(checker)

				runsA, errs := b.Do(ctx)
				assert.Loosely(t, errs[0], convey.Adapt(ShouldHaveAppStatus)(codes.NotFound))
				assert.Loosely(t, runsA, should.Resemble([]*Run{{ID: r404.ID}}))

				runsB, err := b.DoIgnoreNotFound(ctx)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, runsB, should.BeNil)
			})

			t.Run("Mix", func(t *ftt.Test) {
				checker.before = map[common.RunID]error{
					r1.ID: appstatus.Error(codes.NotFound, "not-found-before"),
					r2.ID: errors.New("before-oops"),
				}
				checker.after = map[common.RunID]error{
					r3.ID: appstatus.Error(codes.NotFound, "not-found-after"),
					r4.ID: errors.New("after-oops"),
				}
				t.Run("only found and not found", func(t *ftt.Test) {
					b := LoadRunsFromIDs(r201.ID, r1.ID, r202.ID, r3.ID, r404.ID).Checker(checker)

					runsA, errs := b.Do(ctx)
					assert.Loosely(t, errs[0], should.BeNil) // r201
					assert.Loosely(t, errs[1], should.ErrLike("not-found-before"))
					assert.Loosely(t, errs[2], should.BeNil) // r202
					assert.Loosely(t, errs[3], should.ErrLike("not-found-after"))
					assert.Loosely(t, errs[4], should.ErrLike("not-found-ds"))
					assert.Loosely(t, runsA, should.Resemble([]*Run{
						r201,
						{ID: r1.ID},
						r202,
						r3, // loaded & returned, despite errors
						{ID: r404.ID},
					}))

					runsB, err := b.DoIgnoreNotFound(ctx)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, runsB, should.Resemble([]*Run{r201, r202}))
				})
				t.Run("of everything", func(t *ftt.Test) {
					b := LoadRunsFromIDs(r201.ID, r1.ID, r2.ID, r3.ID, r4.ID, r404.ID).Checker(checker)

					runsA, errs := b.Do(ctx)
					assert.Loosely(t, errs[0], should.BeNil) // r201
					assert.Loosely(t, errs[1], should.ErrLike("not-found-before"))
					assert.Loosely(t, errs[2], should.ErrLike("before-oops"))
					assert.Loosely(t, errs[3], should.ErrLike("not-found-after"))
					assert.Loosely(t, errs[4], should.ErrLike("after-oops"))
					assert.Loosely(t, errs[5], should.ErrLike("not-found-ds"))
					assert.Loosely(t, runsA, should.Resemble([]*Run{
						r201,
						{ID: r1.ID},
						{ID: r2.ID},
						r3, // loaded & returned, despite errors
						r4, // loaded & returned, despite errors
						{ID: r404.ID},
					}))

					runsB, err := b.DoIgnoreNotFound(ctx)
					assert.Loosely(t, err, should.ErrLike("before-oops"))
					assert.Loosely(t, runsB, should.BeNil)
				})
			})
		})
	})
}

type fakeRunChecker struct {
	before          map[common.RunID]error
	beforeFunc      func(common.RunID) error // applied only if Run is not in `before`
	after           map[common.RunID]error
	afterOnNotFound error
}

func (f fakeRunChecker) Before(ctx context.Context, id common.RunID) error {
	err := f.before[id]
	if err == nil && f.beforeFunc != nil {
		err = f.beforeFunc(id)
	}
	return err
}

func (f fakeRunChecker) After(ctx context.Context, runIfFound *Run) error {
	if runIfFound == nil {
		return f.afterOnNotFound
	}
	return f.after[runIfFound.ID]
}
