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
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/retry"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBuffer(t *testing.T) {
	t.Parallel()

	Convey(`Buffer`, t, func() {
		Convey(`construction`, func() {
			Convey(`success`, func() {
				b, err := NewBuffer(nil)
				So(err, ShouldBeNil)

				So(b.Len(), ShouldEqual, 0)
				So(b.NextSendTime(), ShouldBeZeroValue)
				So(b.CanAddItem(), ShouldBeTrue)
				So(b.LeaseOne(context.Background()), ShouldBeNil)
			})
			Convey(`fail`, func() {
				_, err := NewBuffer(&Options{BatchSize: -100})
				So(err, ShouldErrLike, "normalizing buffer.Options")
			})
		})

		Convey(`usage`, func() {
			start := testclock.TestRecentTimeUTC
			ctx, tclock := testclock.UseTime(context.Background(), start)

			Convey(`common behavior`, func() {
				b, err := NewBuffer(nil)
				So(err, ShouldBeNil)
				bi := b.(*bufferImpl)

				Convey(`batch cut by count`, func() {
					for i := 0; i < Defaults.BatchSize; i++ {
						So(bi.heap.Len(), ShouldEqual, 0)
						if i > 0 {
							// The next send time should be when the current batch will be
							// forcibly cut due to BatchDuration.
							So(b.NextSendTime(), ShouldResemble,
								start.Add(Defaults.BatchDuration+time.Millisecond))
						}
						b.AddNoBlock(ctx, i)
					}

					So(b.Len(), ShouldEqual, Defaults.BatchSize)
					// The next send time should be when the current batch is available to
					// send. Since this is a test and time hasn't advanced, it's reset
					// back to the start time.
					So(b.NextSendTime(), ShouldEqual, start)
					So(b.CanAddItem(), ShouldBeTrue)
					So(bi.heap.Len(), ShouldEqual, 1)
					So(bi.currentBatch, ShouldBeNil)

					batch := b.LeaseOne(ctx)
					So(b.LeaseOne(ctx), ShouldBeNil)

					So(b.Len(), ShouldEqual, Defaults.BatchSize)
					So(batch.Data, ShouldHaveLength, Defaults.BatchSize)
					for i := range batch.Data {
						So(batch.Data[i], ShouldEqual, i)
					}

					Convey(`ACK`, func() {
						batch.ACK()

						So(b.Len(), ShouldEqual, 0)

						Convey(`double ACK panic`, func() {
							So(batch.ACK, ShouldPanicLike, "LeasedBatch")
							So(b.Len(), ShouldEqual, 0)
						})
					})

					Convey(`Partial NACK`, func() {
						batch.Data = batch.Data[:10] // pretend we processed some Data

						batch.NACK(ctx, nil)

						So(b.Len(), ShouldEqual, Defaults.BatchSize-10)
						So(bi.heap.Len(), ShouldEqual, 1)

						Convey(`Adding Data does nothing`, func() {
							// no batch yet; the one we NACK'd is sleeping
							So(b.LeaseOne(ctx), ShouldBeNil)
							tclock.Set(b.NextSendTime())
							batch := b.LeaseOne(ctx)
							So(batch, ShouldNotBeNil)

							batch.Data = append(batch.Data, nil, nil, nil)

							batch.NACK(ctx, nil)

							So(b.Len(), ShouldEqual, Defaults.BatchSize-10)
							So(bi.heap.Len(), ShouldEqual, 1)
						})
					})

					Convey(`Full NACK`, func() {
						batch.NACK(ctx, nil)

						So(b.Len(), ShouldEqual, Defaults.BatchSize)
						So(bi.heap.Len(), ShouldEqual, 1)

						Convey(`double NACK panic`, func() {
							So(func() { batch.NACK(ctx, nil) }, ShouldPanicLike, "LeasedBatch")
							So(b.Len(), ShouldEqual, Defaults.BatchSize)
							So(bi.heap.Len(), ShouldEqual, 1)
						})
					})
				})

				Convey(`batch cut by time`, func() {
					b.AddNoBlock(ctx, "bobbie")

					// should be equal to timeout of first batch, plus 1ms
					nextSend := b.NextSendTime()
					So(nextSend, ShouldResemble,
						start.Add(Defaults.BatchDuration+time.Millisecond))
					tclock.Set(nextSend)

					b.AddNoBlock(ctx, "charlie")
					b.AddNoBlock(ctx, "dakota")

					// We haven't leased one yet, so NextSendTime should stay the same.
					So(b.NextSendTime(), ShouldResemble,
						start.Add(Defaults.BatchDuration+time.Millisecond))

					batch1 := b.LeaseOne(ctx)
					So(batch1, ShouldNotBeNil)

					// Now it's an extra iteration in the future.
					So(b.NextSendTime(), ShouldResemble,
						start.Add(2*(Defaults.BatchDuration+time.Millisecond)))
					tclock.Set(b.NextSendTime())

					So(batch1.Data, ShouldHaveLength, 1)
					So(batch1.Data[0], ShouldEqual, "bobbie")

					batch2 := b.LeaseOne(ctx)
					So(batch2, ShouldNotBeNil)

					So(batch2.Data, ShouldHaveLength, 2)
					So(batch2.Data[0], ShouldEqual, "charlie")
					So(batch2.Data[1], ShouldEqual, "dakota")
				})
			})

			Convey(`retry limit eventually drops batch`, func() {
				b, err := NewBuffer(&Options{
					BatchSize: 1,
					Retry: func() retry.Iterator {
						return &retry.Limited{Retries: 1}
					},
				})
				So(err, ShouldBeNil)

				b.AddNoBlock(ctx, 1)

				b.LeaseOne(ctx).NACK(ctx, nil)
				So(b.Len(), ShouldEqual, 1)
				b.LeaseOne(ctx).NACK(ctx, nil)
				// only one retry was allowed, now it's gone.
				So(b.Len(), ShouldEqual, 0)
			})

			Convey(`full buffer behavior`, func() {

				Convey(`DropOldestBatch`, func() {
					b, err := NewBuffer(&Options{
						FullBehavior: DropOldestBatch,
						MaxItems:     Defaults.BatchSize,
					})
					So(err, ShouldBeNil)

					for i := 0; i < Defaults.BatchSize; i++ {
						b.AddNoBlock(ctx, i)
					}

					So(b.Len(), ShouldEqual, Defaults.BatchSize)
					So(b.CanAddItem(), ShouldBeTrue)

					Convey(`via new data`, func() {
						So(b.Len(), ShouldEqual, Defaults.BatchSize)
						b.AddNoBlock(ctx, 100)
						So(b.Len(), ShouldEqual, 1)

						tclock.Set(b.NextSendTime())

						batch := b.LeaseOne(ctx)
						So(batch, ShouldNotBeNil)
						So(batch.Data, ShouldHaveLength, 1)
						So(batch.Data[0], ShouldEqual, 100)
						So(batch.id, ShouldEqual, 2)
					})

					Convey(`via NACK`, func() {
						batch := b.LeaseOne(ctx)
						So(batch, ShouldNotBeNil)

						b.AddNoBlock(ctx, 100)

						// We exceed MaxItems because the outstanding batch still counts.
						So(b.Len(), ShouldEqual, 21)

						// try to re-queue the batch, but it gets dropped.
						batch.NACK(ctx, nil)

						So(b.Len(), ShouldEqual, 1) // it's gone
					})
				})

				Convey(`BlockNewItems`, func() {
					b, err := NewBuffer(&Options{
						FullBehavior: BlockNewItems,
						MaxItems:     Defaults.BatchSize,
					})
					So(err, ShouldBeNil)

					for i := 0; i < Defaults.BatchSize; i++ {
						b.AddNoBlock(ctx, i)
					}

					So(b.Len(), ShouldEqual, Defaults.BatchSize)
					So(b.CanAddItem(), ShouldBeFalse)

					Convey(`Adding more panics`, func() {
						So(func() { b.AddNoBlock(ctx, 100) }, ShouldPanicLike, "buffer is full")
					})

					Convey(`Leasing a batch still rejects Adds`, func() {
						batch := b.LeaseOne(ctx)

						So(b.CanAddItem(), ShouldBeFalse)
						So(func() { b.AddNoBlock(ctx, 100) }, ShouldPanic)

						Convey(`ACK`, func() {
							batch.ACK()
							So(b.CanAddItem(), ShouldBeTrue)
							So(b.Len(), ShouldEqual, 0)
							b.AddNoBlock(ctx, 100)
						})

						Convey(`partial NACK`, func() {
							batch.Data = batch.Data[:10]
							batch.NACK(ctx, nil)

							So(b.CanAddItem(), ShouldBeTrue)
							So(b.Len(), ShouldEqual, Defaults.BatchSize-10)

							for i := 0; i < 10; i++ {
								b.AddNoBlock(ctx, 100+i)
							}

							So(b.CanAddItem(), ShouldBeFalse)
							So(b.LeaseOne(ctx), ShouldBeNil) // no batch cut yet
							tclock.Set(b.NextSendTime())

							So(b.LeaseOne(ctx), ShouldNotBeNil)
						})

						Convey(`NACK`, func() {
							batch.NACK(ctx, nil)
							So(b.Len(), ShouldEqual, Defaults.BatchSize)
							So(b.CanAddItem(), ShouldBeFalse)
						})

					})

				})

			})
		})
	})
}
