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

package datastore

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"

	"go.chromium.org/gae/service/info"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

type counterFilter struct {
	run    int32
	put    int32
	get    int32
	delete int32
}

func (cf *counterFilter) filter() RawFilter {
	return func(c context.Context, rds RawInterface) RawInterface {
		return &counterFilterInst{
			RawInterface:  rds,
			counterFilter: cf,
		}
	}
}

type counterFilterInst struct {
	RawInterface
	*counterFilter
}

func (rc *counterFilterInst) Run(fq *FinalizedQuery, cb RawRunCB) error {
	atomic.AddInt32(&rc.run, 1)
	return rc.RawInterface.Run(fq, cb)
}

func (rc *counterFilterInst) PutMulti(keys []*Key, pmap []PropertyMap, cb NewKeyCB) error {
	atomic.AddInt32(&rc.put, 1)
	return rc.RawInterface.PutMulti(keys, pmap, cb)
}

func (rc *counterFilterInst) GetMulti(keys []*Key, meta MultiMetaGetter, cb GetMultiCB) error {
	atomic.AddInt32(&rc.get, 1)
	return rc.RawInterface.GetMulti(keys, meta, cb)
}

func (rc *counterFilterInst) DeleteMulti(keys []*Key, cb DeleteMultiCB) error {
	atomic.AddInt32(&rc.delete, 1)
	return rc.RawInterface.DeleteMulti(keys, cb)
}

func TestQueryBatch(t *testing.T) {
	t.Parallel()

	Convey("A testing datastore with a data set installed", t, func() {
		c := info.Set(context.Background(), fakeInfo{})

		fds := fakeDatastore{
			entities: 2048,
		}
		c = SetRawFactory(c, fds.factory())

		cf := counterFilter{}
		c = AddRawFilters(c, cf.filter())

		// Given query batch size, how many Run calls will be executed to pull
		// "total" results?
		expectedBatchRunCalls := func(batchSize, total int32) int32 {
			if batchSize <= 0 {
				return 1
			}
			exp := total / batchSize
			if total%batchSize != 0 {
				exp++
			}
			return exp
		}

		// Get all items in the query, then reset the counter.
		all := []*CommonStruct(nil)
		if err := GetAll(c, NewQuery(""), &all); err != nil {
			panic(err)
		}
		cf.run = 0

		for _, sizeBase := range []int32{
			1,
			16,
			1024,
			2048,
		} {
			// Adjust to hit edge cases.
			for _, delta := range []int32{-1, 0, 1} {
				batchSize := sizeBase + delta
				if batchSize <= 0 {
					continue
				}

				getAllBatch := func(c context.Context, batchSize int32, query *Query) ([]*CommonStruct, error) {
					var out []*CommonStruct
					err := RunBatch(c, batchSize, query, func(cs *CommonStruct) {
						out = append(out, cs)
					})
					return out, err
				}

				Convey(fmt.Sprintf(`Batching with size %d installed`, batchSize), func() {
					q := NewQuery("")

					Convey(`Can retrieve all of the items.`, func() {
						got, err := getAllBatch(c, batchSize, q)
						So(err, ShouldBeNil)
						So(got, ShouldResemble, all)

						// One call for every sub-query, plus one to hit Stop.
						runCalls := (int32(len(all)) / batchSize) + 1
						So(cf.run, ShouldEqual, runCalls)
					})

					Convey(`With a limit of 128, will retrieve 128 items.`, func() {
						const limit = 128
						q = q.Limit(int32(limit))

						got, err := getAllBatch(c, batchSize, q)
						So(err, ShouldBeNil)
						So(got, ShouldResemble, all[:limit])

						So(cf.run, ShouldEqual, expectedBatchRunCalls(batchSize, limit))
					})
				})
			}
		}

		Convey(`Test iterative Run with cursors.`, func() {
			// This test will have a naive outer loop that fetches pages in large
			// increments using cursors. The outer loop will use the Batcher
			// internally, which will fetch smaller page sizes.
			testIterativeRun := func(rounds, outerFetchSize, batchSize int32) error {
				// Clear state and configure.
				cf.run = 0
				fds.entities = rounds * outerFetchSize

				var (
					outerCount int32
					cursor     Cursor
				)
				for i := int32(0); i < rounds; i++ {
					// Fetch "outerFetchSize" items from our Batcher.
					q := NewQuery("").Limit(outerFetchSize)
					if cursor != nil {
						q = q.Start(cursor)
					}

					err := RunBatch(c, batchSize, q, func(v CommonStruct, getCursor CursorCB) (err error) {
						if v.Value != int64(outerCount) {
							return fmt.Errorf("query value doesn't match count (%d != %d)", v.Value, outerCount)
						}
						outerCount++

						// Retain our cursor from this round.
						cursor, err = getCursor()
						return
					})
					if err != nil {
						return err
					}
				}

				// Make sure we iterated through everything.
				if outerCount != fds.entities {
					return fmt.Errorf("query returned incomplete results (%d != %d)", outerCount, fds.entities)
				}

				// Make sure the appropriate number of real queries was executed.
				expectedRunCount := expectedBatchRunCalls(batchSize, outerFetchSize) * rounds
				if cf.run != expectedRunCount {
					return fmt.Errorf("unexpected number of raw Run calls (%d != %d)", cf.run, expectedRunCount)
				}
				return nil
			}

			So(testIterativeRun(3, 2, 1), ShouldBeNil)
			So(testIterativeRun(3, 5, 2), ShouldBeNil)
			So(testIterativeRun(3, 1000, 250), ShouldBeNil)

			// We'll use fetch/batch sizes that are not direct multiples of each other
			// so we can test some incongruent boundaries.
			So(testIterativeRun(3, 900, 250), ShouldBeNil)
		})
	})
}

