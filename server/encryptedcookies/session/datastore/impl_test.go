// Copyright 2021 The LUCI Authors.
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
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/server/encryptedcookies/session"
	"go.chromium.org/luci/server/encryptedcookies/session/sessionpb"

	. "github.com/smartystreets/goconvey/convey"
)

func TestWorks(t *testing.T) {
	t.Parallel()

	Convey("With DS", t, func() {
		testTime := testclock.TestRecentTimeUTC.Round(time.Millisecond)
		ctx, _ := testclock.UseTime(context.Background(), testTime)
		ctx = memory.Use(ctx)

		Convey("Works", func() {
			store := Store{}
			id := session.GenerateID()

			s, err := store.FetchSession(ctx, id)
			So(err, ShouldBeNil)
			So(s, ShouldBeNil)

			err = store.UpdateSession(ctx, id, func(s *sessionpb.Session) error {
				So(s.State, ShouldEqual, sessionpb.State_STATE_UNDEFINED)
				s.State = sessionpb.State_STATE_OPEN
				s.Email = "abc@example.com"
				return nil
			})
			So(err, ShouldBeNil)

			s, err = store.FetchSession(ctx, id)
			So(err, ShouldBeNil)
			So(s.State, ShouldEqual, sessionpb.State_STATE_OPEN)
			So(s.Email, ShouldEqual, "abc@example.com")

			ent, err := fetchRawEntity(ctx, id)
			So(err, ShouldBeNil)
			So(ent.ExpireAt.Equal(testTime.Add(InactiveSessionExpiration)), ShouldBeTrue)

			err = store.UpdateSession(ctx, id, func(s *sessionpb.Session) error {
				So(s.State, ShouldEqual, sessionpb.State_STATE_OPEN)
				So(s.Email, ShouldEqual, "abc@example.com")
				s.State = sessionpb.State_STATE_CLOSED
				s.LastRefresh = timestamppb.New(testTime.Add(5 * time.Hour))
				return nil
			})
			So(err, ShouldBeNil)

			s, err = store.FetchSession(ctx, id)
			So(err, ShouldBeNil)
			So(s.State, ShouldEqual, sessionpb.State_STATE_CLOSED)

			ent, err = fetchRawEntity(ctx, id)
			So(err, ShouldBeNil)
			So(ent.ExpireAt.Equal(testTime.Add(5*time.Hour+InactiveSessionExpiration)), ShouldBeTrue)
		})
	})
}

func fetchRawEntity(ctx context.Context, id session.ID) (*SessionEntity, error) {
	ent := &SessionEntity{ID: entityID(id)}
	err := datastore.Get(ctx, ent)
	return ent, err
}
