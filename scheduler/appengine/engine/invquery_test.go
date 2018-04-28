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
	"fmt"
	"testing"

	"golang.org/x/net/context"

	"go.chromium.org/gae/filter/featureBreaker"
	"go.chromium.org/gae/impl/memory"
	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto/google"

	"go.chromium.org/luci/scheduler/appengine/internal"
	"go.chromium.org/luci/scheduler/appengine/task"

	. "github.com/smartystreets/goconvey/convey"
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

	Convey("Empty", t, func() {
		invs, done, err := mergeInvQueries([]invQuery{
			makeInvListQ(), makeInvListQ(),
		}, 100, nil)
		So(invs, ShouldBeEmpty)
		So(done, ShouldBeTrue)
		So(err, ShouldBeNil)
	})

	Convey("Singe source, with limit", t, func() {
		invs, done, err := mergeInvQueries([]invQuery{
			makeInvListQ(1, 2, 3, 4, 5),
		}, 3, nil)
		So(invIDs(invs), ShouldResemble, []int64{1, 2, 3})
		So(done, ShouldBeFalse)
		So(err, ShouldBeNil)
	})

	Convey("Singe source, with limit, appends", t, func() {
		invs := []*Invocation{{ID: 1}, {ID: 2}}
		invs, done, err := mergeInvQueries([]invQuery{
			makeInvListQ(3, 4, 5, 6),
		}, 3, invs)
		So(invIDs(invs), ShouldResemble, []int64{1, 2, 3, 4, 5})
		So(done, ShouldBeFalse)
		So(err, ShouldBeNil)
	})

	Convey("Singe source, dups and out of order", t, func() {
		invs, done, err := mergeInvQueries([]invQuery{
			makeInvListQ(1, 2, 2, 3, 2, 4, 5),
		}, 100, nil)
		So(invIDs(invs), ShouldResemble, []int64{1, 2, 3, 4, 5})
		So(done, ShouldBeTrue)
		So(err, ShouldBeNil)
	})

	Convey("Merging", t, func() {
		invs, done, err := mergeInvQueries([]invQuery{
			makeInvListQ(1, 3, 5),
			makeInvListQ(2, 4, 6),
		}, 100, nil)
		So(invIDs(invs), ShouldResemble, []int64{1, 2, 3, 4, 5, 6})
		So(done, ShouldBeTrue)
		So(err, ShouldBeNil)
	})

	Convey("Merging with dups and limit", t, func() {
		invs, done, err := mergeInvQueries([]invQuery{
			makeInvListQ(1, 2, 3, 4, 5),
			makeInvListQ(1, 2, 3, 4, 5),
		}, 3, nil)
		So(invIDs(invs), ShouldResemble, []int64{1, 2, 3})
		So(done, ShouldBeFalse)
		So(err, ShouldBeNil)
	})

	Convey("Merging with limit that exactly matches query size", t, func() {
		invs, done, err := mergeInvQueries([]invQuery{
			makeInvListQ(1, 2, 3, 4, 5),
			makeInvListQ(1, 2, 3, 4, 5),
		}, 5, nil)
		So(invIDs(invs), ShouldResemble, []int64{1, 2, 3, 4, 5})
		So(done, ShouldBeTrue) // true here! this is important, otherwise we'll get empty pages
		So(err, ShouldBeNil)
	})
}

func TestActiveInvQuery(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		q := activeInvQuery(context.Background(), &Job{
			ActiveInvocations: []int64{1, 2, 3, 4, 5, 8, 6},
		}, 3)
		So(invIDs(q.invs), ShouldResemble, []int64{4, 5, 6, 8})
	})
}

