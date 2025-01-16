// Copyright 2024 The LUCI Authors.
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

package cursor

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/secrets"

	"go.chromium.org/luci/swarming/server/cursor/cursorpb"
)

func TestCursor(t *testing.T) {
	t.Parallel()

	ftt.Run("With key", t, func(t *ftt.Test) {
		ctx := context.Background()
		ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

		t.Run("OpaqueCursor", func(t *ftt.Test) {
			for _, kind := range []cursorpb.RequestKind{
				cursorpb.RequestKind_LIST_BOT_EVENTS,
				cursorpb.RequestKind_LIST_BOT_TASKS,
			} {
				cur, err := Encode(ctx, kind, &cursorpb.OpaqueCursor{
					Cursor: []byte("hello"),
				})
				assert.NoErr(t, err)

				yes, err := IsValidCursor(ctx, cur)
				assert.NoErr(t, err)
				assert.Loosely(t, yes, should.BeTrue)

				dec, err := Decode[cursorpb.OpaqueCursor](ctx, kind, cur)
				assert.NoErr(t, err)
				assert.Loosely(t, string(dec.Cursor), should.Equal("hello"))

				assert.Loosely(t, func() { _, _ = Encode(ctx, kind, &cursorpb.BotsCursor{}) }, should.Panic)
				assert.Loosely(t, func() { _, _ = Decode[cursorpb.BotsCursor](ctx, kind, cur) }, should.Panic)
			}
		})

		t.Run("BotsCursor", func(t *ftt.Test) {
			for _, kind := range []cursorpb.RequestKind{
				cursorpb.RequestKind_LIST_BOTS,
			} {
				cur, err := Encode(ctx, kind, &cursorpb.BotsCursor{
					LastBotId: "hello",
				})
				assert.NoErr(t, err)

				yes, err := IsValidCursor(ctx, cur)
				assert.NoErr(t, err)
				assert.Loosely(t, yes, should.BeTrue)

				dec, err := Decode[cursorpb.BotsCursor](ctx, kind, cur)
				assert.NoErr(t, err)
				assert.Loosely(t, dec.LastBotId, should.Equal("hello"))

				assert.Loosely(t, func() { _, _ = Encode(ctx, kind, &cursorpb.TasksCursor{}) }, should.Panic)
				assert.Loosely(t, func() { _, _ = Decode[cursorpb.TasksCursor](ctx, kind, cur) }, should.Panic)
			}
		})

		t.Run("TasksCursor", func(t *ftt.Test) {
			for _, kind := range []cursorpb.RequestKind{
				cursorpb.RequestKind_LIST_TASKS,
				cursorpb.RequestKind_LIST_TASK_REQUESTS,
				cursorpb.RequestKind_CANCEL_TASKS,
			} {
				cur, err := Encode(ctx, kind, &cursorpb.TasksCursor{
					LastTaskRequestEntityId: 12345,
				})
				assert.NoErr(t, err)

				yes, err := IsValidCursor(ctx, cur)
				assert.NoErr(t, err)
				assert.Loosely(t, yes, should.BeTrue)

				dec, err := Decode[cursorpb.TasksCursor](ctx, kind, cur)
				assert.NoErr(t, err)
				assert.Loosely(t, dec.LastTaskRequestEntityId, should.Equal(12345))

				assert.Loosely(t, func() { _, _ = Encode(ctx, kind, &cursorpb.OpaqueCursor{}) }, should.Panic)
				assert.Loosely(t, func() { _, _ = Decode[cursorpb.OpaqueCursor](ctx, kind, cur) }, should.Panic)
			}
		})

		t.Run("With encrypted cursor", func(t *ftt.Test) {
			cur, err := Encode(ctx, cursorpb.RequestKind_LIST_TASKS, &cursorpb.TasksCursor{
				LastTaskRequestEntityId: 12345,
			})
			assert.NoErr(t, err)

			t.Run("Corrupted", func(t *ftt.Test) {
				dec, err := Decode[cursorpb.TasksCursor](ctx, cursorpb.RequestKind_LIST_TASKS, cur[:len(cur)-2])
				assert.Loosely(t, dec, should.BeNil)
				assert.Loosely(t, err, should.Equal(cursorDecodeErr))
			})

			t.Run("Wrong kind #1", func(t *ftt.Test) {
				dec, err := Decode[cursorpb.TasksCursor](ctx, cursorpb.RequestKind_LIST_TASK_REQUESTS, cur)
				assert.Loosely(t, dec, should.BeNil)
				assert.Loosely(t, err, should.Equal(cursorDecodeErr))
			})

			t.Run("Wrong kind #2", func(t *ftt.Test) {
				dec, err := Decode[cursorpb.BotsCursor](ctx, cursorpb.RequestKind_LIST_BOTS, cur)
				assert.Loosely(t, dec, should.BeNil)
				assert.Loosely(t, err, should.Equal(cursorDecodeErr))
			})
		})

		t.Run("EncodeOpaqueCursor + DecodeOpaqueCursor", func(t *ftt.Test) {
			ctx = memory.Use(ctx)

			type Entity struct {
				ID int64 `gae:"$id"`
			}
			for i := 1; i <= 10; i++ {
				assert.NoErr(t, datastore.Put(ctx, &Entity{ID: int64(i)}))
			}
			datastore.GetTestable(ctx).CatchupIndexes()

			var cur datastore.Cursor
			var fetched []int64
			err := datastore.Run(ctx,
				datastore.NewQuery("Entity"),
				func(e *Entity, cb datastore.CursorCB) error {
					fetched = append(fetched, e.ID)
					if len(fetched) == 5 {
						var err error
						cur, err = cb()
						assert.NoErr(t, err)
						return datastore.Stop
					}
					return nil
				},
			)
			assert.NoErr(t, err)

			enc, err := EncodeOpaqueCursor(ctx, cursorpb.RequestKind_LIST_BOT_EVENTS, cur)
			assert.NoErr(t, err)
			dec, err := DecodeOpaqueCursor(ctx, cursorpb.RequestKind_LIST_BOT_EVENTS, enc)
			assert.NoErr(t, err)

			err = datastore.Run(ctx,
				datastore.NewQuery("Entity").Start(dec),
				func(e *Entity, cb datastore.CursorCB) error {
					fetched = append(fetched, e.ID)
					return nil
				},
			)
			assert.NoErr(t, err)

			// Resumed from the cursor correctly and finished fetching entities.
			assert.Loosely(t, fetched, should.Resemble([]int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}))
		})
	})
}
