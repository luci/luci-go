// Copyright 2019 The LUCI Authors.
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

package globset

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGlobSet(t *testing.T) {
	Convey("Empty", t, func() {
		gs, err := NewBuilder().Build()
		So(err, ShouldBeNil)
		So(gs, ShouldBeNil)
	})

	Convey("Works", t, func() {
		b := NewBuilder()
		So(b.Add("user:*@example.com"), ShouldBeNil)
		So(b.Add("user:*@other.example.com"), ShouldBeNil)
		So(b.Add("service:*"), ShouldBeNil)

		gs, err := b.Build()
		So(err, ShouldBeNil)

		So(gs, ShouldHaveLength, 2)
		So(gs["user"].String(), ShouldEqual, `^((.*@example\.com)|(.*@other\.example\.com))$`)
		So(gs["service"].String(), ShouldEqual, `^.*$`)

		So(gs.Has("user:a@example.com"), ShouldBeTrue)
		So(gs.Has("user:a@other.example.com"), ShouldBeTrue)
		So(gs.Has("user:a@not-example.com"), ShouldBeFalse)
		So(gs.Has("service:zzz"), ShouldBeTrue)
		So(gs.Has("anonymous:anonymous"), ShouldBeFalse)
	})

	Convey("Caches regexps", t, func() {
		b := NewBuilder()

		So(b.Add("user:*@example.com"), ShouldBeNil)
		So(b.Add("user:*@other.example.com"), ShouldBeNil)

		gs1, err := b.Build()
		So(err, ShouldBeNil)

		b.Reset()
		So(b.Add("user:*@other.example.com"), ShouldBeNil)
		So(b.Add("user:*@example.com"), ShouldBeNil)

		gs2, err := b.Build()
		So(err, ShouldBeNil)

		// The exact same regexp object.
		So(gs1["user"], ShouldEqual, gs2["user"])
	})

	Convey("Edge cases in Has", t, func() {
		b := NewBuilder()
		b.Add("user:a*@example.com")
		b.Add("user:*@other.example.com")
		gs, _ := b.Build()

		So(gs.Has("abc@example.com"), ShouldBeFalse)                // no "user:" prefix
		So(gs.Has("service:abc@example.com"), ShouldBeFalse)        // wrong prefix
		So(gs.Has("user:abc@example.com\nsneaky"), ShouldBeFalse)   // sneaky '/n'
		So(gs.Has("user:bbc@example.com"), ShouldBeFalse)           // '^' is checked
		So(gs.Has("user:abc@example.com-and-stuff"), ShouldBeFalse) // '$' is checked
	})
}