func TestRecentInvQuery(t *testing.T) {
	t.Parallel()

	c, _ := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)
	now := google.NewTimestamp(clock.Now(c))

	Convey("Works", t, func() {
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
				{InvocationId: 9, Finished: google.NewTimestamp(clock.Now(c).Add(-FinishedInvocationsHorizon - 1))},
			}),
		}, 3)
		So(invIDs(q.invs), ShouldResemble, []int64{4, 5, 6, 8})
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

	Convey("Empty", t, func() {
		invs, err := run(c, datastore.NewQuery("Invocation"), 100)
		So(err, ShouldBeNil)
		So(len(invs), ShouldEqual, 0)
	})

	Convey("Not empty", t, func() {
		original := []*Invocation{
			{ID: 1},
			{ID: 2},
			{ID: 3},
			{ID: 4},
			{ID: 5},
		}
		datastore.Put(c, original)
		datastore.GetTestable(c).CatchupIndexes()

		Convey("No limit", func() {
			q := datastore.NewQuery("Invocation").Order("__key__")
			invs, err := run(c, q, 100)
			So(err, ShouldBeNil)
			So(invs, ShouldResemble, original)
		})

		Convey("With limit", func() {
			q := datastore.NewQuery("Invocation").Order("__key__")

			gtq := q
			invs, err := run(c, gtq, 2)
			So(err, ShouldBeNil)
			So(invs, ShouldResemble, original[:2])

			gtq = q.Gt("__key__", datastore.KeyForObj(c, invs[1]))
			invs, err = run(c, gtq, 2)
			So(err, ShouldBeNil)
			So(invs, ShouldResemble, original[2:4])

			gtq = q.Gt("__key__", datastore.KeyForObj(c, invs[1]))
			invs, err = run(c, gtq, 2)
			So(err, ShouldBeNil)
			So(invs, ShouldResemble, original[4:5])

			gtq = q.Gt("__key__", datastore.KeyForObj(c, invs[0]))
			invs, err = run(c, gtq, 2)
			So(err, ShouldBeNil)
			So(invs, ShouldBeEmpty)
		})

		Convey("With error", func() {
			dsErr := fmt.Errorf("boo")

			brokenC, breaker := featureBreaker.FilterRDS(c, nil)
			breaker.BreakFeatures(dsErr, "Run")

			q := datastore.NewQuery("Invocation").Order("__key__")
			invs, err := run(brokenC, q, 100)
			So(err, ShouldEqual, dsErr)
			So(len(invs), ShouldEqual, 0)
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

	Convey("With context", t, func() {
		c := memory.Use(context.Background())

		Convey("Empty", func() {
			q := finishedInvQuery(c, &Job{JobID: "proj/job"}, 0)
			So(fetchAll(q), ShouldBeEmpty)
		})

		Convey("Non empty", func() {
			insertInv(c, "proj/job", 3, task.StatusSucceeded)
			insertInv(c, "proj/job", 2, task.StatusSucceeded)
			insertInv(c, "proj/job", 1, task.StatusSucceeded)

			Convey("no cursor", func() {
				q := finishedInvQuery(c, &Job{JobID: "proj/job"}, 0)
				So(invIDs(fetchAll(q)), ShouldResemble, []int64{1, 2, 3})
			})

			Convey("with cursor", func() {
				q := finishedInvQuery(c, &Job{JobID: "proj/job"}, 1)
				So(invIDs(fetchAll(q)), ShouldResemble, []int64{2, 3})
			})
		})
	})
}

func TestFetchInvsPage(t *testing.T) {
	t.Parallel()

	Convey("With context", t, func() {
		const jobID = "proj/job"

		c := memory.Use(context.Background())

		// Note: we use only two queries here for simplicity, since various cases
		// involving 3 queries are not significantly different (and differences are
		// tested separately by other tests).
		makeQS := func(job *Job, opts ListInvocationsOpts, lastScanned int64) []invQuery {
			qs := []invQuery{}
			if !opts.ActiveOnly {
				ds := finishedInvQuery(c, job, lastScanned)
				Reset(ds.close)
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
				So(err, ShouldBeNil)
				So(len(invs)-before, ShouldEqual, page.count)
				So(page.count, ShouldBeLessThanOrEqualTo, opts.PageSize)
				pages = append(pages, page)
				if page.final {
					return
				}
			}
		}

		Convey("ActiveInvocations list is consistent with datastore", func() {
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

			Convey("No paging", func() {
				opts := ListInvocationsOpts{PageSize: 7}
				invs, page, err := fetchInvsPage(c, makeQS(job, opts, 0), opts, nil)
				So(err, ShouldBeNil)
				So(page, ShouldResemble, invsPage{6, true, 6})
				So(invs, ShouldResemble, []*Invocation{i1, i2, i3, i4, i5, i6})
			})

			Convey("No paging, active only", func() {
				opts := ListInvocationsOpts{PageSize: 7, ActiveOnly: true}
				invs, page, err := fetchInvsPage(c, makeQS(job, opts, 0), opts, nil)
				So(err, ShouldBeNil)
				So(page, ShouldResemble, invsPage{3, true, 3})
				So(invs, ShouldResemble, []*Invocation{i1, i2, i3})
			})

			Convey("No paging, finished only", func() {
				opts := ListInvocationsOpts{PageSize: 7, FinishedOnly: true}
				invs, page, err := fetchInvsPage(c, makeQS(job, opts, 0), opts, nil)
				So(err, ShouldBeNil)
				So(page, ShouldResemble, invsPage{3, true, 6})
				So(invs, ShouldResemble, []*Invocation{i4, i5, i6})
			})

			Convey("Paging", func() {
				opts := ListInvocationsOpts{PageSize: 2}
				invs, pages := fetchAllPages(makeQS(job, opts, 0), opts)
				So(invs, ShouldResemble, []*Invocation{i1, i2, i3, i4, i5, i6})
				So(pages, ShouldResemble, []invsPage{
					{2, false, 2},
					{2, false, 4},
					{2, true, 6},
				})
			})

			Convey("Paging, resuming from cursor", func() {
				opts := ListInvocationsOpts{PageSize: 2}
				invs, pages := fetchAllPages(makeQS(job, opts, 3), opts)
				So(invs, ShouldResemble, []*Invocation{i4, i5, i6})
				So(pages, ShouldResemble, []invsPage{
					{2, false, 5},
					{1, true, 6},
				})
			})

			Convey("Paging, active only", func() {
				opts := ListInvocationsOpts{PageSize: 2, ActiveOnly: true}
				invs, pages := fetchAllPages(makeQS(job, opts, 0), opts)
				So(invs, ShouldResemble, []*Invocation{i1, i2, i3})
				So(pages, ShouldResemble, []invsPage{
					{2, false, 2},
					{1, true, 3},
				})
			})

			Convey("Paging, finished only", func() {
				opts := ListInvocationsOpts{PageSize: 2, FinishedOnly: true}
				invs, pages := fetchAllPages(makeQS(job, opts, 0), opts)
				So(invs, ShouldResemble, []*Invocation{i4, i5, i6})
				So(pages, ShouldResemble, []invsPage{
					{2, false, 5},
					{1, true, 6},
				})
			})
		})

		Convey("ActiveInvocations list is stale", func() {
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

			Convey("No paging", func() {
				opts := ListInvocationsOpts{PageSize: 7}
				invs, page, err := fetchInvsPage(c, makeQS(job, opts, 0), opts, nil)
				So(err, ShouldBeNil)
				So(page, ShouldResemble, invsPage{6, true, 6})
				So(invs, ShouldResemble, []*Invocation{i1, i2, i3, i4, i5, i6})
			})

			Convey("No paging, active only", func() {
				opts := ListInvocationsOpts{PageSize: 7, ActiveOnly: true}
				invs, page, err := fetchInvsPage(c, makeQS(job, opts, 0), opts, nil)
				So(err, ShouldBeNil)
				So(page, ShouldResemble, invsPage{2, true, 3}) // 3 was scanned and skipped!
				So(invs, ShouldResemble, []*Invocation{i1, i2})
			})

			Convey("No paging, finished only", func() {
				opts := ListInvocationsOpts{PageSize: 7, FinishedOnly: true}
				invs, page, err := fetchInvsPage(c, makeQS(job, opts, 0), opts, nil)
				So(err, ShouldBeNil)
				So(page, ShouldResemble, invsPage{4, true, 6})
				So(invs, ShouldResemble, []*Invocation{i3, i4, i5, i6})
			})

			Convey("Paging", func() {
				opts := ListInvocationsOpts{PageSize: 2}
				invs, pages := fetchAllPages(makeQS(job, opts, 0), opts)
				So(invs, ShouldResemble, []*Invocation{i1, i2, i3, i4, i5, i6})
				So(pages, ShouldResemble, []invsPage{
					{2, false, 2},
					{2, false, 4},
					{2, true, 6},
				})
			})

			Convey("Paging, resuming from cursor", func() {
				opts := ListInvocationsOpts{PageSize: 2}
				invs, pages := fetchAllPages(makeQS(job, opts, 3), opts)
				So(invs, ShouldResemble, []*Invocation{i4, i5, i6})
				So(pages, ShouldResemble, []invsPage{
					{2, false, 5},
					{1, true, 6},
				})
			})

			Convey("Paging, active only", func() {
				opts := ListInvocationsOpts{PageSize: 1, ActiveOnly: true}
				invs, pages := fetchAllPages(makeQS(job, opts, 0), opts)
				So(invs, ShouldResemble, []*Invocation{i1, i2})
				So(pages, ShouldResemble, []invsPage{
					{1, false, 1},
					{1, false, 2},
					{0, true, 3}, // empty mini-page, but advanced cursor
				})
			})

			Convey("Paging, finished only", func() {
				opts := ListInvocationsOpts{PageSize: 2, FinishedOnly: true}
				invs, pages := fetchAllPages(makeQS(job, opts, 0), opts)
				So(invs, ShouldResemble, []*Invocation{i3, i4, i5, i6})
				So(pages, ShouldResemble, []invsPage{
					{2, false, 4},
					{2, true, 6},
				})
			})
		})
	})
}

func TestFillShallowInvs(t *testing.T) {
	t.Parallel()

	Convey("With context", t, func() {
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

		Convey("no filtering", func() {
			fat, err := fillShallowInvs(c, shallow, ListInvocationsOpts{})
			So(err, ShouldBeNil)
			So(fat, ShouldResemble, []*Invocation{
				{ID: 1, Status: task.StatusSucceeded},
				{ID: 2, Status: task.StatusSucceeded},
				{ID: 3, Status: task.StatusRunning},
				{ID: 10, Status: task.StatusRunning},
				{ID: 10, Status: task.StatusRunning},
				{ID: 11, Status: task.StatusSucceeded},
			})
			So(&shallow[0], ShouldEqual, &fat[0]) // same backing array
		})

		Convey("finished only", func() {
			fat, err := fillShallowInvs(c, shallow, ListInvocationsOpts{
				FinishedOnly: true,
			})
			So(err, ShouldBeNil)
			So(fat, ShouldResemble, []*Invocation{
				{ID: 1, Status: task.StatusSucceeded},
				{ID: 2, Status: task.StatusSucceeded},
				{ID: 11, Status: task.StatusSucceeded},
			})
			So(&shallow[0], ShouldEqual, &fat[0]) // same backing array
		})

		Convey("active only", func() {
			fat, err := fillShallowInvs(c, shallow, ListInvocationsOpts{
				ActiveOnly: true,
			})
			So(err, ShouldBeNil)
			So(fat, ShouldResemble, []*Invocation{
				{ID: 3, Status: task.StatusRunning},
				{ID: 10, Status: task.StatusRunning},
				{ID: 10, Status: task.StatusRunning},
			})
			So(&shallow[0], ShouldEqual, &fat[0]) // same backing array
		})
	})
}
