// Copyright 2019 The LUCI Authors.
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

package buffer

import (
	"context"
	"math/rand"
	"strings"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/retry"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBuffer(t *testing.T) {
	must := func(b *Batch[any], err error) *Batch[any] {
		So(err, ShouldBeNil)
		return b
	}
	addNoBlockZero := func(b *Buffer[any], now time.Time, item any) (*Batch[any], error) {
		return b.AddNoBlock(now, item, 0)
	}
	addNoBlockStr := func(b *Buffer[any], now time.Time, item string) (*Batch[any], error) {
		return b.AddNoBlock(now, item, len(item))
	}

	Convey(`Buffer`, t, func() {
		Convey(`construction`, func() {
			Convey(`success`, func() {
				b, err := NewBuffer[any](nil)
				So(err, ShouldBeNil)

				So(b.Stats(), ShouldResemble, Stats{})
				So(b.NextSendTime(), ShouldBeZeroValue)
				So(b.CanAddItem(), ShouldBeTrue)
				So(b.LeaseOne(time.Time{}), ShouldBeNil)
			})
			Convey(`fail`, func() {
				_, err := NewBuffer[any](&Options{BatchItemsMax: -100})
				So(err, ShouldErrLike, "normalizing buffer.Options")
			})
		})

		Convey(`usage`, func() {
			start := testclock.TestRecentTimeUTC
			ctx, tclock := testclock.UseTime(context.Background(), start)

			Convey(`common behavior`, func() {
				b, err := NewBuffer[any](&Options{
					MaxLeases:     2,
					BatchItemsMax: 20,
				})
				So(err, ShouldBeNil)
				nextSendTimeOffset := b.opts.BatchAgeMax

				Convey(`batch cut by count`, func() {
					for i := 0; i < int(b.opts.BatchItemsMax); i++ {
						So(b.unleased.Len(), ShouldEqual, 0)
						if i > 0 {
							// The next send time should be when the current batch will be
							// forcibly cut due to BatchAgeMax.
							So(b.NextSendTime(), ShouldResemble, start.Add(nextSendTimeOffset))
						}
						So(must(addNoBlockZero(b, clock.Now(ctx), i)), ShouldBeNil)
					}

					So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 20})
					So(b.Stats().Total(), ShouldResemble, 20)
					So(b.Stats().Empty(), ShouldResemble, false)
					// The next send time should be when the current batch is available to
					// send. this is a test and time hasn't advanced, it's reset
					// back to the start time.
					So(b.NextSendTime(), ShouldEqual, start)
					So(b.CanAddItem(), ShouldBeTrue)
					So(b.unleased.Len(), ShouldEqual, 1)
					So(b.currentBatch, ShouldBeNil)

					batch := b.LeaseOne(clock.Now(ctx))
					So(b.LeaseOne(clock.Now(ctx)), ShouldBeNil)

					So(b.stats, ShouldResemble, Stats{LeasedItemCount: 20})
					So(batch.Data, ShouldHaveLength, b.opts.BatchItemsMax)
					for i := range batch.Data {
						So(batch.Data[i].Item, ShouldEqual, i)
					}

					Convey(`ACK`, func() {
						b.ACK(batch)

						So(b.stats, ShouldResemble, Stats{})
						So(b.Stats().Total(), ShouldResemble, 0)
						So(b.Stats().Empty(), ShouldResemble, true)

						Convey(`double ACK panic`, func() {
							So(func() { b.ACK(batch) }, ShouldPanicLike, "unknown *Batch")
							So(b.stats, ShouldResemble, Stats{})
						})
					})

					Convey(`Partial NACK`, func() {
						batch.Data = batch.Data[:10] // pretend we processed some Data

						b.NACK(ctx, nil, batch)

						So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 10})
						So(b.unleased.Len(), ShouldEqual, 1)

						Convey(`Adding Data does nothing`, func() {
							// no batch yet; the one we NACK'd is sleeping
							So(b.LeaseOne(clock.Now(ctx)), ShouldBeNil)
							tclock.Set(b.NextSendTime())
							newBatch := b.LeaseOne(clock.Now(ctx))
							So(newBatch, ShouldNotBeNil)

							newBatch.Data = append(newBatch.Data, BatchItem[any]{}, BatchItem[any]{}, BatchItem[any]{})

							b.NACK(ctx, nil, newBatch)

							So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 10})
							So(b.unleased.Len(), ShouldEqual, 1)
						})
					})

					Convey(`Full NACK`, func() {
						b.NACK(ctx, nil, batch)

						So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 20})
						So(b.unleased.Len(), ShouldEqual, 1)

						Convey(`double NACK panic`, func() {
							So(func() { b.NACK(ctx, nil, batch) }, ShouldPanicLike, "unknown *Batch")
							So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 20})
							So(b.unleased.Len(), ShouldEqual, 1)
						})
					})

					Convey(`Max leases limits LeaseOne`, func() {
						now := clock.Now(ctx)
						So(must(addNoBlockZero(b, now, 10)), ShouldBeNil)
						b.Flush(now)
						So(must(addNoBlockZero(b, now, 20)), ShouldBeNil)
						b.Flush(now)

						So(b.LeaseOne(clock.Now(ctx)), ShouldNotBeNil)

						// There's something to send
						So(b.NextSendTime(), ShouldNotBeZeroValue)

						// But lease limit of 2 is hit
						So(b.LeaseOne(clock.Now(ctx)), ShouldBeNil)
					})
				})

				Convey(`batch cut by time`, func() {
					So(must(addNoBlockZero(b, clock.Now(ctx), "bobbie")), ShouldBeNil)

					// should be equal to timeout of first batch, plus 1ms
					nextSend := b.NextSendTime()
					So(nextSend, ShouldResemble, start.Add(nextSendTimeOffset))
					tclock.Set(nextSend)

					So(must(addNoBlockZero(b, clock.Now(ctx), "charlie")), ShouldBeNil)
					So(must(addNoBlockZero(b, clock.Now(ctx), "dakota")), ShouldBeNil)

					// We haven't leased one yet, so NextSendTime should stay the same.
					nextSend = b.NextSendTime()
					So(nextSend, ShouldResemble, start.Add(nextSendTimeOffset))

					// Eventually time passes, and we can lease the batch.
					tclock.Set(nextSend)
					batch := b.LeaseOne(clock.Now(ctx))
					So(batch, ShouldNotBeNil)

					// that batch included everything
					So(b.NextSendTime(), ShouldBeZeroValue)

					So(batch.Data, ShouldHaveLength, 3)
					So(batch.Data[0].Item, ShouldResemble, "bobbie")
					So(batch.Data[1].Item, ShouldResemble, "charlie")
					So(batch.Data[2].Item, ShouldResemble, "dakota")
				})

				Convey(`batch cut by flush`, func() {
					So(must(addNoBlockZero(b, clock.Now(ctx), "bobbie")), ShouldBeNil)
					So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 1})

					So(b.LeaseOne(clock.Now(ctx)), ShouldBeNil)

					b.Flush(start)
					So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 1})
					So(b.currentBatch, ShouldBeNil)
					So(b.unleased.data, ShouldHaveLength, 1)

					Convey(`double flush is noop`, func() {
						b.Flush(start)
						So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 1})
						So(b.currentBatch, ShouldBeNil)
						So(b.unleased.data, ShouldHaveLength, 1)
					})

					batch := b.LeaseOne(clock.Now(ctx))
					So(batch, ShouldNotBeNil)
					So(b.stats, ShouldResemble, Stats{LeasedItemCount: 1})

					b.ACK(batch)

					So(b.stats.Empty(), ShouldBeTrue)
				})
			})

			Convey(`retry limit eventually drops batch`, func() {
				b, err := NewBuffer[any](&Options{
					BatchItemsMax: 1,
					Retry: func() retry.Iterator {
						return &retry.Limited{Retries: 1}
					},
				})
				So(err, ShouldBeNil)

				So(must(addNoBlockZero(b, clock.Now(ctx), 1)), ShouldBeNil)

				b.NACK(ctx, nil, b.LeaseOne(clock.Now(ctx)))
				So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 1})
				b.NACK(ctx, nil, b.LeaseOne(clock.Now(ctx)))
				// only one retry was allowed, start it's gone.
				So(b.stats, ShouldResemble, Stats{})
			})

			Convey(`in-order delivery`, func() {
				b, err := NewBuffer[any](&Options{
					MaxLeases:     1,
					BatchItemsMax: 1,
					FullBehavior:  InfiniteGrowth{},
					FIFO:          true,
					Retry: func() retry.Iterator {
						return &retry.Limited{Retries: -1}
					},
				})
				So(err, ShouldBeNil)

				expect := make([]int, 20)
				for i := 0; i < 20; i++ {
					expect[i] = i
					So(must(addNoBlockZero(b, clock.Now(ctx), i)), ShouldBeNil)
					tclock.Add(time.Millisecond)
				}

				out := make([]int, 0, 20)

				// ensure a reasonably random distribution below.
				rand.Seed(time.Now().UnixNano())

				for !b.Stats().Empty() {
					batch := b.LeaseOne(clock.Now(ctx))
					tclock.Add(time.Millisecond)

					if rand.Intn(2) == 0 {
						out = append(out, batch.Data[0].Item.(int))
						b.ACK(batch)
					} else {
						b.NACK(ctx, nil, batch)
					}
				}

				So(out, ShouldResemble, expect)
			})

			Convey(`batch size`, func() {
				b, err := NewBuffer[any](&Options{
					FullBehavior:  &BlockNewItems{MaxSize: 150},
					BatchItemsMax: -1,
					BatchSizeMax:  100,
				})
				So(err, ShouldBeNil)

				t0 := tclock.Now()

				Convey(`cuts previous batch when adding another item`, func() {
					must(addNoBlockStr(b, t0, "hello, this is a string which is 47 bytes long."))
					must(addNoBlockStr(b, t0, "hello, this is a string which is 47 bytes long."))

					// Cuts previous batch when adding a new item
					must(addNoBlockStr(b, t0, "hello, this is a string which is 47 bytes long."))

					Convey(`and cannot exceed FullBehavior`, func() {
						must(addNoBlockStr(b, t0, "this is a kind of long string")) // buffer is now past capacity.

						_, err := addNoBlockStr(b, t0, "boom time")
						So(err, ShouldErrLike, ErrBufferFull)
					})

					Convey(`we should see only two items when leasing a batch`, func() {
						// BlockNewItems accounts for addition of too-large item.
						batch := b.LeaseOne(t0)
						So(batch, ShouldNotBeNil)
						So(batch.Data, ShouldHaveLength, 2)
						b.ACK(batch)
					})
				})

				Convey(`cuts batch if it exactly equals size limit`, func() {
					// Batch is cut if it equals BatchSizeMax after insertion.
					must(addNoBlockStr(b, t0, "hello, this is a string which is 47 bytes long."))
					must(addNoBlockStr(b, t0, "hello, this is a longer string which is 53 bytes long"))

					batch := b.LeaseOne(t0)
					So(batch, ShouldNotBeNil)
					So(batch.Data, ShouldHaveLength, 2)
					So(batch.Data[1].Item, ShouldContainSubstring, "longer")
					b.ACK(batch)
				})

				Convey(`too-large items error`, func() {
					_, err := addNoBlockStr(b, t0, strings.Repeat("this is 21 chars long", 5))
					So(err, ShouldErrLike, ErrItemTooLarge)
				})

				Convey(`too-small items error`, func() {
					// note; it's up to the users to ensure they don't put zero-size
					// items in here. If they really wanted 'empty' items in here, they
					// should still assign them some arbitrary non-zero size (like 1).
					_, err := addNoBlockStr(b, t0, "")
					So(err, ShouldErrLike, ErrItemTooSmall)
				})

				Convey(`exactly-BatchSizeMax items do a double flush`, func() {
					must(addNoBlockStr(b, t0, "short stuff"))

					_, err := b.AddNoBlock(t0, "I'm lazy and lying about the size.", b.opts.BatchSizeMax)
					So(err, ShouldBeNil)
					batch := b.LeaseOne(t0)
					So(batch, ShouldNotBeNil)
					So(batch.Data, ShouldHaveLength, 1)
					So(batch.Data[0].Item, ShouldContainSubstring, "short stuff")
					b.ACK(batch)

					batch = b.LeaseOne(t0)
					So(batch, ShouldNotBeNil)
					So(batch.Data, ShouldHaveLength, 1)
					So(batch.Data[0].Item, ShouldContainSubstring, "lazy and lying")
					b.ACK(batch)
				})

				Convey(`NACK`, func() {
					Convey(`adds size back`, func() {
						must(addNoBlockStr(b, t0, "stuff"))

						b.Flush(t0)
						batch := b.LeaseOne(t0)
						So(b.Stats(), ShouldResemble, Stats{LeasedItemCount: 1, LeasedItemSize: 5})

						// can shrink the batch, too
						batch.Data[0].Size = 1
						b.NACK(ctx, nil, batch)
						So(b.Stats(), ShouldResemble, Stats{UnleasedItemCount: 1, UnleasedItemSize: 1})
					})
				})
			})

			Convey(`full buffer behavior`, func() {

				Convey(`DropOldestBatch`, func() {
					b, err := NewBuffer[any](&Options{
						FullBehavior:  &DropOldestBatch{MaxLiveItems: 1},
						BatchItemsMax: 1,
					})
					So(err, ShouldBeNil)

					So(must(addNoBlockZero(b, clock.Now(ctx), 0)), ShouldBeNil)
					So(must(addNoBlockZero(b, clock.Now(ctx), 1)), ShouldNotBeNil) // drops 0

					So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 1}) // full!
					So(b.CanAddItem(), ShouldBeTrue)

					Convey(`via new data`, func() {
						So(must(addNoBlockZero(b, clock.Now(ctx), 100)), ShouldNotBeNil) // drops 1
						So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 1})         // still full

						tclock.Set(b.NextSendTime())

						batch := b.LeaseOne(clock.Now(ctx))
						So(batch, ShouldNotBeNil)
						So(batch.Data, ShouldHaveLength, 1)
						So(batch.Data[0].Item, ShouldEqual, 100)
						So(batch.id, ShouldEqual, 3)
					})

					Convey(`via NACK`, func() {
						leased := b.LeaseOne(clock.Now(ctx))
						So(leased, ShouldNotBeNil)
						So(b.stats, ShouldResemble, Stats{LeasedItemCount: 1})

						dropped := must(addNoBlockZero(b, clock.Now(ctx), 100))
						So(dropped, ShouldResemble, leased)
						So(b.stats, ShouldResemble, Stats{
							UnleasedItemCount:      1,
							DroppedLeasedItemCount: 1,
						})

						dropped = must(addNoBlockZero(b, clock.Now(ctx), 200))
						So(dropped.Data[0].Item, ShouldResemble, 100)
						So(b.stats, ShouldResemble, Stats{
							UnleasedItemCount:      1,
							DroppedLeasedItemCount: 1,
						})

						b.NACK(ctx, nil, leased)

						So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 1})
					})
				})

				Convey(`DropOldestBatch dropping items from the current batch`, func() {
					b, err := NewBuffer[any](&Options{
						FullBehavior:  &DropOldestBatch{MaxLiveItems: 1},
						BatchItemsMax: -1,
					})
					So(err, ShouldBeNil)

					So(must(addNoBlockZero(b, clock.Now(ctx), 0)), ShouldBeNil)
					So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 1})
					So(must(addNoBlockZero(b, clock.Now(ctx), 1)), ShouldNotBeNil)
					So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 1})
				})

				Convey(`BlockNewItems`, func() {
					b, err := NewBuffer[any](&Options{
						FullBehavior:  &BlockNewItems{MaxItems: 21},
						BatchItemsMax: 20,
					})
					So(err, ShouldBeNil)

					for i := 0; i < int(b.opts.BatchItemsMax)+1; i++ {
						So(must(addNoBlockZero(b, clock.Now(ctx), i)), ShouldBeNil)
					}

					So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 21})
					So(b.CanAddItem(), ShouldBeFalse)

					Convey(`Adding more errors`, func() {
						_, err := addNoBlockZero(b, clock.Now(ctx), 100)
						So(err, ShouldErrLike, ErrBufferFull)
					})

					Convey(`Leasing a batch still rejects Adds`, func() {
						batch := b.LeaseOne(clock.Now(ctx))
						So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 1, LeasedItemCount: 20})

						So(b.CanAddItem(), ShouldBeFalse)
						_, err := addNoBlockZero(b, clock.Now(ctx), 100)
						So(err, ShouldErrLike, ErrBufferFull)

						Convey(`ACK`, func() {
							b.ACK(batch)
							So(b.CanAddItem(), ShouldBeTrue)
							So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 1})
							So(must(addNoBlockZero(b, clock.Now(ctx), 100)), ShouldBeNil)
							So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 2})
						})

						Convey(`partial NACK`, func() {
							batch.Data = batch.Data[:10]
							b.NACK(ctx, nil, batch)

							So(b.CanAddItem(), ShouldBeTrue)
							So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 11})

							for i := 0; i < 10; i++ {
								So(must(addNoBlockZero(b, clock.Now(ctx), 100+i)), ShouldBeNil)
							}

							So(b.CanAddItem(), ShouldBeFalse)
							So(b.LeaseOne(clock.Now(ctx)), ShouldBeNil) // no batch cut yet
							tclock.Set(b.NextSendTime())

							So(b.LeaseOne(clock.Now(ctx)), ShouldNotBeNil)
						})

						Convey(`NACK`, func() {
							b.NACK(ctx, nil, batch)
							So(b.stats, ShouldResemble, Stats{UnleasedItemCount: 21})
							So(b.CanAddItem(), ShouldBeFalse)
						})

					})

				})

			})
		})
	})
}
