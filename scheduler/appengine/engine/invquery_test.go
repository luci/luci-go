// Copyright 2018 The LUCI Authors.
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

package engine

import (
	"context"
	"fmt"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"
)

func makeInvListQ(ids ...int64) *invListQuery {
	invs := make([]*Invocation, len(ids))
	for i, id := range ids {
		invs[i] = &Invocation{ID: id}
	}
	return &invListQuery{invs, 0}
}

func invIDs(invs []*Invocation) []int64 {
	out := make([]int64, len(invs))
	for i, inv := range invs {
		out[i] = inv.ID
	}
	return out
}

func TestMergeInvQueries(t *testing.T) {
	t.Parallel()

	ftt.Run("Empty", t, func(t *ftt.Test) {
		invs, done, err := mergeInvQueries([]invQuery{
			makeInvListQ(), makeInvListQ(),
		}, 100, nil)
		assert.Loosely(t, invs, should.BeEmpty)
		assert.Loosely(t, done, should.BeTrue)
		assert.Loosely(t, err, should.BeNil)
	})

	ftt.Run("Single source, with limit", t, func(t *ftt.Test) {
		invs, done, err := mergeInvQueries([]invQuery{
			makeInvListQ(1, 2, 3, 4, 5),
		}, 3, nil)
		assert.Loosely(t, invIDs(invs), should.Resemble([]int64{1, 2, 3}))
		assert.Loosely(t, done, should.BeFalse)
		assert.Loosely(t, err, should.BeNil)
	})

	ftt.Run("Single source, with limit, appends", t, func(t *ftt.Test) {
		invs := []*Invocation{{ID: 1}, {ID: 2}}
		invs, done, err := mergeInvQueries([]invQuery{
			makeInvListQ(3, 4, 5, 6),
		}, 3, invs)
		assert.Loosely(t, invIDs(invs), should.Resemble([]int64{1, 2, 3, 4, 5}))
		assert.Loosely(t, done, should.BeFalse)
		assert.Loosely(t, err, should.BeNil)
	})

	ftt.Run("Single source, dups and out of order", t, func(t *ftt.Test) {
		invs, done, err := mergeInvQueries([]invQuery{
			makeInvListQ(1, 2, 2, 3, 2, 4, 5),
		}, 100, nil)
		assert.Loosely(t, invIDs(invs), should.Resemble([]int64{1, 2, 3, 4, 5}))
		assert.Loosely(t, done, should.BeTrue)
		assert.Loosely(t, err, should.BeNil)
	})

	ftt.Run("Merging", t, func(t *ftt.Test) {
		invs, done, err := mergeInvQueries([]invQuery{
			makeInvListQ(1, 3, 5),
			makeInvListQ(2, 4, 6),
		}, 100, nil)
		assert.Loosely(t, invIDs(invs), should.Resemble([]int64{1, 2, 3, 4, 5, 6}))
		assert.Loosely(t, done, should.BeTrue)
		assert.Loosely(t, err, should.BeNil)
	})

	ftt.Run("Merging with dups and limit", t, func(t *ftt.Test) {
		invs, done, err := mergeInvQueries([]invQuery{
			makeInvListQ(1, 2, 3, 4, 5),
			makeInvListQ(1, 2, 3, 4, 5),
		}, 3, nil)
		assert.Loosely(t, invIDs(invs), should.Resemble([]int64{1, 2, 3}))
		assert.Loosely(t, done, should.BeFalse)
		assert.Loosely(t, err, should.BeNil)
	})

	ftt.Run("Merging with limit that exactly matches query size", t, func(t *ftt.Test) {
		invs, done, err := mergeInvQueries([]invQuery{
			makeInvListQ(1, 2, 3, 4, 5),
			makeInvListQ(1, 2, 3, 4, 5),
		}, 5, nil)
		assert.Loosely(t, invIDs(invs), should.Resemble([]int64{1, 2, 3, 4, 5}))
		assert.Loosely(t, done, should.BeTrue) // true here! this is important, otherwise we'll get empty pages
		assert.Loosely(t, err, should.BeNil)
	})
}

func TestActiveInvQuery(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		q := activeInvQuery(context.Background(), &Job{
			ActiveInvocations: []int64{1, 2, 3, 4, 5, 8, 6},
		}, 3)
		assert.Loosely(t, invIDs(q.invs), should.Resemble([]int64{4, 5, 6, 8}))
	})
}

