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

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/retry"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBuffer(t *testing.T) {
	Convey(`Buffer`, t, func() {
		Convey(`construction`, func() {
			Convey(`success`, func() {
				b, err := NewBuffer(nil)
				So(err, ShouldBeNil)

				So(b.Stats(), ShouldResemble, Stats{})
				So(b.NextSendTime(), ShouldBeZeroValue)
				So(b.CanAddItem(), ShouldBeTrue)
				So(b.LeaseOne(time.Time{}), ShouldBeNil)
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
				b, err := NewBuffer(&Options{
					MaxLeases: 2,
					BatchSize: 20,
				})
				So(err, ShouldBeNil)
				nextSendTimeOffset := b.opts.BatchDuration

				Convey(`batch cut by count`, func() {
					for i := 0; i < int(b.opts.BatchSize); i++ {
						So(b.unleased.Len(), ShouldEqual, 0)
						if i > 0 {
							// The next send time should be when the current batch will be
							// forcibly cut due to BatchDuration.
							So(b.NextSendTime(), ShouldResemble, start.Add(nextSendTimeOffset))
						}
						So(b.AddNoBlock(clock.Now(ctx), i), ShouldBeNil)
					}

					So(b.stats, ShouldResemble, Stats{20, 0, 0})
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

					So(b.stats, ShouldResemble, Stats{0, 20, 0})
					So(batch.Data, ShouldHaveLength, b.opts.BatchSize)
					for i := range batch.Data {
						So(batch.Data[i], ShouldEqual, i)
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

						So(b.stats, ShouldResemble, Stats{10, 0, 0})
						So(b.unleased.Len(), ShouldEqual, 1)

						Convey(`Adding Data does nothing`, func() {
							// no batch yet; the one we NACK'd is sleeping
							So(b.LeaseOne(clock.Now(ctx)), ShouldBeNil)
							tclock.Set(b.NextSendTime())
							newBatch := b.LeaseOne(clock.Now(ctx))
							So(newBatch, ShouldNotBeNil)

							newBatch.Data = append(newBatch.Data, nil, nil, nil)

							b.NACK(ctx, nil, newBatch)

							So(b.stats, ShouldResemble, Stats{10, 0, 0})
							So(b.unleased.Len(), ShouldEqual, 1)
						})
					})

					Convey(`Full NACK`, func() {
						b.NACK(ctx, nil, batch)

						So(b.stats, ShouldResemble, Stats{20, 0, 0})
						So(b.unleased.Len(), ShouldEqual, 1)

						Convey(`double NACK panic`, func() {
							So(func() { b.NACK(ctx, nil, batch) }, ShouldPanicLike, "unknown *Batch")
							So(b.stats, ShouldResemble, Stats{20, 0, 0})
							So(b.unleased.Len(), ShouldEqual, 1)
						})
					})

					Convey(`Max leases limits LeaseOne`, func() {
						now := clock.Now(ctx)
						So(b.AddNoBlock(now, 10), ShouldBeNil)
						b.Flush(now)
						So(b.AddNoBlock(now, 20), ShouldBeNil)
						b.Flush(now)

						So(b.LeaseOne(clock.Now(ctx)), ShouldNotBeNil)

						// There's something to send
						So(b.NextSendTime(), ShouldNotBeZeroValue)

						// But lease limit of 2 is hit
						So(b.LeaseOne(clock.Now(ctx)), ShouldBeNil)
					})
				})

				Convey(`batch cut by time`, func() {
					So(b.AddNoBlock(clock.Now(ctx), "bobbie"), ShouldBeNil)

					// should be equal to timeout of first batch, plus 1ms
					nextSend := b.NextSendTime()
					So(nextSend, ShouldResemble, start.Add(nextSendTimeOffset))
					tclock.Set(nextSend)

					So(b.AddNoBlock(clock.Now(ctx), "charlie"), ShouldBeNil)
					So(b.AddNoBlock(clock.Now(ctx), "dakota"), ShouldBeNil)

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
					So(batch.Data, ShouldResemble, []interface{}{
						"bobbie", "charlie", "dakota",
					})

				})

				Convey(`batch cut by flush`, func() {
					So(b.AddNoBlock(clock.Now(ctx), "bobbie"), ShouldBeNil)
					So(b.stats, ShouldResemble, Stats{1, 0, 0})

					So(b.LeaseOne(clock.Now(ctx)), ShouldBeNil)

					b.Flush(start)
					So(b.stats, ShouldResemble, Stats{1, 0, 0})
					So(b.currentBatch, ShouldBeNil)
					So(b.unleased, ShouldHaveLength, 1)

					Convey(`double flush is noop`, func() {
						b.Flush(start)
						So(b.stats, ShouldResemble, Stats{1, 0, 0})
						So(b.currentBatch, ShouldBeNil)
						So(b.unleased, ShouldHaveLength, 1)
					})

					batch := b.LeaseOne(clock.Now(ctx))
					So(batch, ShouldNotBeNil)
					So(b.stats, ShouldResemble, Stats{0, 1, 0})

					b.ACK(batch)

					So(b.stats.Empty(), ShouldBeTrue)
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

				So(b.AddNoBlock(clock.Now(ctx), 1), ShouldBeNil)

				b.NACK(ctx, nil, b.LeaseOne(clock.Now(ctx)))
				So(b.stats, ShouldResemble, Stats{1, 0, 0})
				b.NACK(ctx, nil, b.LeaseOne(clock.Now(ctx)))
				// only one retry was allowed, start it's gone.
				So(b.stats, ShouldResemble, Stats{})
			})

			Convey(`full buffer behavior`, func() {

				Convey(`DropOldestBatch`, func() {
					b, err := NewBuffer(&Options{
						FullBehavior: &DropOldestBatch{1},
						BatchSize:    1,
					})
					So(err, ShouldBeNil)

					So(b.AddNoBlock(clock.Now(ctx), 0), ShouldBeNil)
					So(b.AddNoBlock(clock.Now(ctx), 1), ShouldNotBeNil) // drops 0

					So(b.stats, ShouldResemble, Stats{1, 0, 0}) // full!
					So(b.CanAddItem(), ShouldBeTrue)

					Convey(`via new data`, func() {
						So(b.AddNoBlock(clock.Now(ctx), 100), ShouldNotBeNil) // drops 1
						So(b.stats, ShouldResemble, Stats{1, 0, 0})           // still full

						tclock.Set(b.NextSendTime())

						batch := b.LeaseOne(clock.Now(ctx))
						So(batch, ShouldNotBeNil)
						So(batch.Data, ShouldHaveLength, 1)
						So(batch.Data[0], ShouldEqual, 100)
						So(batch.id, ShouldEqual, 3)
					})

					Convey(`via NACK`, func() {
						leased := b.LeaseOne(clock.Now(ctx))
						So(leased, ShouldNotBeNil)
						So(b.stats, ShouldResemble, Stats{0, 1, 0})

						dropped := b.AddNoBlock(clock.Now(ctx), 100)
						So(dropped, ShouldResemble, leased)
						So(b.stats, ShouldResemble, Stats{1, 0, 1})

						dropped = b.AddNoBlock(clock.Now(ctx), 200)
						So(dropped.Data, ShouldResemble, []interface{}{100})
						So(b.stats, ShouldResemble, Stats{1, 0, 1})

						b.NACK(ctx, nil, leased)

						So(b.stats, ShouldResemble, Stats{1, 0, 0})
					})
				})

				Convey(`DropOldestBatch dropping items from the current batch`, func() {
					b, err := NewBuffer(&Options{
						FullBehavior: &DropOldestBatch{1},
						BatchSize:    -1,
					})
					So(err, ShouldBeNil)

					So(b.AddNoBlock(clock.Now(ctx), 0), ShouldBeNil)
					So(b.stats, ShouldResemble, Stats{1, 0, 0})
					So(b.AddNoBlock(clock.Now(ctx), 1), ShouldNotBeNil)
					So(b.stats, ShouldResemble, Stats{1, 0, 0})
				})

				Convey(`BlockNewItems`, func() {
					b, err := NewBuffer(&Options{
						FullBehavior: &BlockNewItems{21},
						BatchSize:    20,
					})
					So(err, ShouldBeNil)

					for i := 0; i < int(b.opts.BatchSize)+1; i++ {
						So(b.AddNoBlock(clock.Now(ctx), i), ShouldBeNil)
					}

					So(b.stats, ShouldResemble, Stats{21, 0, 0})
					So(b.CanAddItem(), ShouldBeFalse)

					Convey(`Adding more panics`, func() {
						So(func() { b.AddNoBlock(clock.Now(ctx), 100) }, ShouldPanicLike, "buffer is full")
					})

					Convey(`Leasing a batch still rejects Adds`, func() {
						batch := b.LeaseOne(clock.Now(ctx))
						So(b.stats, ShouldResemble, Stats{1, 20, 0})

						So(b.CanAddItem(), ShouldBeFalse)
						So(func() { b.AddNoBlock(clock.Now(ctx), 100) }, ShouldPanic)

						Convey(`ACK`, func() {
							b.ACK(batch)
							So(b.CanAddItem(), ShouldBeTrue)
							So(b.stats, ShouldResemble, Stats{1, 0, 0})
							So(b.AddNoBlock(clock.Now(ctx), 100), ShouldBeNil)
							So(b.stats, ShouldResemble, Stats{2, 0, 0})
						})

						Convey(`partial NACK`, func() {
							batch.Data = batch.Data[:10]
							b.NACK(ctx, nil, batch)

							So(b.CanAddItem(), ShouldBeTrue)
							So(b.stats, ShouldResemble, Stats{11, 0, 0})

							for i := 0; i < 10; i++ {
								So(b.AddNoBlock(clock.Now(ctx), 100+i), ShouldBeNil)
							}

							So(b.CanAddItem(), ShouldBeFalse)
							So(b.LeaseOne(clock.Now(ctx)), ShouldBeNil) // no batch cut yet
							tclock.Set(b.NextSendTime())

							So(b.LeaseOne(clock.Now(ctx)), ShouldNotBeNil)
						})

						Convey(`NACK`, func() {
							b.NACK(ctx, nil, batch)
							So(b.stats, ShouldResemble, Stats{21, 0, 0})
							So(b.CanAddItem(), ShouldBeFalse)
						})

					})

				})

			})
		})
	})
}
