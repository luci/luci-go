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

	"github.com/google/go-cmp/cmp"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/registry"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func init() {
	registry.RegisterCmpOption(cmp.AllowUnexported(retry.Limited{}))
}

func TestBuffer(t *testing.T) {
	must := func(b *Batch[any], err error) *Batch[any] {
		assert.Loosely(t, err, should.BeNil)
		return b
	}
	addNoBlockZero := func(b *Buffer[any], now time.Time, item any) (*Batch[any], error) {
		return b.AddNoBlock(now, item, 0)
	}
	addNoBlockStr := func(b *Buffer[any], now time.Time, item string) (*Batch[any], error) {
		return b.AddNoBlock(now, item, len(item))
	}

	ftt.Run(`Buffer`, t, func(t *ftt.Test) {
		t.Run(`construction`, func(t *ftt.Test) {
			t.Run(`success`, func(t *ftt.Test) {
				b, err := NewBuffer[any](nil)
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, b.Stats(), should.Resemble(Stats{}))
				assert.Loosely(t, b.NextSendTime(), should.BeZero)
				assert.Loosely(t, b.CanAddItem(), should.BeTrue)
				assert.Loosely(t, b.LeaseOne(time.Time{}), should.BeNil)
			})
			t.Run(`fail`, func(t *ftt.Test) {
				_, err := NewBuffer[any](&Options{BatchItemsMax: -100})
				assert.Loosely(t, err, should.ErrLike("normalizing buffer.Options"))
			})
		})

		t.Run(`usage`, func(t *ftt.Test) {
			start := testclock.TestRecentTimeUTC
			ctx, tclock := testclock.UseTime(context.Background(), start)

			t.Run(`common behavior`, func(t *ftt.Test) {
				b, err := NewBuffer[any](&Options{
					MaxLeases:     2,
					BatchItemsMax: 20,
				})
				assert.Loosely(t, err, should.BeNil)
				nextSendTimeOffset := b.opts.BatchAgeMax

				t.Run(`batch cut by count`, func(t *ftt.Test) {
					for i := 0; i < int(b.opts.BatchItemsMax); i++ {
						assert.Loosely(t, b.unleased.Len(), should.BeZero)
						if i > 0 {
							// The next send time should be when the current batch will be
							// forcibly cut due to BatchAgeMax.
							assert.Loosely(t, b.NextSendTime(), should.Resemble(start.Add(nextSendTimeOffset)))
						}
						assert.Loosely(t, must(addNoBlockZero(b, clock.Now(ctx), i)), should.BeNil)
					}

					assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 20}))
					assert.Loosely(t, b.Stats().Total(), should.Match(20))
					assert.Loosely(t, b.Stats().Empty(), should.Resemble(false))
					// The next send time should be when the current batch is available to
					// send. this is a test and time hasn't advanced, it's reset
					// back to the start time.
					assert.Loosely(t, b.NextSendTime(), should.Match(start))
					assert.Loosely(t, b.CanAddItem(), should.BeTrue)
					assert.Loosely(t, b.unleased.Len(), should.Equal(1))
					assert.Loosely(t, b.currentBatch, should.BeNil)

					batch := b.LeaseOne(clock.Now(ctx))
					assert.Loosely(t, b.LeaseOne(clock.Now(ctx)), should.BeNil)

					assert.Loosely(t, b.stats, should.Resemble(Stats{LeasedItemCount: 20}))
					assert.Loosely(t, batch.Data, should.HaveLength(b.opts.BatchItemsMax))
					for i := range batch.Data {
						assert.Loosely(t, batch.Data[i].Item, should.Equal(i))
					}

					t.Run(`ACK`, func(t *ftt.Test) {
						b.ACK(batch)

						assert.Loosely(t, b.stats, should.Resemble(Stats{}))
						assert.Loosely(t, b.Stats().Total(), should.BeZero)
						assert.Loosely(t, b.Stats().Empty(), should.Resemble(true))

						t.Run(`double ACK panic`, func(t *ftt.Test) {
							assert.Loosely(t, func() { b.ACK(batch) }, should.PanicLike("unknown *Batch"))
							assert.Loosely(t, b.stats, should.Resemble(Stats{}))
						})
						t.Run(`nil ACK panic`, func(t *ftt.Test) {
							assert.Loosely(t, func() { b.ACK(nil) }, should.PanicLike("called with `nil`"))
						})
					})

					t.Run(`Partial NACK`, func(t *ftt.Test) {
						batch.Data = batch.Data[:10] // pretend we processed some Data

						b.NACK(ctx, nil, batch)

						assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 10}))
						assert.Loosely(t, b.unleased.Len(), should.Equal(1))

						t.Run(`Adding Data does nothing`, func(t *ftt.Test) {
							// no batch yet; the one we NACK'd is sleeping
							assert.Loosely(t, b.LeaseOne(clock.Now(ctx)), should.BeNil)
							tclock.Set(b.NextSendTime())
							newBatch := b.LeaseOne(clock.Now(ctx))
							assert.Loosely(t, newBatch, should.NotBeNil)

							newBatch.Data = append(newBatch.Data, BatchItem[any]{}, BatchItem[any]{}, BatchItem[any]{})

							b.NACK(ctx, nil, newBatch)

							assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 10}))
							assert.Loosely(t, b.unleased.Len(), should.Equal(1))
						})
					})

					t.Run(`Full NACK`, func(t *ftt.Test) {
						b.NACK(ctx, nil, batch)

						assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 20}))
						assert.Loosely(t, b.unleased.Len(), should.Equal(1))

						t.Run(`double NACK panic`, func(t *ftt.Test) {
							assert.Loosely(t, func() { b.NACK(ctx, nil, batch) }, should.PanicLike("unknown *Batch"))
							assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 20}))
							assert.Loosely(t, b.unleased.Len(), should.Equal(1))
						})
					})

					t.Run(`Max leases limits LeaseOne`, func(t *ftt.Test) {
						now := clock.Now(ctx)
						assert.Loosely(t, must(addNoBlockZero(b, now, 10)), should.BeNil)
						b.Flush(now)
						assert.Loosely(t, must(addNoBlockZero(b, now, 20)), should.BeNil)
						b.Flush(now)

						assert.Loosely(t, b.LeaseOne(clock.Now(ctx)), should.NotBeNil)

						// There's something to send
						assert.Loosely(t, b.NextSendTime(), should.NotBeZero)

						// But lease limit of 2 is hit
						assert.Loosely(t, b.LeaseOne(clock.Now(ctx)), should.BeNil)
					})

					t.Run(`nil NACK panic`, func(t *ftt.Test) {
						assert.Loosely(t, func() { b.NACK(ctx, nil, nil) }, should.PanicLike("called with `nil`"))
					})
				})

				t.Run(`batch cut by time`, func(t *ftt.Test) {
					assert.Loosely(t, must(addNoBlockZero(b, clock.Now(ctx), "bobbie")), should.BeNil)

					// should be equal to timeout of first batch, plus 1ms
					nextSend := b.NextSendTime()
					assert.Loosely(t, nextSend, should.Resemble(start.Add(nextSendTimeOffset)))
					tclock.Set(nextSend)

					assert.Loosely(t, must(addNoBlockZero(b, clock.Now(ctx), "charlie")), should.BeNil)
					assert.Loosely(t, must(addNoBlockZero(b, clock.Now(ctx), "dakota")), should.BeNil)

					// We haven't leased one yet, so NextSendTime should stay the same.
					nextSend = b.NextSendTime()
					assert.Loosely(t, nextSend, should.Resemble(start.Add(nextSendTimeOffset)))

					// Eventually time passes, and we can lease the batch.
					tclock.Set(nextSend)
					batch := b.LeaseOne(clock.Now(ctx))
					assert.Loosely(t, batch, should.NotBeNil)

					// that batch included everything
					assert.Loosely(t, b.NextSendTime(), should.BeZero)

					assert.Loosely(t, batch.Data, should.HaveLength(3))
					assert.Loosely(t, batch.Data[0].Item, should.Match("bobbie"))
					assert.Loosely(t, batch.Data[1].Item, should.Match("charlie"))
					assert.Loosely(t, batch.Data[2].Item, should.Match("dakota"))
				})

				t.Run(`batch cut by flush`, func(t *ftt.Test) {
					assert.Loosely(t, must(addNoBlockZero(b, clock.Now(ctx), "bobbie")), should.BeNil)
					assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 1}))

					assert.Loosely(t, b.LeaseOne(clock.Now(ctx)), should.BeNil)

					b.Flush(start)
					assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 1}))
					assert.Loosely(t, b.currentBatch, should.BeNil)
					assert.Loosely(t, b.unleased.data, should.HaveLength(1))

					t.Run(`double flush is noop`, func(t *ftt.Test) {
						b.Flush(start)
						assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 1}))
						assert.Loosely(t, b.currentBatch, should.BeNil)
						assert.Loosely(t, b.unleased.data, should.HaveLength(1))
					})

					batch := b.LeaseOne(clock.Now(ctx))
					assert.Loosely(t, batch, should.NotBeNil)
					assert.Loosely(t, b.stats, should.Resemble(Stats{LeasedItemCount: 1}))

					b.ACK(batch)

					assert.Loosely(t, b.stats.Empty(), should.BeTrue)
				})
			})

			t.Run(`retry limit eventually drops batch`, func(t *ftt.Test) {
				b, err := NewBuffer[any](&Options{
					BatchItemsMax: 1,
					Retry: func() retry.Iterator {
						return &retry.Limited{Retries: 1}
					},
				})
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, must(addNoBlockZero(b, clock.Now(ctx), 1)), should.BeNil)

				b.NACK(ctx, nil, b.LeaseOne(clock.Now(ctx)))
				assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 1}))
				b.NACK(ctx, nil, b.LeaseOne(clock.Now(ctx)))
				// only one retry was allowed, start it's gone.
				assert.Loosely(t, b.stats, should.Resemble(Stats{}))
			})

			t.Run(`in-order delivery`, func(t *ftt.Test) {
				b, err := NewBuffer[any](&Options{
					MaxLeases:     1,
					BatchItemsMax: 1,
					FullBehavior:  InfiniteGrowth{},
					FIFO:          true,
					Retry: func() retry.Iterator {
						return &retry.Limited{Retries: -1}
					},
				})
				assert.Loosely(t, err, should.BeNil)

				expect := make([]int, 20)
				for i := 0; i < 20; i++ {
					expect[i] = i
					assert.Loosely(t, must(addNoBlockZero(b, clock.Now(ctx), i)), should.BeNil)
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

				assert.Loosely(t, out, should.Resemble(expect))
			})

			t.Run(`batch size`, func(t *ftt.Test) {
				b, err := NewBuffer[any](&Options{
					FullBehavior:  &BlockNewItems{MaxSize: 150},
					BatchItemsMax: -1,
					BatchSizeMax:  100,
				})
				assert.Loosely(t, err, should.BeNil)

				t0 := tclock.Now()

				t.Run(`cuts previous batch when adding another item`, func(t *ftt.Test) {
					must(addNoBlockStr(b, t0, "hello, this is a string which is 47 bytes long."))
					must(addNoBlockStr(b, t0, "hello, this is a string which is 47 bytes long."))

					// Cuts previous batch when adding a new item
					must(addNoBlockStr(b, t0, "hello, this is a string which is 47 bytes long."))

					t.Run(`and cannot exceed FullBehavior`, func(t *ftt.Test) {
						must(addNoBlockStr(b, t0, "this is a kind of long string")) // buffer is now past capacity.

						_, err := addNoBlockStr(b, t0, "boom time")
						assert.Loosely(t, err, should.ErrLike(ErrBufferFull))
					})

					t.Run(`we should see only two items when leasing a batch`, func(t *ftt.Test) {
						// BlockNewItems accounts for addition of too-large item.
						batch := b.LeaseOne(t0)
						assert.Loosely(t, batch, should.NotBeNil)
						assert.Loosely(t, batch.Data, should.HaveLength(2))
						b.ACK(batch)
					})
				})

				t.Run(`cuts batch if it exactly equals size limit`, func(t *ftt.Test) {
					// Batch is cut if it equals BatchSizeMax after insertion.
					must(addNoBlockStr(b, t0, "hello, this is a string which is 47 bytes long."))
					must(addNoBlockStr(b, t0, "hello, this is a longer string which is 53 bytes long"))

					batch := b.LeaseOne(t0)
					assert.Loosely(t, batch, should.NotBeNil)
					assert.Loosely(t, batch.Data, should.HaveLength(2))
					assert.Loosely(t, batch.Data[1].Item, should.ContainSubstring("longer"))
					b.ACK(batch)
				})

				t.Run(`too-large items error`, func(t *ftt.Test) {
					_, err := addNoBlockStr(b, t0, strings.Repeat("this is 21 chars long", 5))
					assert.Loosely(t, err, should.ErrLike(ErrItemTooLarge))
				})

				t.Run(`too-small items error`, func(t *ftt.Test) {
					// note; it's up to the users to ensure they don't put zero-size
					// items in here. If they really wanted 'empty' items in here, they
					// should still assign them some arbitrary non-zero size (like 1).
					_, err := addNoBlockStr(b, t0, "")
					assert.Loosely(t, err, should.ErrLike(ErrItemTooSmall))
				})

				t.Run(`exactly-BatchSizeMax items do a double flush`, func(t *ftt.Test) {
					must(addNoBlockStr(b, t0, "short stuff"))

					_, err := b.AddNoBlock(t0, "I'm lazy and lying about the size.", b.opts.BatchSizeMax)
					assert.Loosely(t, err, should.BeNil)
					batch := b.LeaseOne(t0)
					assert.Loosely(t, batch, should.NotBeNil)
					assert.Loosely(t, batch.Data, should.HaveLength(1))
					assert.Loosely(t, batch.Data[0].Item, should.ContainSubstring("short stuff"))
					b.ACK(batch)

					batch = b.LeaseOne(t0)
					assert.Loosely(t, batch, should.NotBeNil)
					assert.Loosely(t, batch.Data, should.HaveLength(1))
					assert.Loosely(t, batch.Data[0].Item, should.ContainSubstring("lazy and lying"))
					b.ACK(batch)
				})

				t.Run(`NACK`, func(t *ftt.Test) {
					t.Run(`adds size back`, func(t *ftt.Test) {
						must(addNoBlockStr(b, t0, "stuff"))

						b.Flush(t0)
						batch := b.LeaseOne(t0)
						assert.Loosely(t, b.Stats(), should.Resemble(Stats{LeasedItemCount: 1, LeasedItemSize: 5}))

						// can shrink the batch, too
						batch.Data[0].Size = 1
						b.NACK(ctx, nil, batch)
						assert.Loosely(t, b.Stats(), should.Resemble(Stats{UnleasedItemCount: 1, UnleasedItemSize: 1}))
					})
				})
			})

			t.Run(`full buffer behavior`, func(t *ftt.Test) {

				t.Run(`DropOldestBatch`, func(t *ftt.Test) {
					b, err := NewBuffer[any](&Options{
						FullBehavior:  &DropOldestBatch{MaxLiveItems: 1},
						BatchItemsMax: 1,
					})
					assert.Loosely(t, err, should.BeNil)

					assert.Loosely(t, must(addNoBlockZero(b, clock.Now(ctx), 0)), should.BeNil)
					assert.Loosely(t, must(addNoBlockZero(b, clock.Now(ctx), 1)), should.NotBeNil) // drops 0

					assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 1})) // full!
					assert.Loosely(t, b.CanAddItem(), should.BeTrue)

					t.Run(`via new data`, func(t *ftt.Test) {
						assert.Loosely(t, must(addNoBlockZero(b, clock.Now(ctx), 100)), should.NotBeNil) // drops 1
						assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 1}))         // still full

						tclock.Set(b.NextSendTime())

						batch := b.LeaseOne(clock.Now(ctx))
						assert.Loosely(t, batch, should.NotBeNil)
						assert.Loosely(t, batch.Data, should.HaveLength(1))
						assert.Loosely(t, batch.Data[0].Item, should.Equal(100))
						assert.Loosely(t, batch.id, should.Equal(uint64(3)))
					})

					t.Run(`via NACK`, func(t *ftt.Test) {
						leased := b.LeaseOne(clock.Now(ctx))
						assert.Loosely(t, leased, should.NotBeNil)
						assert.Loosely(t, b.stats, should.Resemble(Stats{LeasedItemCount: 1}))

						dropped := must(addNoBlockZero(b, clock.Now(ctx), 100))
						assert.Loosely(t, dropped, should.Resemble(leased))
						assert.Loosely(t, b.stats, should.Resemble(Stats{
							UnleasedItemCount:      1,
							DroppedLeasedItemCount: 1,
						}))

						dropped = must(addNoBlockZero(b, clock.Now(ctx), 200))
						assert.Loosely(t, dropped.Data[0].Item, should.Match(100))
						assert.Loosely(t, b.stats, should.Resemble(Stats{
							UnleasedItemCount:      1,
							DroppedLeasedItemCount: 1,
						}))

						b.NACK(ctx, nil, leased)

						assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 1}))
					})
				})

				t.Run(`DropOldestBatch dropping items from the current batch`, func(t *ftt.Test) {
					b, err := NewBuffer[any](&Options{
						FullBehavior:  &DropOldestBatch{MaxLiveItems: 1},
						BatchItemsMax: -1,
					})
					assert.Loosely(t, err, should.BeNil)

					assert.Loosely(t, must(addNoBlockZero(b, clock.Now(ctx), 0)), should.BeNil)
					assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 1}))
					assert.Loosely(t, must(addNoBlockZero(b, clock.Now(ctx), 1)), should.NotBeNil)
					assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 1}))
				})

				t.Run(`BlockNewItems`, func(t *ftt.Test) {
					b, err := NewBuffer[any](&Options{
						FullBehavior:  &BlockNewItems{MaxItems: 21},
						BatchItemsMax: 20,
					})
					assert.Loosely(t, err, should.BeNil)

					for i := 0; i < int(b.opts.BatchItemsMax)+1; i++ {
						assert.Loosely(t, must(addNoBlockZero(b, clock.Now(ctx), i)), should.BeNil)
					}

					assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 21}))
					assert.Loosely(t, b.CanAddItem(), should.BeFalse)

					t.Run(`Adding more errors`, func(t *ftt.Test) {
						_, err := addNoBlockZero(b, clock.Now(ctx), 100)
						assert.Loosely(t, err, should.ErrLike(ErrBufferFull))
					})

					t.Run(`Leasing a batch still rejects Adds`, func(t *ftt.Test) {
						batch := b.LeaseOne(clock.Now(ctx))
						assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 1, LeasedItemCount: 20}))

						assert.Loosely(t, b.CanAddItem(), should.BeFalse)
						_, err := addNoBlockZero(b, clock.Now(ctx), 100)
						assert.Loosely(t, err, should.ErrLike(ErrBufferFull))

						t.Run(`ACK`, func(t *ftt.Test) {
							b.ACK(batch)
							assert.Loosely(t, b.CanAddItem(), should.BeTrue)
							assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 1}))
							assert.Loosely(t, must(addNoBlockZero(b, clock.Now(ctx), 100)), should.BeNil)
							assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 2}))
						})

						t.Run(`partial NACK`, func(t *ftt.Test) {
							batch.Data = batch.Data[:10]
							b.NACK(ctx, nil, batch)

							assert.Loosely(t, b.CanAddItem(), should.BeTrue)
							assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 11}))

							for i := 0; i < 10; i++ {
								assert.Loosely(t, must(addNoBlockZero(b, clock.Now(ctx), 100+i)), should.BeNil)
							}

							assert.Loosely(t, b.CanAddItem(), should.BeFalse)
							assert.Loosely(t, b.LeaseOne(clock.Now(ctx)), should.BeNil) // no batch cut yet
							tclock.Set(b.NextSendTime())

							assert.Loosely(t, b.LeaseOne(clock.Now(ctx)), should.NotBeNil)
						})

						t.Run(`NACK`, func(t *ftt.Test) {
							b.NACK(ctx, nil, batch)
							assert.Loosely(t, b.stats, should.Resemble(Stats{UnleasedItemCount: 21}))
							assert.Loosely(t, b.CanAddItem(), should.BeFalse)
						})

					})

				})

			})
		})
	})
}