func TestRecentInvQuery(t *testing.T) {
	t.Parallel()

	c, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
	now := timestamppb.New(clock.Now(c))

	ftt.Run("Works", t, func(t *ftt.Test) {
		q := recentInvQuery(c, &Job{
			FinishedInvocationsRaw: marshalFinishedInvs([]*internal.FinishedInvocation{
				{InvocationId: 1, Finished: now},
				{InvocationId: 2, Finished: now},
				{InvocationId: 3, Finished: now},
				{InvocationId: 4, Finished: now},
				{InvocationId: 5, Finished: now},
				{InvocationId: 8, Finished: now},
				{InvocationId: 6, Finished: now},
				// And this one should be ignored, as it is "too old".
				{InvocationId: 9, Finished: timestamppb.New(clock.Now(c).Add(-FinishedInvocationsHorizon - 1))},
			}),
		}, 3)
		assert.Loosely(t, invIDs(q.invs), should.Resemble([]int64{4, 5, 6, 8}))
	})
}

func TestInvDatastoreIter(t *testing.T) {
	t.Parallel()

	run := func(c context.Context, query *datastore.Query, limit int) ([]*Invocation, error) {
		it := invDatastoreIter{}
		it.start(c, query)
		defer it.stop()
		invs := []*Invocation{}
		for len(invs) != limit {
			switch inv, err := it.next(); {
			case err != nil:
				return nil, err
			case inv == nil:
				return invs, nil // fetched everything we had
			default:
				invs = append(invs, inv)
			}
		}
		return invs, nil
	}

	c := memory.Use(context.Background())

	ftt.Run("Empty", t, func(t *ftt.Test) {
		invs, err := run(c, datastore.NewQuery("Invocation"), 100)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, len(invs), should.BeZero)
	})

	ftt.Run("Not empty", t, func(t *ftt.Test) {
		original := []*Invocation{
			{ID: 1},
			{ID: 2},
			{ID: 3},
			{ID: 4},
			{ID: 5},
		}
		datastore.Put(c, original)
		datastore.GetTestable(c).CatchupIndexes()

		t.Run("No limit", func(t *ftt.Test) {
			q := datastore.NewQuery("Invocation").Order("__key__")
			invs, err := run(c, q, 100)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invs, should.Resemble(original))
		})

		t.Run("With limit", func(t *ftt.Test) {
			q := datastore.NewQuery("Invocation").Order("__key__")

			gtq := q
			invs, err := run(c, gtq, 2)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invs, should.Resemble(original[:2]))

			gtq = q.Gt("__key__", datastore.KeyForObj(c, invs[1]))
			invs, err = run(c, gtq, 2)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invs, should.Resemble(original[2:4]))

			gtq = q.Gt("__key__", datastore.KeyForObj(c, invs[1]))
			invs, err = run(c, gtq, 2)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invs, should.Resemble(original[4:5]))

			gtq = q.Gt("__key__", datastore.KeyForObj(c, invs[0]))
			invs, err = run(c, gtq, 2)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, invs, should.BeEmpty)
		})

		t.Run("With error", func(t *ftt.Test) {
			dsErr := fmt.Errorf("boo")

			brokenC, breaker := featureBreaker.FilterRDS(c, nil)
			breaker.BreakFeatures(dsErr, "Run")

			q := datastore.NewQuery("Invocation").Order("__key__")
			invs, err := run(brokenC, q, 100)
			assert.Loosely(t, err, should.Equal(dsErr))
			assert.Loosely(t, len(invs), should.BeZero)
		})
	})
}

func insertInv(c context.Context, jobID string, invID int64, status task.Status) *Invocation {
	inv := &Invocation{
		ID:     invID,
		JobID:  jobID,
		Status: status,
	}
	if status.Final() {
		inv.IndexedJobID = jobID
	}
	if err := datastore.Put(c, inv); err != nil {
		panic(err)
	}
	datastore.GetTestable(c).CatchupIndexes()
	return inv
}

