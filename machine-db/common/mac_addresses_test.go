// Copyright 2018 The LUCI Authors.
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

package common

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMAC48(t *testing.T) {
	t.Parallel()

	Convey("String", t, func() {
		So(MAC48(1108152157446).String(), ShouldEqual, "01:02:03:04:05:06")
		So(MAC48(0).String(), ShouldEqual, "00:00:00:00:00:00")
		So(MaxMAC48.String(), ShouldEqual, "ff:ff:ff:ff:ff:ff")
	})
}

func TestParseMAC48(t *testing.T) {
	t.Parallel()

	Convey("eui64", t, func() {
		_, err := ParseMAC48("01:02:03:04:05:06:07:08")
		So(err, ShouldErrLike, "invalid MAC-48 address")
	})

	Convey("ok", t, func() {
		mac, err := ParseMAC48("01:02:03:04:05:06")
		So(err, ShouldBeNil)
		So(mac, ShouldEqual, MAC48(1108152157446))
	})

	Convey("min", t, func() {
		mac, err := ParseMAC48("00:00:00:00:00:00")
		So(err, ShouldBeNil)
		So(mac, ShouldEqual, 0)
	})

	Convey("max", t, func() {
		mac, err := ParseMAC48("ff:ff:ff:ff:ff:ff")
		So(err, ShouldBeNil)
		So(mac, ShouldEqual, MaxMAC48)
	})
}
