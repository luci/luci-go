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
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/server/auth"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSession(t *testing.T) {
	Convey("Works", t, func() {
		ctx, _ := testclock.UseTime(context.Background(), time.Unix(1442540000, 0))
		s := MemorySessionStore{}

		ss, err := s.GetSession(ctx, "missing")
		So(err, ShouldBeNil)
		So(ss, ShouldBeNil)

		sid, err := s.OpenSession(ctx, "uid", &auth.User{Name: "dude"}, clock.Now(ctx).Add(1*time.Hour))
		So(err, ShouldBeNil)
		So(sid, ShouldEqual, "uid/1")

		ss, err = s.GetSession(ctx, "uid/1")
		So(err, ShouldBeNil)
		So(ss, ShouldResemble, &Session{
			SessionID: "uid/1",
			UserID:    "uid",
			User:      auth.User{Name: "dude"},
			Exp:       clock.Now(ctx).Add(1 * time.Hour),
		})

		So(s.CloseSession(ctx, "uid/1"), ShouldBeNil)

		ss, err = s.GetSession(ctx, "missing")
		So(err, ShouldBeNil)
		So(ss, ShouldBeNil)
	})
}
