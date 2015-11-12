// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"errors"
	"testing"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/server/auth/service/protocol"

	. "github.com/smartystreets/goconvey/convey"
)

func TestContextAndCache(t *testing.T) {
	Convey("DBCache works", t, func() {
		c := context.Background()
		c, tc := testclock.UseTime(c, time.Unix(1442540000, 0))

		c = UseDB(c, NewDBCache(func(c context.Context, prev DB) (DB, error) {
			if prev == nil {
				return &SnapshotDB{Rev: 1}, nil
			}
			cpy := *prev.(*SnapshotDB)
			cpy.Rev++
			return &cpy, nil
		}))

		// Initial fetch.
		db, err := GetDB(c)
		So(err, ShouldBeNil)
		So(db.(*SnapshotDB).Rev, ShouldEqual, 1)

		// Refetch, using cached copy.
		db, err = GetDB(c)
		So(err, ShouldBeNil)
		So(db.(*SnapshotDB).Rev, ShouldEqual, 1)

		// Advance time to expire cache.
		tc.Add(6 * time.Second)

		// Returns new copy now.
		db, err = GetDB(c)
		So(err, ShouldBeNil)
		So(db.(*SnapshotDB).Rev, ShouldEqual, 2)
	})

	Convey("Fetch failure propagates", t, func() {
		fail := errors.New("fail")

		c := context.Background()
		c = UseDB(c, NewDBCache(func(c context.Context, prev DB) (DB, error) {
			return nil, fail
		}))

		db, err := GetDB(c)
		So(err, ShouldEqual, fail)
		So(db, ShouldBeNil)
	})
}

func TestSnapshotDB(t *testing.T) {
	Convey("IsAllowedOAuthClientID works", t, func() {
		c := context.Background()
		db := NewSnapshotDB(&protocol.AuthDB{
			OauthClientId: strPtr("primary-client-id"),
			OauthAdditionalClientIds: []string{
				"additional-client-id-1",
				"additional-client-id-2",
			},
		}, "http://auth-service", 1234)

		call := func(email, clientID string) bool {
			res, err := db.IsAllowedOAuthClientID(c, email, clientID)
			So(err, ShouldBeNil)
			return res
		}

		So(call("abc@appspot.gserviceaccount.com", "anonymous"), ShouldBeTrue)
		So(call("dude@example.com", ""), ShouldBeFalse)
		So(call("dude@example.com", googleAPIExplorerClientID), ShouldBeTrue)
		So(call("dude@example.com", "primary-client-id"), ShouldBeTrue)
		So(call("dude@example.com", "additional-client-id-2"), ShouldBeTrue)
		So(call("dude@example.com", "unknown-client-id"), ShouldBeFalse)
	})
}

func strPtr(s string) *string { return &s }
