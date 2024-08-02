// Copyright 2015 The LUCI Authors.
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

package server

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/deprecated"
)

func TestWorks(t *testing.T) {
	ftt.Run("Works", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		c, _ = testclock.UseTime(c, time.Unix(1442540000, 0))
		s := SessionStore{Prefix: "ns"}
		u := auth.User{
			Identity: "user:abc@example.com",
			Email:    "abc@example.com",
			Name:     "Name",
			Picture:  "picture",
		}

		sid, err := s.OpenSession(c, "uid", &u, clock.Now(c).Add(time.Hour))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, sid, should.Equal("ns/uid/1"))

		session, err := s.GetSession(c, sid)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, session, should.Resemble(&deprecated.Session{
			SessionID: "ns/uid/1",
			UserID:    "uid",
			User:      u,
			Exp:       clock.Now(c).Add(time.Hour).UTC(),
		}))

		assert.Loosely(t, s.CloseSession(c, sid), should.BeNil)

		session, err = s.GetSession(c, sid)
		assert.Loosely(t, session, should.BeNil)
		assert.Loosely(t, err, should.BeNil)

		// Closed closed session is fine.
		assert.Loosely(t, s.CloseSession(c, sid), should.BeNil)
	})

	ftt.Run("Test expiration", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		c, tc := testclock.UseTime(c, time.Unix(1442540000, 0))
		s := SessionStore{Prefix: "ns"}
		u := auth.User{Identity: "user:abc@example.com"}

		sid, err := s.OpenSession(c, "uid", &u, clock.Now(c).Add(time.Hour))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, sid, should.Equal("ns/uid/1"))

		session, err := s.GetSession(c, sid)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, session, should.NotBeNil)

		tc.Add(2 * time.Hour)

		session, err = s.GetSession(c, sid)
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, session, should.BeNil)
	})

	ftt.Run("Test bad params in OpenSession", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		u := auth.User{Identity: "user:abc@example.com"}
		exp := time.Unix(1442540000, 0)

		s := SessionStore{Prefix: "/"}
		_, err := s.OpenSession(c, "uid", &u, exp)
		assert.Loosely(t, err, should.NotBeNil)

		s = SessionStore{Prefix: "ns"}
		_, err = s.OpenSession(c, "u/i/d", &u, exp)
		assert.Loosely(t, err, should.NotBeNil)

		_, err = s.OpenSession(c, "uid", &auth.User{Identity: "bad"}, exp)
		assert.Loosely(t, err, should.NotBeNil)
	})

	ftt.Run("Test bad session ID", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		s := SessionStore{Prefix: "ns"}

		session, err := s.GetSession(c, "ns/uid")
		assert.Loosely(t, session, should.BeNil)
		assert.Loosely(t, err, should.BeNil)

		session, err = s.GetSession(c, "badns/uid/1")
		assert.Loosely(t, session, should.BeNil)
		assert.Loosely(t, err, should.BeNil)

		session, err = s.GetSession(c, "ns/uid/notint")
		assert.Loosely(t, session, should.BeNil)
		assert.Loosely(t, err, should.BeNil)

		session, err = s.GetSession(c, "ns/missing/1")
		assert.Loosely(t, session, should.BeNil)
		assert.Loosely(t, err, should.BeNil)
	})
}
