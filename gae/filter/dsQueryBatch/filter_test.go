// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package dsQueryBatch

import (
	"errors"
	"fmt"
	"testing"

	"github.com/luci/gae/filter/count"
	"github.com/luci/gae/impl/memory"
	ds "github.com/luci/gae/service/datastore"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

type Item struct {
	ID int64 `gae:"$id"`
}

func TestRun(t *testing.T) {
	t.Parallel()

	Convey("A memory datastore with a counting filter and data set installed", t, func() {
		c, cf := count.FilterRDS(memory.Use(context.Background()))

		items := make([]*Item, 2048)
		for i := range items {
			items[i] = &Item{int64(i + 1)}
		}
		if err := ds.Put(c, items); err != nil {
			panic(err)
		}
		ds.GetTestable(c).CatchupIndexes()

		for _, sizeBase := range []int{
			1,
			16,
			1024,
			2048,
		} {
			// Adjust to hit edge cases.
			for _, delta := range []int{-1, 0, 1} {
				size := sizeBase + delta
				if size <= 0 {
					continue
				}

				Convey(fmt.Sprintf(`With a batch filter size %d installed`, size), func() {
					c = BatchQueries(c, int32(size))
					q := ds.NewQuery("Item")

					Convey(`Can retrieve all of the items.`, func() {
						var got []*Item
						So(ds.GetAll(c, q, &got), ShouldBeNil)
						So(got, ShouldResemble, items)

						// One call for every sub-query, plus one to hit Stop.
						runCalls := (len(items) / size) + 1
						So(cf.Run.Successes(), ShouldEqual, runCalls)
					})

					Convey(`With a limit of 128, will retrieve 128 items.`, func() {
						const limit = 128
						q = q.Limit(int32(limit))

						var got []*Item
						So(ds.GetAll(c, q, &got), ShouldBeNil)
						So(got, ShouldResemble, items[:limit])

						// One call for every sub-query, plus one to hit Stop.
						runCalls := (limit / size)
						if (limit % size) != 0 {
							runCalls++
						}
						So(cf.Run.Successes(), ShouldEqual, runCalls)
					})
				})
			}
		}

		Convey(`With callbacks`, func() {
			const batchSize = 16
			var countA, countB int
			var errA error

			c = BatchQueries(c, int32(batchSize),
				func(context.Context) error {
					countA++
					return errA
				}, func(context.Context) error {
					countB++
					return nil
				})

			q := ds.NewQuery("Item")

			Convey(`Executes the callbacks during batching.`, func() {
				// Get 250% of the batch size. This will result in several full batches
				// and one partial batch, each of which should get a callback.
				limit := 2.5 * batchSize
				cbCount := int(limit / batchSize)

				q = q.Limit(int32(limit))
				var items []*Item
				So(ds.GetAll(c, q, &items), ShouldBeNil)
				So(len(items), ShouldEqual, limit)
				So(countA, ShouldEqual, cbCount)
				So(countB, ShouldEqual, cbCount)
			})

			Convey(`Will stop querying if a callback errors.`, func() {
				errA = errors.New("test error")

				var items []*Item
				So(ds.GetAll(c, q, &items), ShouldEqual, errA)
				So(countA, ShouldEqual, 1)
				So(countB, ShouldEqual, 0)
			})
		})
	})
}
