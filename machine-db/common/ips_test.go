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

func TestIPv4(t *testing.T) {
	t.Parallel()

	Convey("String", t, func() {
		So(IPv4(2130706433).String(), ShouldEqual, "127.0.0.1")
		So(IPv4(0).String(), ShouldEqual, "0.0.0.0")
		So(IPv4(uint32(4294967295)).String(), ShouldEqual, "255.255.255.255")
	})
}

func TestIPv4Range(t *testing.T) {
	t.Parallel()

	Convey("invalid CIDR block", t, func() {
		_, _, err := IPv4Range("a.b.c.d/f")
		So(err, ShouldErrLike, "invalid CIDR block")
	})

	Convey("IPv6 CIDR block", t, func() {
		_, _, err := IPv4Range("2001:db8:a0b:12f0::1/32")
		So(err, ShouldErrLike, "invalid IPv4 CIDR block")
	})

	Convey("min", t, func() {
		start, length, err := IPv4Range("127.0.0.1/32")
		So(err, ShouldBeNil)
		So(start, ShouldEqual, 2130706433)
		So(length, ShouldEqual, 1)
	})

	Convey("max", t, func() {
		start, length, err := IPv4Range("127.0.0.1/0")
		So(err, ShouldBeNil)
		So(start, ShouldEqual, 0)
		So(length, ShouldEqual, int64(4294967296))
	})

	Convey("ok", t, func() {
		start, length, err := IPv4Range("127.0.0.1/24")
		So(err, ShouldBeNil)
		So(start, ShouldEqual, 2130706432)
		So(length, ShouldEqual, 256)
	})
}

func TestParseIPv4(t *testing.T) {
	t.Parallel()

	Convey("IPv64", t, func() {
		_, err := ParseIPv4("2001:db8:a0b:12f0::1")
		So(err, ShouldErrLike, "invalid IPv4 address")
	})

	Convey("min", t, func() {
		ip, err := ParseIPv4("0.0.0.0")
		So(err, ShouldBeNil)
		So(ip, ShouldEqual, 0)
	})

	Convey("max", t, func() {
		ip, err := ParseIPv4("255.255.255.255")
		So(err, ShouldBeNil)
		So(ip, ShouldEqual, IPv4(uint32(4294967295)))
	})

	Convey("ok", t, func() {
		ip, err := ParseIPv4("127.0.0.1")
		So(err, ShouldBeNil)
		So(ip, ShouldEqual, IPv4(2130706433))
	})
}