func TestFinishedInvQuery(t *testing.T) {
	t.Parallel()

	fetchAll := func(q *invDatastoreQuery) (out []*Invocation) {
		defer q.close()
		for {
			inv, err := q.peek()
			if err != nil {
				panic(err)
			}
			if inv == nil {
				return
			}
			out = append(out, inv)
			if err := q.advance(); err != nil {
				panic(err)
			}
		}
	}

	ftt.Run("With context", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())

		t.Run("Empty", func(t *ftt.Test) {
			q := finishedInvQuery(c, &Job{JobID: "proj/job"}, 0)
			assert.Loosely(t, fetchAll(q), should.BeEmpty)
		})

		t.Run("Non empty", func(t *ftt.Test) {
			insertInv(c, "proj/job", 3, task.StatusSucceeded)
			insertInv(c, "proj/job", 2, task.StatusSucceeded)
			insertInv(c, "proj/job", 1, task.StatusSucceeded)

			t.Run("no cursor", func(t *ftt.Test) {
				q := finishedInvQuery(c, &Job{JobID: "proj/job"}, 0)
				assert.Loosely(t, invIDs(fetchAll(q)), should.Resemble([]int64{1, 2, 3}))
			})

			t.Run("with cursor", func(t *ftt.Test) {
				q := finishedInvQuery(c, &Job{JobID: "proj/job"}, 1)
				assert.Loosely(t, invIDs(fetchAll(q)), should.Resemble([]int64{2, 3}))
			})
		})
	})
}

