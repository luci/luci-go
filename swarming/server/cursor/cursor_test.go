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

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/secrets"

	"go.chromium.org/luci/swarming/server/cursor/cursorpb"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCursor(t *testing.T) {
	t.Parallel()

	Convey("With key", t, func() {
		ctx := context.Background()
		ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

		Convey("OpaqueCursor", func() {
			for _, kind := range []cursorpb.RequestKind{
				cursorpb.RequestKind_LIST_BOT_EVENTS,
				cursorpb.RequestKind_LIST_BOT_TASKS,
			} {
				cur, err := Encode(ctx, kind, &cursorpb.OpaqueCursor{
					Cursor: []byte("hello"),
				})
				So(err, ShouldBeNil)

				yes, err := IsValidCursor(ctx, cur)
				So(err, ShouldBeNil)
				So(yes, ShouldBeTrue)

				dec, err := Decode[cursorpb.OpaqueCursor](ctx, kind, cur)
				So(err, ShouldBeNil)
				So(string(dec.Cursor), ShouldEqual, "hello")

				So(func() { _, _ = Encode(ctx, kind, &cursorpb.BotsCursor{}) }, ShouldPanic)
				So(func() { _, _ = Decode[cursorpb.BotsCursor](ctx, kind, cur) }, ShouldPanic)
			}
		})

		Convey("BotsCursor", func() {
			for _, kind := range []cursorpb.RequestKind{
				cursorpb.RequestKind_LIST_BOTS,
			} {
				cur, err := Encode(ctx, kind, &cursorpb.BotsCursor{
					LastBotId: "hello",
				})
				So(err, ShouldBeNil)

				yes, err := IsValidCursor(ctx, cur)
				So(err, ShouldBeNil)
				So(yes, ShouldBeTrue)

				dec, err := Decode[cursorpb.BotsCursor](ctx, kind, cur)
				So(err, ShouldBeNil)
				So(dec.LastBotId, ShouldEqual, "hello")

				So(func() { _, _ = Encode(ctx, kind, &cursorpb.TasksCursor{}) }, ShouldPanic)
				So(func() { _, _ = Decode[cursorpb.TasksCursor](ctx, kind, cur) }, ShouldPanic)
			}
		})

		Convey("TasksCursor", func() {
			for _, kind := range []cursorpb.RequestKind{
				cursorpb.RequestKind_LIST_TASKS,
				cursorpb.RequestKind_LIST_TASK_REQUESTS,
				cursorpb.RequestKind_CANCEL_TASKS,
			} {
				cur, err := Encode(ctx, kind, &cursorpb.TasksCursor{
					LastTaskRequestEntityId: 12345,
				})
				So(err, ShouldBeNil)

				yes, err := IsValidCursor(ctx, cur)
				So(err, ShouldBeNil)
				So(yes, ShouldBeTrue)

				dec, err := Decode[cursorpb.TasksCursor](ctx, kind, cur)
				So(err, ShouldBeNil)
				So(dec.LastTaskRequestEntityId, ShouldEqual, 12345)

				So(func() { _, _ = Encode(ctx, kind, &cursorpb.OpaqueCursor{}) }, ShouldPanic)
				So(func() { _, _ = Decode[cursorpb.OpaqueCursor](ctx, kind, cur) }, ShouldPanic)
			}
		})

		Convey("With encrypted cursor", func() {
			cur, err := Encode(ctx, cursorpb.RequestKind_LIST_TASKS, &cursorpb.TasksCursor{
				LastTaskRequestEntityId: 12345,
			})
			So(err, ShouldBeNil)

			Convey("Corrupted", func() {
				dec, err := Decode[cursorpb.TasksCursor](ctx, cursorpb.RequestKind_LIST_TASKS, cur[:len(cur)-2])
				So(dec, ShouldBeNil)
				So(err, ShouldEqual, cursorDecodeErr)
			})

			Convey("Wrong kind #1", func() {
				dec, err := Decode[cursorpb.TasksCursor](ctx, cursorpb.RequestKind_LIST_TASK_REQUESTS, cur)
				So(dec, ShouldBeNil)
				So(err, ShouldEqual, cursorDecodeErr)
			})

			Convey("Wrong kind #2", func() {
				dec, err := Decode[cursorpb.BotsCursor](ctx, cursorpb.RequestKind_LIST_BOTS, cur)
				So(dec, ShouldBeNil)
				So(err, ShouldEqual, cursorDecodeErr)
			})
		})

		Convey("EncodeOpaqueCursor + DecodeOpaqueCursor", func() {
			ctx = memory.Use(ctx)

			type Entity struct {
				ID int64 `gae:"$id"`
			}
			for i := 1; i <= 10; i++ {
				So(datastore.Put(ctx, &Entity{ID: int64(i)}), ShouldBeNil)
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
						So(err, ShouldBeNil)
						return datastore.Stop
					}
					return nil
				},
			)
			So(err, ShouldBeNil)

			enc, err := EncodeOpaqueCursor(ctx, cursorpb.RequestKind_LIST_BOT_EVENTS, cur)
			So(err, ShouldBeNil)
			dec, err := DecodeOpaqueCursor(ctx, cursorpb.RequestKind_LIST_BOT_EVENTS, enc)
			So(err, ShouldBeNil)

			err = datastore.Run(ctx,
				datastore.NewQuery("Entity").Start(dec),
				func(e *Entity, cb datastore.CursorCB) error {
					fetched = append(fetched, e.ID)
					return nil
				},
			)
			So(err, ShouldBeNil)

			// Resumed from the cursor correctly and finished fetching entities.
			So(fetched, ShouldResemble, []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
		})
	})
}
