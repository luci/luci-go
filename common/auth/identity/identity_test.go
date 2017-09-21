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

package identity

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestIdentity(t *testing.T) {
	Convey("MakeIdentity works", t, func() {
		id, err := MakeIdentity("anonymous:anonymous")
		So(err, ShouldBeNil)
		So(id, ShouldEqual, Identity("anonymous:anonymous"))
		So(id.Kind(), ShouldEqual, Anonymous)
		So(id.Value(), ShouldEqual, "anonymous")
		So(id.Email(), ShouldEqual, "")

		_, err = MakeIdentity("bad ident")
		So(err, ShouldNotBeNil)
	})

	Convey("Validate works", t, func() {
		So(Identity("user:abc@example.com").Validate(), ShouldBeNil)
		So(Identity("user:").Validate(), ShouldNotBeNil)
		So(Identity(":abc").Validate(), ShouldNotBeNil)
		So(Identity("abc@example.com").Validate(), ShouldNotBeNil)
		So(Identity("user:abc").Validate(), ShouldNotBeNil)
	})

	Convey("Kind works", t, func() {
		So(Identity("user:abc@example.com").Kind(), ShouldEqual, User)
		So(Identity("???").Kind(), ShouldEqual, Anonymous)
	})

	Convey("Value works", t, func() {
		So(Identity("service:abc").Value(), ShouldEqual, "abc")
		So(Identity("???").Value(), ShouldEqual, "anonymous")
	})

	Convey("Email works", t, func() {
		So(Identity("user:abc@example.com").Email(), ShouldEqual, "abc@example.com")
		So(Identity("service:abc").Email(), ShouldEqual, "")
		So(Identity("???").Email(), ShouldEqual, "")
	})
}
