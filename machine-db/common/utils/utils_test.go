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

package utils

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestRange(t *testing.T) {
	Convey("invalid CIDR block", t, func() {
		_, _, err := Range("a.b.c.d/f")
		So(err, ShouldErrLike, "invalid CIDR block")
	})

	Convey("IPv6 CIDR block", t, func() {
		_, _, err := Range("2001:db8:a0b:12f0::1/32")
		So(err, ShouldErrLike, "invalid IPv4 CIDR block")
	})

	Convey("ok", t, func() {
		start, length, err := Range("127.0.0.1/24")
		So(err, ShouldBeNil)
		So(start, ShouldEqual, 2130706432)
		So(length, ShouldEqual, 256)
	})
}
