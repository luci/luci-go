// Copyright 2016 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dsQueryBatch

import (
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

	Convey("A memory with a counting filter and data set installed", t, func() {
		c, cf := count.FilterRDS(memory.Use(context.Background()))

		items := make([]*Item, 1024)
		for i := range items {
			items[i] = &Item{int64(i + 1)}
		}
		if err := ds.Get(c).PutMulti(items); err != nil {
			panic(err)
		}
		ds.Get(c).Testable().CatchupIndexes()

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
						So(ds.Get(c).GetAll(q, &got), ShouldBeNil)
						So(got, ShouldResemble, items)

						// One call for every sub-query, plus one to hit Stop.
						runCalls := (len(items) / size) + 1
						So(cf.Run.Successes(), ShouldEqual, runCalls)
					})

					Convey(`With a limit of 128, will retrieve 128 items.`, func() {
						const limit = 128
						q = q.Limit(int32(limit))

						var got []*Item
						So(ds.Get(c).GetAll(q, &got), ShouldBeNil)
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
	})
}
