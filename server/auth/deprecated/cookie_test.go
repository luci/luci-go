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

package deprecated

import (
	"context"
	"net/http"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCookie(t *testing.T) {
	Convey("With context", t, func() {
		c := context.Background()
		c, _ = testclock.UseTime(c, time.Unix(1442540000, 0))
		c = secrets.Use(c, &testsecrets.Store{})

		Convey("Encode and decode works", func() {
			cookie, err := makeSessionCookie(c, "sid", true)
			So(err, ShouldBeNil)
			So(cookie, ShouldResemble, &http.Cookie{
				Name: "oid_session",
				Value: "AXsiX2kiOiIxNDQyNTQwMDAwMDAwIiwic2lkIjoic2lkIn1NXPzKTFXWhzt" +
					"tmqW2uODV4f1Nvt1zLxAnWTtjqkhGEQ",
				Path:     "/",
				Expires:  clock.Now(c).Add(2591100 * time.Second),
				MaxAge:   2591100,
				Secure:   true,
				HttpOnly: true,
			})

			r, err := http.NewRequest("GET", "http://example.com", nil)
			So(err, ShouldBeNil)
			r.AddCookie(cookie)

			sid, err := decodeSessionCookie(c, r)
			So(err, ShouldBeNil)
			So(sid, ShouldEqual, "sid")
		})

		Convey("Bad cookie is ignored", func() {
			r, err := http.NewRequest("GET", "http://example.com", nil)
			So(err, ShouldBeNil)
			r.AddCookie(&http.Cookie{
				Name:     "oid_session",
				Value:    "garbage",
				Path:     "/",
				Expires:  clock.Now(c).Add(2591100 * time.Second),
				MaxAge:   2591100,
				Secure:   true,
				HttpOnly: true,
			})
			sid, err := decodeSessionCookie(c, r)
			So(err, ShouldBeNil)
			So(sid, ShouldEqual, "")
		})

		Convey("Expired session token is ignored", func() {
			r, err := http.NewRequest("GET", "http://example.com", nil)
			So(err, ShouldBeNil)
			r.AddCookie(&http.Cookie{
				Name: "oid_session",
				Value: "AXsiX2kiOiIxNDQyNTQwMDAwMDAwIiwic2lkIjoic2lkIn1NXPzKTFXWhzt" +
					"tmqW2uODV4f1Nvt1zLxAnWTtjqkhGEQ",
				Path:     "/",
				Expires:  clock.Now(c).Add(2591100 * time.Second),
				MaxAge:   2591100,
				Secure:   true,
				HttpOnly: true,
			})

			// Works now.
			sid, err := decodeSessionCookie(c, r)
			So(err, ShouldBeNil)
			So(sid, ShouldEqual, "sid")

			// Doesn't work after expiration.
			clock.Get(c).(testclock.TestClock).Add(2600000 * time.Second)
			sid, err = decodeSessionCookie(c, r)
			So(err, ShouldBeNil)
			So(sid, ShouldEqual, "")
		})
	})
}
