// Copyright 2017 The LUCI Authors.
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

package settings

import (
	"context"
	"testing"

	. "github.com/smartystreets/goconvey/convey"

	"go.chromium.org/luci/server/settings"
)

func TestDatabaseSettings(t *testing.T) {
	t.Parallel()

	Convey("New: ok", t, func() {
		s := New(context.Background())
		So(s, ShouldNotBeNil)
		So(s.Server, ShouldEqual, "")
		So(s.Username, ShouldEqual, "")
		So(s.Password, ShouldEqual, "")
	})

	Convey("GetUncached: returns defaults", t, func() {
		s, e := GetUncached(context.Background())
		So(s, ShouldNotBeNil)
		So(s.Server, ShouldEqual, "")
		So(s.Username, ShouldEqual, "")
		So(s.Password, ShouldEqual, "")
		So(e, ShouldBeNil)
	})

	Convey("GetUncached: returns settings", t, func() {
		c := settings.Use(context.Background(), settings.New(&settings.MemoryStorage{}))
		expected := &DatabaseSettings{
			Server:   "server",
			Username: "username",
			Password: "password",
		}
		e := settings.Set(c, settingsKey, expected, "whoever", "for whatever reason")
		So(e, ShouldBeNil)

		actual, e := GetUncached(c)
		So(actual, ShouldNotBeNil)
		So(actual.Server, ShouldEqual, expected.Server)
		So(actual.Username, ShouldEqual, expected.Username)
		So(actual.Password, ShouldEqual, expected.Password)
		So(e, ShouldBeNil)
	})
}
