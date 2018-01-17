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
	"net"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMAC48ToUint64(t *testing.T) {
	t.Parallel()

	Convey("eui64", t, func() {
		_, err := MAC48ToUint64(net.HardwareAddr{0x1, 0x2, 0x3, 0x4, 0x5, 0x6, 0x7, 0x8})
		So(err, ShouldErrLike, "invalid MAC-48 address")
	})

	Convey("ok", t, func() {
		mac, err := MAC48ToUint64(net.HardwareAddr{0x1, 0x2, 0x3, 0x4, 0x5, 0x6})
		So(err, ShouldBeNil)
		So(mac, ShouldEqual, 1108152157446)
	})

	Convey("min", t, func() {
		mac, err := MAC48ToUint64(net.HardwareAddr{0x0, 0x0, 0x0, 0x0, 0x0, 0x0})
		So(err, ShouldBeNil)
		So(mac, ShouldEqual, 0)
	})

	Convey("max", t, func() {
		mac, err := MAC48ToUint64(net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
		So(err, ShouldBeNil)
		So(mac, ShouldEqual, MaxMAC48)
	})
}

func TestMAC48StringToUint64(t *testing.T) {
	t.Parallel()

	Convey("eui64", t, func() {
		_, err := MAC48StringToUint64("01:02:03:04:05:06:07:08")
		So(err, ShouldErrLike, "invalid MAC-48 address")
	})

	Convey("ok", t, func() {
		mac, err := MAC48StringToUint64("01:02:03:04:05:06")
		So(err, ShouldBeNil)
		So(mac, ShouldEqual, 1108152157446)
	})

	Convey("min", t, func() {
		mac, err := MAC48StringToUint64("00:00:00:00:00:00")
		So(err, ShouldBeNil)
		So(mac, ShouldEqual, 0)
	})

	Convey("max", t, func() {
		mac, err := MAC48StringToUint64("ff:ff:ff:ff:ff:ff")
		So(err, ShouldBeNil)
		So(mac, ShouldEqual, MaxMAC48)
	})
}

func TestUint64ToMAC48(t *testing.T) {
	t.Parallel()

	Convey("eui64", t, func() {
		_, err := Uint64ToMAC48(MaxMAC48 + 1)
		So(err, ShouldErrLike, "invalid MAC-48 address")
	})

	Convey("ok", t, func() {
		mac, err := Uint64ToMAC48(1108152157446)
		So(err, ShouldBeNil)
		So(mac, ShouldResemble, net.HardwareAddr{0x1, 0x2, 0x3, 0x4, 0x5, 0x6})
		So(mac.String(), ShouldEqual, "01:02:03:04:05:06")
	})

	Convey("min", t, func() {
		mac, err := Uint64ToMAC48(0)
		So(err, ShouldBeNil)
		So(mac, ShouldResemble, net.HardwareAddr{0x0, 0x0, 0x0, 0x0, 0x0, 0x0})
		So(mac.String(), ShouldEqual, "00:00:00:00:00:00")
	})

	Convey("max", t, func() {
		mac, err := Uint64ToMAC48(MaxMAC48)
		So(err, ShouldBeNil)
		So(mac, ShouldResemble, net.HardwareAddr{0xff, 0xff, 0xff, 0xff, 0xff, 0xff})
		So(mac.String(), ShouldEqual, "ff:ff:ff:ff:ff:ff")
	})
}
