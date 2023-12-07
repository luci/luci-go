// Copyright 2022 The LUCI Authors.
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

package internal

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/server/loginsessions/internal/statepb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func testStore(ctx context.Context, store SessionStore) {
	Convey("Create", func() {
		So(store.Create(ctx, &statepb.LoginSession{Id: "session-0"}), ShouldBeNil)
		So(store.Create(ctx, &statepb.LoginSession{Id: "session-0"}), ShouldNotBeNil)
	})

	Convey("Get", func() {
		stored := &statepb.LoginSession{
			Id:                     "session-0",
			OauthS256CodeChallenge: "some-string",
		}
		So(store.Create(ctx, stored), ShouldBeNil)

		session, err := store.Get(ctx, "session-0")
		So(err, ShouldBeNil)
		So(session, ShouldResembleProto, stored)

		session, err = store.Get(ctx, "another")
		So(err, ShouldEqual, ErrNoSession)
		So(session, ShouldBeNil)
	})

	Convey("Update", func() {
		stored := &statepb.LoginSession{
			Id:                     "session-0",
			OauthS256CodeChallenge: "some-string",
		}
		So(store.Create(ctx, stored), ShouldBeNil)

		unchanged, err := store.Update(ctx, "session-0", func(session *statepb.LoginSession) {
			So(session, ShouldResembleProto, stored)
		})
		So(err, ShouldBeNil)
		So(unchanged, ShouldResembleProto, stored)

		updated, err := store.Update(ctx, "session-0", func(session *statepb.LoginSession) {
			session.OauthS256CodeChallenge = "updated-string"
		})
		So(err, ShouldBeNil)
		So(updated, ShouldResembleProto, &statepb.LoginSession{
			Id:                     "session-0",
			OauthS256CodeChallenge: "updated-string",
		})

		fetched, err := store.Get(ctx, "session-0")
		So(err, ShouldBeNil)
		So(fetched, ShouldResembleProto, updated)
	})

	Convey("Update missing", func() {
		_, err := store.Update(ctx, "session-0", func(*statepb.LoginSession) {
			panic("must not be called")
		})
		So(err, ShouldEqual, ErrNoSession)
	})

	Convey("Update ID change", func() {
		So(store.Create(ctx, &statepb.LoginSession{
			Id:                     "session-0",
			OauthS256CodeChallenge: "some-string",
		}), ShouldBeNil)

		So(func() {
			store.Update(ctx, "session-0", func(session *statepb.LoginSession) {
				session.Id = "another"
			})
		}, ShouldPanic)
	})
}

func TestMemorySessionStore(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx := context.Background()

		testStore(ctx, &MemorySessionStore{})
	})
}

func TestDatastoreSessionStore(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		var now = testclock.TestRecentTimeUTC.Round(time.Millisecond)

		ctx, tc := testclock.UseTime(memory.Use(context.Background()), now)
		datastore.GetTestable(ctx).Consistent(true)

		testStore(ctx, &DatastoreSessionStore{})

		Convey("Cleanup", func() {
			store := &DatastoreSessionStore{}

			put := func(id string, dur time.Duration) {
				So(store.Create(ctx, &statepb.LoginSession{
					Id:     id,
					Expiry: timestamppb.New(now.Add(dur)),
				}), ShouldBeNil)
			}

			put("keep", time.Minute)
			put("delete-1", time.Second)

			// Make sure changing Expiry also moves the cleanup deadline.
			put("delete-2", time.Minute)
			_, err := store.Update(ctx, "delete-2", func(s *statepb.LoginSession) {
				s.Expiry = timestamppb.New(now.Add(time.Second))
			})
			So(err, ShouldBeNil)

			tc.Add(sessionCleanupDelay + 2*time.Second)
			So(store.Cleanup(ctx), ShouldBeNil)

			get := func(id string) bool {
				_, err := store.Get(ctx, id)
				return err == nil
			}
			So(get("keep"), ShouldBeTrue)
			So(get("delete-1"), ShouldBeFalse)
			So(get("delete-2"), ShouldBeFalse)
		})
	})
}