func TestFetchInvsPage(t *testing.T) {
	t.Parallel()

	ftt.Run("With context", t, func(t *ftt.Test) {
		const jobID = "proj/job"

		c := memory.Use(context.Background())

		// Note: we use only two queries here for simplicity, since various cases
		// involving 3 queries are not significantly different (and differences are
		// tested separately by other tests).
		makeQS := func(job *Job, opts ListInvocationsOpts, lastScanned int64) []invQuery {
			qs := []invQuery{}
			if !opts.ActiveOnly {
				ds := finishedInvQuery(c, job, lastScanned)
				t.Cleanup(ds.close)
				qs = append(qs, ds)
			}
			if !opts.FinishedOnly {
				qs = append(qs, activeInvQuery(c, job, lastScanned))
			}
			return qs
		}

		fetchAllPages := func(qs []invQuery, opts ListInvocationsOpts) (invs []*Invocation, pages []invsPage) {
			var page invsPage
			var err error
			for {
				before := len(invs)
				invs, page, err = fetchInvsPage(c, qs, opts, invs)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(invs)-before, should.Equal(page.count))
				assert.Loosely(t, page.count, should.BeLessThanOrEqual(opts.PageSize))
				pages = append(pages, page)
				if page.final {
					return
				}
			}
		}

		t.Run("ActiveInvocations list is consistent with datastore", func(t *ftt.Test) {
			// List of finished invocations, oldest to newest.
			i6 := insertInv(c, jobID, 6, task.StatusSucceeded)
			i5 := insertInv(c, jobID, 5, task.StatusFailed)
			i4 := insertInv(c, jobID, 4, task.StatusSucceeded)
			// List of still running invocations, oldest to newest.
			i3 := insertInv(c, jobID, 3, task.StatusRunning)
			i2 := insertInv(c, jobID, 2, task.StatusRunning)
			i1 := insertInv(c, jobID, 1, task.StatusRunning)

			job := &Job{
				JobID: jobID,
				ActiveInvocations: []int64{
					3, 1, 2, // the set of active invocations, unordered
				},
			}

			t.Run("No paging", func(t *ftt.Test) {
				opts := ListInvocationsOpts{PageSize: 7}
				invs, page, err := fetchInvsPage(c, makeQS(job, opts, 0), opts, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, page, should.Resemble(invsPage{6, true, 6}))
				assert.Loosely(t, invs, should.Resemble([]*Invocation{i1, i2, i3, i4, i5, i6}))
			})

			t.Run("No paging, active only", func(t *ftt.Test) {
				opts := ListInvocationsOpts{PageSize: 7, ActiveOnly: true}
				invs, page, err := fetchInvsPage(c, makeQS(job, opts, 0), opts, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, page, should.Resemble(invsPage{3, true, 3}))
				assert.Loosely(t, invs, should.Resemble([]*Invocation{i1, i2, i3}))
			})

			t.Run("No paging, finished only", func(t *ftt.Test) {
				opts := ListInvocationsOpts{PageSize: 7, FinishedOnly: true}
				invs, page, err := fetchInvsPage(c, makeQS(job, opts, 0), opts, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, page, should.Resemble(invsPage{3, true, 6}))
				assert.Loosely(t, invs, should.Resemble([]*Invocation{i4, i5, i6}))
			})

			t.Run("Paging", func(t *ftt.Test) {
				opts := ListInvocationsOpts{PageSize: 2}
				invs, pages := fetchAllPages(makeQS(job, opts, 0), opts)
				assert.Loosely(t, invs, should.Resemble([]*Invocation{i1, i2, i3, i4, i5, i6}))
				assert.Loosely(t, pages, should.Resemble([]invsPage{
					{2, false, 2},
					{2, false, 4},
					{2, true, 6},
				}))
			})

			t.Run("Paging, resuming from cursor", func(t *ftt.Test) {
				opts := ListInvocationsOpts{PageSize: 2}
				invs, pages := fetchAllPages(makeQS(job, opts, 3), opts)
				assert.Loosely(t, invs, should.Resemble([]*Invocation{i4, i5, i6}))
				assert.Loosely(t, pages, should.Resemble([]invsPage{
					{2, false, 5},
					{1, true, 6},
				}))
			})

			t.Run("Paging, active only", func(t *ftt.Test) {
				opts := ListInvocationsOpts{PageSize: 2, ActiveOnly: true}
				invs, pages := fetchAllPages(makeQS(job, opts, 0), opts)
				assert.Loosely(t, invs, should.Resemble([]*Invocation{i1, i2, i3}))
				assert.Loosely(t, pages, should.Resemble([]invsPage{
					{2, false, 2},
					{1, true, 3},
				}))
			})

			t.Run("Paging, finished only", func(t *ftt.Test) {
				opts := ListInvocationsOpts{PageSize: 2, FinishedOnly: true}
				invs, pages := fetchAllPages(makeQS(job, opts, 0), opts)
				assert.Loosely(t, invs, should.Resemble([]*Invocation{i4, i5, i6}))
				assert.Loosely(t, pages, should.Resemble([]invsPage{
					{2, false, 5},
					{1, true, 6},
				}))
			})
		})

		t.Run("ActiveInvocations list is stale", func(t *ftt.Test) {
			// List of finished invocations, oldest to newest.
			i6 := insertInv(c, jobID, 6, task.StatusSucceeded)
			i5 := insertInv(c, jobID, 5, task.StatusFailed)
			i4 := insertInv(c, jobID, 4, task.StatusSucceeded)
			// List of still invocations referenced by ActiveInvocations.
			i3 := insertInv(c, jobID, 3, task.StatusSucceeded) // actually done!
			i2 := insertInv(c, jobID, 2, task.StatusRunning)
			i1 := insertInv(c, jobID, 1, task.StatusRunning)

			job := &Job{
				JobID:             jobID,
				ActiveInvocations: []int64{3, 1, 2},
			}

			t.Run("No paging", func(t *ftt.Test) {
				opts := ListInvocationsOpts{PageSize: 7}
				invs, page, err := fetchInvsPage(c, makeQS(job, opts, 0), opts, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, page, should.Resemble(invsPage{6, true, 6}))
				assert.Loosely(t, invs, should.Resemble([]*Invocation{i1, i2, i3, i4, i5, i6}))
			})

			t.Run("No paging, active only", func(t *ftt.Test) {
				opts := ListInvocationsOpts{PageSize: 7, ActiveOnly: true}
				invs, page, err := fetchInvsPage(c, makeQS(job, opts, 0), opts, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, page, should.Resemble(invsPage{2, true, 3})) // 3 was scanned and skipped!
				assert.Loosely(t, invs, should.Resemble([]*Invocation{i1, i2}))
			})

			t.Run("No paging, finished only", func(t *ftt.Test) {
				opts := ListInvocationsOpts{PageSize: 7, FinishedOnly: true}
				invs, page, err := fetchInvsPage(c, makeQS(job, opts, 0), opts, nil)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, page, should.Resemble(invsPage{4, true, 6}))
				assert.Loosely(t, invs, should.Resemble([]*Invocation{i3, i4, i5, i6}))
			})

			t.Run("Paging", func(t *ftt.Test) {
				opts := ListInvocationsOpts{PageSize: 2}
				invs, pages := fetchAllPages(makeQS(job, opts, 0), opts)
				assert.Loosely(t, invs, should.Resemble([]*Invocation{i1, i2, i3, i4, i5, i6}))
				assert.Loosely(t, pages, should.Resemble([]invsPage{
					{2, false, 2},
					{2, false, 4},
					{2, true, 6},
				}))
			})

			t.Run("Paging, resuming from cursor", func(t *ftt.Test) {
				opts := ListInvocationsOpts{PageSize: 2}
				invs, pages := fetchAllPages(makeQS(job, opts, 3), opts)
				assert.Loosely(t, invs, should.Resemble([]*Invocation{i4, i5, i6}))
				assert.Loosely(t, pages, should.Resemble([]invsPage{
					{2, false, 5},
					{1, true, 6},
				}))
			})

			t.Run("Paging, active only", func(t *ftt.Test) {
				opts := ListInvocationsOpts{PageSize: 1, ActiveOnly: true}
				invs, pages := fetchAllPages(makeQS(job, opts, 0), opts)
				assert.Loosely(t, invs, should.Resemble([]*Invocation{i1, i2}))
				assert.Loosely(t, pages, should.Resemble([]invsPage{
					{1, false, 1},
					{1, false, 2},
					{0, true, 3}, // empty mini-page, but advanced cursor
				}))
			})

			t.Run("Paging, finished only", func(t *ftt.Test) {
				opts := ListInvocationsOpts{PageSize: 2, FinishedOnly: true}
				invs, pages := fetchAllPages(makeQS(job, opts, 0), opts)
				assert.Loosely(t, invs, should.Resemble([]*Invocation{i3, i4, i5, i6}))
				assert.Loosely(t, pages, should.Resemble([]invsPage{
					{2, false, 4},
					{2, true, 6},
				}))
			})
		})
	})
}

