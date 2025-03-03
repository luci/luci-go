// Copyright 2020 The LUCI Authors.
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

package runquery

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
)

func TestProjectQueryBuilder(t *testing.T) {
	t.Parallel()

	ftt.Run("ProjectQueryBuilder works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		getAll := func(t testing.TB, qb ProjectQueryBuilder) common.RunIDs {
			t.Helper()
			return execQueryInTestSameRunsAndKeys(ctx, t, qb)
		}

		makeRun := func(t testing.TB, proj string, delay time.Duration, s run.Status) common.RunID {
			t.Helper()
			createdAt := ct.Clock.Now().Add(delay)
			runID := common.MakeRunID(proj, createdAt, 1, []byte{0, byte(delay / time.Millisecond)})
			assert.Loosely(t, datastore.Put(ctx, &run.Run{ID: runID, Status: s}), should.BeNil, truth.LineContext())
			return runID
		}

		// RunID below are ordered lexicographically.
		bond9 := makeRun(t, "bond", 9*time.Millisecond, run.Status_RUNNING)
		bond4 := makeRun(t, "bond", 4*time.Millisecond, run.Status_FAILED)
		bond2 := makeRun(t, "bond", 2*time.Millisecond, run.Status_CANCELLED)
		xero7 := makeRun(t, "xero", 7*time.Millisecond, run.Status_RUNNING)
		xero6 := makeRun(t, "xero", 6*time.Millisecond, run.Status_RUNNING)
		xero5 := makeRun(t, "xero", 5*time.Millisecond, run.Status_SUCCEEDED)

		t.Run("Project without Runs", func(t *ftt.Test) {
			qb := ProjectQueryBuilder{Project: "missing"}
			assert.Loosely(t, getAll(t, qb), should.BeEmpty)
		})

		t.Run("Project with some Runs", func(t *ftt.Test) {
			qb := ProjectQueryBuilder{Project: "bond"}
			assert.That(t, getAll(t, qb), should.Match(common.RunIDs{bond9, bond4, bond2}))
		})

		t.Run("Obeys limit and returns correct page token", func(t *ftt.Test) {
			qb := ProjectQueryBuilder{Project: "bond", Limit: 2}
			runs1, pageToken1, err := qb.LoadRuns(ctx)
			assert.NoErr(t, err)
			assert.That(t, idsOf(runs1), should.Match(common.RunIDs{bond9, bond4}))
			assert.Loosely(t, pageToken1, should.NotBeNil)

			qb = qb.PageToken(pageToken1)
			assert.That(t, qb.MinExcl, should.Match(bond4))
			runs2, pageToken2, err := qb.LoadRuns(ctx)
			assert.NoErr(t, err)
			assert.That(t, idsOf(runs2), should.Match(common.RunIDs{bond2}))
			assert.Loosely(t, pageToken2, should.BeNil)
		})

		t.Run("Filters by Status", func(t *ftt.Test) {
			t.Run("Simple", func(t *ftt.Test) {
				qb := ProjectQueryBuilder{Project: "xero", Status: run.Status_RUNNING}
				assert.That(t, getAll(t, qb), should.Match(common.RunIDs{xero7, xero6}))

				qb = ProjectQueryBuilder{Project: "xero", Status: run.Status_SUCCEEDED}
				assert.That(t, getAll(t, qb), should.Match(common.RunIDs{xero5}))
			})
			t.Run("Status_ENDED_MASK", func(t *ftt.Test) {
				qb := ProjectQueryBuilder{Project: "bond", Status: run.Status_ENDED_MASK}
				assert.That(t, getAll(t, qb), should.Match(common.RunIDs{bond4, bond2}))

				t.Run("Obeys limit", func(t *ftt.Test) {
					qb.Limit = 1
					assert.That(t, getAll(t, qb), should.Match(common.RunIDs{bond4}))
				})
			})
		})

		t.Run("Min", func(t *ftt.Test) {
			qb := ProjectQueryBuilder{Project: "bond", MinExcl: bond9}
			assert.That(t, getAll(t, qb), should.Match(common.RunIDs{bond4, bond2}))

			t.Run("same as Before", func(t *ftt.Test) {
				qb2 := ProjectQueryBuilder{}.Before(bond9)
				assert.That(t, qb, should.Match(qb2))
			})
		})

		t.Run("Max", func(t *ftt.Test) {
			qb := ProjectQueryBuilder{Project: "bond", MaxExcl: bond2}
			assert.That(t, getAll(t, qb), should.Match(common.RunIDs{bond9, bond4}))

			t.Run("same as After", func(t *ftt.Test) {
				qb2 := ProjectQueryBuilder{}.After(bond2)
				assert.That(t, qb, should.Match(qb2))
			})
		})

		t.Run("After .. Before", func(t *ftt.Test) {
			t.Run("Some", func(t *ftt.Test) {
				qb := ProjectQueryBuilder{}.After(bond2).Before(bond9)
				assert.That(t, getAll(t, qb), should.Match(common.RunIDs{bond4}))
			})

			t.Run("Empty", func(t *ftt.Test) {
				qb := ProjectQueryBuilder{Project: "bond"}.After(bond4).Before(bond9)
				assert.Loosely(t, getAll(t, qb), should.HaveLength(0))
			})

			t.Run("Overconstrained", func(t *ftt.Test) {
				qb := ProjectQueryBuilder{Project: "bond"}.After(bond9).Before(bond2)
				_, err := qb.BuildKeysOnly(ctx).Finalize()
				assert.Loosely(t, err, should.Equal(datastore.ErrNullQuery))
			})

			t.Run("With status", func(t *ftt.Test) {
				qb := ProjectQueryBuilder{Status: run.Status_FAILED}.After(bond2).Before(bond9)
				assert.That(t, getAll(t, qb), should.Match(common.RunIDs{bond4}))
				qb = ProjectQueryBuilder{Status: run.Status_SUCCEEDED}.After(bond2).Before(bond9)
				assert.Loosely(t, getAll(t, qb), should.HaveLength(0))
			})

		})

		t.Run("Invalid usage panics", func(t *ftt.Test) {
			assert.Loosely(t, func() { ProjectQueryBuilder{}.BuildKeysOnly(ctx) }, should.Panic)
			assert.Loosely(t, func() { ProjectQueryBuilder{Project: "not-bond", MinExcl: bond4}.BuildKeysOnly(ctx) }, should.Panic)
			assert.Loosely(t, func() { ProjectQueryBuilder{Project: "not-bond", MaxExcl: bond4}.BuildKeysOnly(ctx) }, should.Panic)
			assert.Loosely(t, func() { ProjectQueryBuilder{Project: "not-bond"}.Before(bond4) }, should.Panic)
			assert.Loosely(t, func() { ProjectQueryBuilder{Project: "not-bond"}.After(bond4) }, should.Panic)
			assert.Loosely(t, func() { ProjectQueryBuilder{}.After(bond4).Before(xero7) }, should.Panic)
		})
	})
}
