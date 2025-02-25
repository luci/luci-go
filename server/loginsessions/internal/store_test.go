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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/server/loginsessions/internal/statepb"
)

func testStore(ctx context.Context, t *ftt.Test, store SessionStore) {
	t.Run("Create", func(t *ftt.Test) {
		assert.Loosely(t, store.Create(ctx, &statepb.LoginSession{Id: "session-0"}), should.BeNil)
		assert.Loosely(t, store.Create(ctx, &statepb.LoginSession{Id: "session-0"}), should.NotBeNil)
	})

	t.Run("Get", func(t *ftt.Test) {
		stored := &statepb.LoginSession{
			Id:                     "session-0",
			OauthS256CodeChallenge: "some-string",
		}
		assert.Loosely(t, store.Create(ctx, stored), should.BeNil)

		session, err := store.Get(ctx, "session-0")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, session, should.Match(stored))

		session, err = store.Get(ctx, "another")
		assert.Loosely(t, err, should.Equal(ErrNoSession))
		assert.Loosely(t, session, should.BeNil)
	})

	t.Run("Update", func(t *ftt.Test) {
		stored := &statepb.LoginSession{
			Id:                     "session-0",
			OauthS256CodeChallenge: "some-string",
		}
		assert.Loosely(t, store.Create(ctx, stored), should.BeNil)

		unchanged, err := store.Update(ctx, "session-0", func(session *statepb.LoginSession) {
			assert.Loosely(t, session, should.Match(stored))
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, unchanged, should.Match(stored))

		updated, err := store.Update(ctx, "session-0", func(session *statepb.LoginSession) {
			session.OauthS256CodeChallenge = "updated-string"
		})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, updated, should.Match(&statepb.LoginSession{
			Id:                     "session-0",
			OauthS256CodeChallenge: "updated-string",
		}))

		fetched, err := store.Get(ctx, "session-0")
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, fetched, should.Match(updated))
	})

	t.Run("Update missing", func(t *ftt.Test) {
		_, err := store.Update(ctx, "session-0", func(*statepb.LoginSession) {
			panic("must not be called")
		})
		assert.Loosely(t, err, should.Equal(ErrNoSession))
	})

	t.Run("Update ID change", func(t *ftt.Test) {
		assert.Loosely(t, store.Create(ctx, &statepb.LoginSession{
			Id:                     "session-0",
			OauthS256CodeChallenge: "some-string",
		}), should.BeNil)

		assert.Loosely(t, func() {
			store.Update(ctx, "session-0", func(session *statepb.LoginSession) {
				session.Id = "another"
			})
		}, should.Panic)
	})
}

func TestMemorySessionStore(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		ctx := context.Background()

		testStore(ctx, t, &MemorySessionStore{})
	})
}

func TestDatastoreSessionStore(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		var now = testclock.TestRecentTimeUTC.Round(time.Millisecond)

		ctx, tc := testclock.UseTime(memory.Use(context.Background()), now)
		datastore.GetTestable(ctx).Consistent(true)

		testStore(ctx, t, &DatastoreSessionStore{})

		t.Run("Cleanup", func(t *ftt.Test) {
			store := &DatastoreSessionStore{}

			put := func(id string, dur time.Duration) {
				assert.Loosely(t, store.Create(ctx, &statepb.LoginSession{
					Id:     id,
					Expiry: timestamppb.New(now.Add(dur)),
				}), should.BeNil)
			}

			put("keep", time.Minute)
			put("delete-1", time.Second)

			// Make sure changing Expiry also moves the cleanup deadline.
			put("delete-2", time.Minute)
			_, err := store.Update(ctx, "delete-2", func(s *statepb.LoginSession) {
				s.Expiry = timestamppb.New(now.Add(time.Second))
			})
			assert.Loosely(t, err, should.BeNil)

			tc.Add(sessionCleanupDelay + 2*time.Second)
			assert.Loosely(t, store.Cleanup(ctx), should.BeNil)

			get := func(id string) bool {
				_, err := store.Get(ctx, id)
				return err == nil
			}
			assert.Loosely(t, get("keep"), should.BeTrue)
			assert.Loosely(t, get("delete-1"), should.BeFalse)
			assert.Loosely(t, get("delete-2"), should.BeFalse)
		})
	})
}
