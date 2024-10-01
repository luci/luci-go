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

func TestCLQueryBuilder(t *testing.T) {
	t.Parallel()

	ftt.Run("CLQueryBuilder works", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		// getAll asserts that LoadRuns returns Runs with the given RunIDs.
		getAll := func(t testing.TB, q CLQueryBuilder) common.RunIDs {
			t.Helper()

			keys, err := q.GetAllRunKeys(ctx)
			assert.Loosely(t, err, should.BeNil, truth.LineContext())
			// They keys may be different than the Runs because some Runs
			// may be filtered out (by isSatisfied).
			runs, pageToken, err := q.LoadRuns(ctx)
			assert.Loosely(t, err, should.BeNil, truth.LineContext())
			ids := idsOf(runs)
			assertCorrectPageToken(t, q, keys, pageToken)
			return ids
		}

		// makeRun puts a Run and returns the RunID.
		makeRun := func(t testing.TB, proj string, delay time.Duration, clids ...common.CLID) common.RunID {
			createdAt := ct.Clock.Now().Add(delay)
			runID := common.MakeRunID(proj, createdAt, 1, []byte{0, byte(delay / time.Millisecond)})
			assert.Loosely(t, datastore.Put(ctx, &run.Run{ID: runID, CLs: clids}), should.BeNil, truth.LineContext())
			for _, clid := range clids {
				assert.Loosely(t, datastore.Put(ctx, &run.RunCL{
					Run:       datastore.MakeKey(ctx, common.RunKind, string(runID)),
					ID:        clid,
					IndexedID: clid,
				}), should.BeNil, truth.LineContext())
			}
			return runID
		}

		clA, clB, clZ := common.CLID(1), common.CLID(2), common.CLID(3)

		// The below runs are sorted by RunID; by project then by time from
		// latest to earliest.
		bond9 := makeRun(t, "bond", 9*time.Millisecond, clA)
		bond4 := makeRun(t, "bond", 4*time.Millisecond, clA, clB)
		bond2 := makeRun(t, "bond", 2*time.Millisecond, clA)
		dart5 := makeRun(t, "dart", 5*time.Millisecond, clA)
		dart3 := makeRun(t, "dart", 3*time.Millisecond, clA)
		rust1 := makeRun(t, "rust", 1*time.Millisecond, clA, clB)
		xero7 := makeRun(t, "xero", 7*time.Millisecond, clA)

		t.Run("CL without Runs", func(t *ftt.Test) {
			qb := CLQueryBuilder{CLID: clZ}
			assert.Loosely(t, getAll(t, qb), should.Resemble(common.RunIDs(nil)))
		})

		t.Run("CL with some Runs", func(t *ftt.Test) {
			qb := CLQueryBuilder{CLID: clB}
			assert.Loosely(t, getAll(t, qb), should.Resemble(common.RunIDs{bond4, rust1}))
		})

		t.Run("CL with all Runs", func(t *ftt.Test) {
			qb := CLQueryBuilder{CLID: clA}
			assert.Loosely(t, getAll(t, qb), should.Resemble(common.RunIDs{bond9, bond4, bond2, dart5, dart3, rust1, xero7}))
		})

		t.Run("Two CLs, with some Runs", func(t *ftt.Test) {
			qb := CLQueryBuilder{CLID: clB, AdditionalCLIDs: common.MakeCLIDsSet(int64(clA))}
			assert.Loosely(t, getAll(t, qb), should.Resemble(common.RunIDs{bond4, rust1}))
		})

		t.Run("Two CLs with some Runs, other order", func(t *ftt.Test) {
			qb := CLQueryBuilder{CLID: clA, AdditionalCLIDs: common.MakeCLIDsSet(int64(clB))}
			assert.Loosely(t, getAll(t, qb), should.Resemble(common.RunIDs{bond4, rust1}))
		})

		t.Run("Two CLs, with no Runs", func(t *ftt.Test) {
			qb := CLQueryBuilder{CLID: clA, AdditionalCLIDs: common.MakeCLIDsSet(int64(clZ))}
			assert.Loosely(t, getAll(t, qb), should.BeEmpty)
		})

		t.Run("Filter by Project", func(t *ftt.Test) {
			qb := CLQueryBuilder{CLID: clA, Project: "bond"}
			assert.Loosely(t, getAll(t, qb), should.Resemble(common.RunIDs{bond9, bond4, bond2}))
		})

		t.Run("Filtering by Project and Min with diff project", func(t *ftt.Test) {
			qb := CLQueryBuilder{CLID: clA, Project: "dart", MinExcl: bond4}
			assert.Loosely(t, getAll(t, qb), should.Resemble(common.RunIDs{dart5, dart3}))

			qb = CLQueryBuilder{CLID: clA, Project: "dart", MinExcl: rust1}
			_, err := qb.BuildKeysOnly(ctx).Finalize()
			assert.Loosely(t, err, should.Equal(datastore.ErrNullQuery))
		})

		t.Run("Filtering by Project and Max with diff project", func(t *ftt.Test) {
			qb := CLQueryBuilder{CLID: clA, Project: "dart", MaxExcl: xero7}
			assert.Loosely(t, getAll(t, qb), should.Resemble(common.RunIDs{dart5, dart3}))

			qb = CLQueryBuilder{CLID: clA, Project: "dart", MaxExcl: bond4}
			_, err := qb.BuildKeysOnly(ctx).Finalize()
			assert.Loosely(t, err, should.Equal(datastore.ErrNullQuery))
		})

		t.Run("Before", func(t *ftt.Test) {
			qb := CLQueryBuilder{CLID: clA}.BeforeInProject(bond9)
			assert.Loosely(t, getAll(t, qb), should.Resemble(common.RunIDs{bond4, bond2}))

			qb = CLQueryBuilder{CLID: clA}.BeforeInProject(bond4)
			assert.Loosely(t, getAll(t, qb), should.Resemble(common.RunIDs{bond2}))

			qb = CLQueryBuilder{CLID: clA}.BeforeInProject(bond2)
			assert.Loosely(t, getAll(t, qb), should.Resemble(common.RunIDs(nil)))
		})

		t.Run("After", func(t *ftt.Test) {
			qb := CLQueryBuilder{CLID: clA}.AfterInProject(bond2)
			assert.Loosely(t, getAll(t, qb), should.Resemble(common.RunIDs{bond9, bond4}))

			qb = CLQueryBuilder{CLID: clA}.AfterInProject(bond4)
			assert.Loosely(t, getAll(t, qb), should.Resemble(common.RunIDs{bond9}))

			qb = CLQueryBuilder{CLID: clA}.AfterInProject(bond9)
			assert.Loosely(t, getAll(t, qb), should.Resemble(common.RunIDs(nil)))
		})

		t.Run("Obeys limit and returns correct page token", func(t *ftt.Test) {
			qb := CLQueryBuilder{CLID: clA, Limit: 1}.AfterInProject(bond2)
			runs1, pageToken1, err := qb.LoadRuns(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsOf(runs1), should.Resemble(common.RunIDs{bond9}))
			assert.Loosely(t, pageToken1, should.NotBeNil)

			qb = qb.PageToken(pageToken1)
			assert.Loosely(t, qb.MinExcl, should.Resemble(bond9))
			runs2, pageToken2, err := qb.LoadRuns(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, idsOf(runs2), should.Resemble(common.RunIDs{bond4}))
			assert.Loosely(t, pageToken2, should.NotBeNil)

			qb = qb.PageToken(pageToken2)
			assert.Loosely(t, qb.MinExcl, should.Resemble(bond4))
			runs3, pageToken3, err := qb.LoadRuns(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, runs3, should.BeEmpty)
			assert.Loosely(t, pageToken3, should.BeNil)
		})

		t.Run("After and Before", func(t *ftt.Test) {
			qb := CLQueryBuilder{CLID: clA}.AfterInProject(bond2).BeforeInProject(bond9)
			assert.Loosely(t, getAll(t, qb), should.Resemble(common.RunIDs{bond4}))
		})

		t.Run("Invalid usage panics", func(t *ftt.Test) {
			assert.Loosely(t, func() { CLQueryBuilder{CLID: clA, Project: "dart"}.BeforeInProject(bond2) }, should.Panic)
			assert.Loosely(t, func() { CLQueryBuilder{CLID: clA, Project: "dart"}.AfterInProject(bond2) }, should.Panic)
			assert.Loosely(t, func() { CLQueryBuilder{CLID: clA}.AfterInProject(dart3).BeforeInProject(xero7) }, should.Panic)
		})
	})
}
