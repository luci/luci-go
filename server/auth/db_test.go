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
	"github.com/luci/luci-go/server/auth/identity"
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
		db, err := NewSnapshotDB(&protocol.AuthDB{
			OauthClientId: strPtr("primary-client-id"),
			OauthAdditionalClientIds: []string{
				"additional-client-id-1",
				"additional-client-id-2",
			},
		}, "http://auth-service", 1234)
		So(err, ShouldBeNil)

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

	Convey("IsMember works", t, func() {
		c := context.Background()
		db, err := NewSnapshotDB(&protocol.AuthDB{
			Groups: []*protocol.AuthGroup{
				{
					Name:    strPtr("direct"),
					Members: []string{"user:abc@example.com"},
				},
				{
					Name:  strPtr("via glob"),
					Globs: []string{"user:*@example.com"},
				},
				{
					Name:   strPtr("via nested"),
					Nested: []string{"direct"},
				},
				{
					Name:   strPtr("cycle"),
					Nested: []string{"cycle"},
				},
				{
					Name:   strPtr("unknown nested"),
					Nested: []string{"unknown"},
				},
			},
		}, "http://auth-service", 1234)
		So(err, ShouldBeNil)

		call := func(ident, group string) bool {
			res, err := db.IsMember(c, identity.Identity(ident), group)
			So(err, ShouldBeNil)
			return res
		}

		So(call("user:abc@example.com", "direct"), ShouldBeTrue)
		So(call("user:another@example.com", "direct"), ShouldBeFalse)

		So(call("user:abc@example.com", "via glob"), ShouldBeTrue)
		So(call("user:abc@another.com", "via glob"), ShouldBeFalse)

		So(call("user:abc@example.com", "via nested"), ShouldBeTrue)
		So(call("user:another@example.com", "via nested"), ShouldBeFalse)

		So(call("user:abc@example.com", "cycle"), ShouldBeFalse)
		So(call("user:abc@example.com", "unknown"), ShouldBeFalse)
		So(call("user:abc@example.com", "unknown nested"), ShouldBeFalse)
	})
}

func strPtr(s string) *string { return &s }
