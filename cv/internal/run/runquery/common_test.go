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
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
)

// TestLoadRunsFromQuery provides additional coverage to loadRunsFromQuery
// which isn't achieved in other tests in this file, where loadRunsFromQuery is
// tested indirectly as part of ...QueryBuilder.LoadRuns().
func TestLoadRunsFromQuery(t *testing.T) {
	t.Parallel()

	ftt.Run("loadRunsFromQuery", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		makeRun := func(proj string, delay time.Duration, clids ...common.CLID) common.RunID {
			createdAt := ct.Clock.Now().Add(delay)
			runID := common.MakeRunID(proj, createdAt, 1, []byte{0, byte(delay / time.Millisecond)})
			assert.Loosely(t, datastore.Put(ctx, &run.Run{ID: runID, CLs: clids}), should.BeNil)
			for _, clid := range clids {
				assert.Loosely(t, datastore.Put(ctx, &run.RunCL{
					Run:       datastore.MakeKey(ctx, common.RunKind, string(runID)),
					ID:        clid,
					IndexedID: clid,
				}), should.BeNil)
			}
			return runID
		}

		clA, clB, clZ := common.CLID(1), common.CLID(2), common.CLID(3)

		// RunID below are ordered lexicographically.
		bond9 := makeRun("bond", 9*time.Millisecond, clA)
		bond4 := makeRun("bond", 4*time.Millisecond, clA, clB)
		bond2 := makeRun("bond", 2*time.Millisecond, clA) // ignored by aclChecker
		dart5 := makeRun("dart", 5*time.Millisecond, clA)
		dart3 := makeRun("dart", 3*time.Millisecond, clA)      // ignored by aclChecker
		rust8 := makeRun("rust", 8*time.Millisecond, clA, clB) // ignored by aclChecker
		rust1 := makeRun("rust", 1*time.Millisecond, clA, clB)
		xero7 := makeRun("xero", 7*time.Millisecond, clA)

		errNotFound := appstatus.Error(codes.NotFound, "but really, no permission")
		aclChecker := &fakeRunChecker{
			before: map[common.RunID]error{
				bond2: errNotFound,
				rust8: errNotFound,
			},
			after: map[common.RunID]error{
				dart3: errNotFound,
			},
		}

		t.Run("If there is no limit, page token must not be returned but ACLs must be obeyed", func(t *ftt.Test) {
			q := CLQueryBuilder{CLID: clA}
			runs, pt, err := loadRunsFromQuery(ctx, q, aclChecker)
			assert.NoErr(t, err)
			assert.Loosely(t, idsOf(runs), should.Resemble(common.RunIDs{bond9, bond4, dart5, rust1, xero7}))
			assert.Loosely(t, pt, should.BeNil)
		})

		t.Run("Obeys and provides exactly the limit of Runs when available", func(t *ftt.Test) {
			t.Run("without ACLs filtering", func(t *ftt.Test) {
				q := CLQueryBuilder{CLID: clA, Limit: 3}
				runs, pt, err := loadRunsFromQuery(ctx, q)
				assert.NoErr(t, err)
				assert.Loosely(t, idsOf(runs), should.Resemble(common.RunIDs{bond9, bond4, bond2}))
				assert.Loosely(t, pt, should.NotBeNil)
			})

			t.Run("even with ACLs filtering", func(t *ftt.Test) {
				t.Run("limit=3", func(t *ftt.Test) {
					q := CLQueryBuilder{CLID: clA, Limit: 3}
					runs, pt, err := loadRunsFromQuery(ctx, q, aclChecker)
					assert.NoErr(t, err)
					assert.Loosely(t, idsOf(runs), should.Resemble(common.RunIDs{bond9, bond4, dart5}))
					t.Log("and chooses pagetoken to maximally avoid redundant work")
					// NOTE: the second behind-the-scenes query should have fetched
					// keys for {dart5,dart3,rust8}.
					assert.Loosely(t, pt.GetRun(), should.Resemble(string(rust8)))

					q = q.PageToken(pt)
					runs, pt, err = loadRunsFromQuery(ctx, q, aclChecker)
					assert.NoErr(t, err)
					assert.Loosely(t, idsOf(runs), should.Resemble(common.RunIDs{rust1, xero7}))
					assert.Loosely(t, pt, should.BeNil)
				})
				t.Run("limit=4", func(t *ftt.Test) {
					q := CLQueryBuilder{CLID: clA, Limit: 4}
					runs, pt, err := loadRunsFromQuery(ctx, q, aclChecker)
					assert.NoErr(t, err)
					assert.Loosely(t, idsOf(runs), should.Resemble(common.RunIDs{bond9, bond4, dart5, rust1}))
					t.Log("and chooses pagetoken to maximally avoid redundant work")
					// NOTE: the second behind-the-scenes query should have fetched
					// keys for {dart3,rust8,rust1,xero7} but xero7 didn't fit into
					// `runs`, thus next time it's sufficient to start with >rust1.
					assert.Loosely(t, pt.GetRun(), should.Resemble(string(rust1)))

					q = q.PageToken(pt)
					runs, pt, err = loadRunsFromQuery(ctx, q, aclChecker)
					assert.NoErr(t, err)
					assert.Loosely(t, idsOf(runs), should.Resemble(common.RunIDs{xero7}))
					assert.Loosely(t, pt, should.BeNil)
				})
				t.Run("limit=5", func(t *ftt.Test) {
					q := CLQueryBuilder{CLID: clA, Limit: 5}
					runs, pt, err := loadRunsFromQuery(ctx, q, aclChecker)
					assert.NoErr(t, err)
					assert.Loosely(t, idsOf(runs), should.Resemble(common.RunIDs{bond9, bond4, dart5, rust1, xero7}))
					// NOTE: the second behind-the-scenes query should have fetched
					// keys for {rust8,rust1,xero7}, and thus exhausted the search.
					assert.Loosely(t, pt, should.BeNil)
				})
			})
		})

		t.Run("If there are exactly `limit` Runs, page token may be returned", func(t *ftt.Test) {
			q := CLQueryBuilder{CLID: clB, Limit: 3}
			runs, pt, err := loadRunsFromQuery(ctx, q, aclChecker)
			assert.NoErr(t, err)
			assert.Loosely(t, idsOf(runs), should.Resemble(common.RunIDs{bond4, rust1}))
			if pt != nil {
				// If page token is returned, then next page must be empty.
				q = q.PageToken(pt)
				runs, pt, err = loadRunsFromQuery(ctx, q, aclChecker)
				assert.NoErr(t, err)
				assert.Loosely(t, runs, should.BeEmpty)
				assert.Loosely(t, pt, should.BeNil)
			}
		})

		t.Run("If there are no Runs, then limit is not obeyed", func(t *ftt.Test) {
			q := CLQueryBuilder{CLID: clZ, Limit: 1}
			runs, pt, err := loadRunsFromQuery(ctx, q, aclChecker)
			assert.NoErr(t, err)
			assert.Loosely(t, runs, should.BeEmpty)
			assert.Loosely(t, pt, should.BeNil)
		})

		t.Run("Limits looping when most Runs are filtered out, returning partial page instead", func(t *ftt.Test) {
			// Create batches Runs referencing the same CL with every batch having 1
			// visible Run and many not visible ones.
			// It's important that each batch has a shared project name prefix,
			// s.t. that CLQueryBuilder iterates Runs in the order of batches.
			const batchesN = queryStopAfterIterations + 2
			const invisibleN = 7
			const visibleSuffix = "visible"
			const invisibleSuffix = "zzz-no-access" // must be after visibleSuffix lexicographically
			var visible, invisible common.RunIDs
			visibleSet := stringset.New(batchesN)
			clX := common.CLID(25)
			for i := 1; i < batchesN; i++ {
				creationDelay := time.Duration(i) * time.Millisecond
				id := makeRun(fmt.Sprintf("%d-%s", i, visibleSuffix), creationDelay, clX)
				visible = append(visible, id)
				visibleSet.Add(string(id))
				for j := 1; j <= invisibleN; j++ {
					id := makeRun(fmt.Sprintf("%d-%s-%02d", i, invisibleSuffix, j), creationDelay, clX)
					invisible = append(invisible, id)
				}
			}
			// Simulate extremely slow ACL checks.
			const aclCheckDuration = queryStopAfterDuration / 20
			// Quick check test setup: the queryStopAfterDuration must be hit before
			// queryStopAfterIterations are done.
			assert.Loosely(t, aclCheckDuration*(invisibleN+1)*queryStopAfterIterations, should.BeGreaterThan(queryStopAfterDuration))
			aclChecker.beforeFunc = func(id common.RunID) error {
				ct.Clock.Add(aclCheckDuration)
				if visibleSet.Has(string(id)) {
					return nil
				}
				return errNotFound
			}

			tStart := ct.Clock.Now()
			// Run the query s.t. every iteration it loads an entire batch.
			q := CLQueryBuilder{CLID: clX, Limit: invisibleN + 1}
			runs, pt, err := loadRunsFromQuery(ctx, q, aclChecker)
			took := ct.Clock.Now().Sub(tStart)
			assert.NoErr(t, err)
			// Shouldn't have terminated prematurely.
			assert.Loosely(t, took, should.BeGreaterThanOrEqual(queryStopAfterDuration))
			// It should have loaded `queryStopAfterIterations` of batches, each
			// having 1 visible Run.
			assert.Loosely(t, idsOf(runs), should.Resemble(visible[:queryStopAfterIterations]))
			// Which must be less than the requested limit.
			assert.That(t, len(runs), should.BeLessThan(int(q.Limit)))
			// Quick check test assumption.
			assert.Loosely(t, invisibleSuffix, should.BeGreaterThan(visibleSuffix))
			// Thus the page token must point to the last Run among invisible ones.
			assert.Loosely(t, pt.GetRun(), should.Resemble(string(invisible[(queryStopAfterIterations*invisibleN)-1])))
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

func (f fakeRunChecker) After(ctx context.Context, runIfFound *run.Run) error {
	if runIfFound == nil {
		return f.afterOnNotFound
	}
	return f.after[runIfFound.ID]
}

type projQueryInTest interface {
	runKeysQuery
	LoadRuns(context.Context, ...run.LoadRunChecker) ([]*run.Run, *PageToken, error)
}

// execQueryInTest calls GetAllRunKeys and LoadRuns and returns the Run keys,
// Runs and page token.
func execQueryInTest(ctx context.Context, t testing.TB, q projQueryInTest, checkers ...run.LoadRunChecker) ([]*datastore.Key, []*run.Run, *PageToken) {
	t.Helper()

	keys, err := q.GetAllRunKeys(ctx)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	runs, pageToken, err := q.LoadRuns(ctx, checkers...)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	return keys, runs, pageToken
}

// execQueryInTestSameRunsAndKeysWithPageToken asserts that the Run keys and
// Runs match; then returns the IDs and page token.
func execQueryInTestSameRunsAndKeysWithPageToken(ctx context.Context, t testing.TB, q projQueryInTest, checkers ...run.LoadRunChecker) (common.RunIDs, *PageToken) {
	t.Helper()

	keys, runs, pageToken := execQueryInTest(ctx, t, q, checkers...)
	ids := idsOfKeys(keys)
	assert.Loosely(t, ids, should.Resemble(idsOf(runs)), truth.LineContext())
	assertCorrectPageToken(t, q, keys, pageToken)
	return ids, pageToken
}

// execQueryInTestSameRunsAndKeys asserts that the Run keys and Runs match;
// then returns just the IDs.
func execQueryInTestSameRunsAndKeys(ctx context.Context, t testing.TB, q projQueryInTest) common.RunIDs {
	t.Helper()

	ids, _ := execQueryInTestSameRunsAndKeysWithPageToken(ctx, t, q)
	return ids
}

// assertCorrectPageToken asserts that page token is as expected.
//
// That is, page token should be nil if the number of keys is less than the
// limit (or if there's no limit); and page token should have the correct value
// when the number of keys is equal to the limit.
func assertCorrectPageToken(t testing.TB, q runKeysQuery, keys []*datastore.Key, pageToken *PageToken) {
	t.Helper()
	if l := len(keys); q.qLimit() <= 0 || l < int(q.qLimit()) {
		assert.Loosely(t, pageToken, should.BeNil, truth.LineContext())
	} else {
		assert.Loosely(t, pageToken.GetRun(), should.Resemble(keys[l-1].StringID()), truth.LineContext())
	}
}

func idsOf(runs []*run.Run) common.RunIDs {
	if len(runs) == 0 {
		return nil
	}
	out := make(common.RunIDs, len(runs))
	for i, r := range runs {
		out[i] = r.ID
	}
	return out
}

func idsOfKeys(keys []*datastore.Key) common.RunIDs {
	if len(keys) == 0 {
		return nil
	}
	out := make(common.RunIDs, len(keys))
	for i, k := range keys {
		out[i] = common.RunID(k.StringID())
	}
	return out
}