func TestFillShallowInvs(t *testing.T) {
	t.Parallel()

	ftt.Run("With context", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())

		// Bodies for inflated items.
		datastore.Put(c, []*Invocation{
			{ID: 1, Status: task.StatusSucceeded},
			{ID: 10, Status: task.StatusRunning},
		})

		shallow := []*Invocation{
			{ID: 1}, // to be inflated
			{ID: 2, Status: task.StatusSucceeded},
			{ID: 3, Status: task.StatusRunning},
			{ID: 10}, // to be inflated
			{ID: 10}, // to be inflated, again... as an edge case
			{ID: 11, Status: task.StatusSucceeded},
		}

		t.Run("no filtering", func(t *ftt.Test) {
			fat, err := fillShallowInvs(c, shallow, ListInvocationsOpts{})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fat, should.Resemble([]*Invocation{
				{ID: 1, Status: task.StatusSucceeded},
				{ID: 2, Status: task.StatusSucceeded},
				{ID: 3, Status: task.StatusRunning},
				{ID: 10, Status: task.StatusRunning},
				{ID: 10, Status: task.StatusRunning},
				{ID: 11, Status: task.StatusSucceeded},
			}))
			assert.Loosely(t, &shallow[0], should.Equal(&fat[0])) // same backing array
		})

		t.Run("finished only", func(t *ftt.Test) {
			fat, err := fillShallowInvs(c, shallow, ListInvocationsOpts{
				FinishedOnly: true,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fat, should.Resemble([]*Invocation{
				{ID: 1, Status: task.StatusSucceeded},
				{ID: 2, Status: task.StatusSucceeded},
				{ID: 11, Status: task.StatusSucceeded},
			}))
			assert.Loosely(t, &shallow[0], should.Equal(&fat[0])) // same backing array
		})

		t.Run("active only", func(t *ftt.Test) {
			fat, err := fillShallowInvs(c, shallow, ListInvocationsOpts{
				ActiveOnly: true,
			})
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, fat, should.Resemble([]*Invocation{
				{ID: 3, Status: task.StatusRunning},
				{ID: 10, Status: task.StatusRunning},
				{ID: 10, Status: task.StatusRunning},
			}))
			assert.Loosely(t, &shallow[0], should.Equal(&fat[0])) // same backing array
		})
	})
}