func TestBatchFilter(t *testing.T) {
	t.Parallel()

	type IndexEntity struct {
		_kind string `gae:"$kind,Index"`

		Key   *Key `gae:"$key"`
		Value int64
	}

	Convey("A testing datastore", t, func() {
		c := info.Set(context.Background(), fakeInfo{})

		fds := fakeDatastore{}
		c = SetRawFactory(c, fds.factory())

		cf := counterFilter{}
		c = AddRawFilters(c, cf.filter())

		expectedRounds := func(constraint, size int) int {
			v := size / constraint
			if size%constraint != 0 {
				v++
			}
			return v
		}

		for _, sz := range []int32{11, 10, 7, 5, 2} {
			Convey(fmt.Sprintf("With maximunm Put size %d", sz), func(convey C) {
				fds.convey = convey
				fds.constraints.MaxGetSize = 10
				fds.constraints.MaxPutSize = 10
				fds.constraints.MaxDeleteSize = 10

				css := make([]*IndexEntity, 10)
				for i := range css {
					css[i] = &IndexEntity{Value: int64(i + 1)}
				}

				So(Put(c, css), ShouldBeNil)
				So(cf.put, ShouldEqual, expectedRounds(fds.constraints.MaxPutSize, len(css)))

				for i, ent := range css {
					So(ent.Key, ShouldNotBeNil)
					So(ent.Key.IntID(), ShouldEqual, i+1)
				}

				Convey(`Get`, func() {
					// Clear Value and Get, populating Value from Key.IntID.
					for _, ent := range css {
						ent.Value = 0
					}

					So(Get(c, css), ShouldBeNil)
					So(cf.get, ShouldEqual, expectedRounds(fds.constraints.MaxGetSize, len(css)))

					for i, ent := range css {
						So(ent.Value, ShouldEqual, i+1)
					}
				})

				Convey(`Delete`, func() {
					// Record which entities get deleted.
					var lock sync.Mutex
					deleted := make(map[int64]struct{}, len(css))
					fds.onDelete = func(k *Key) {
						lock.Lock()
						defer lock.Unlock()
						deleted[k.IntID()] = struct{}{}
					}

					So(Delete(c, css), ShouldBeNil)
					So(cf.delete, ShouldEqual, expectedRounds(fds.constraints.MaxDeleteSize, len(css)))

					// Confirm that all entities have been deleted.
					So(len(deleted), ShouldEqual, len(css))
					for i := range css {
						_, ok := deleted[int64(i+1)]
						So(ok, ShouldBeTrue)
					}
				})
			})
		}
	})
}
