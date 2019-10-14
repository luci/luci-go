// Copyright 2016 The LUCI Authors.
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

package prpc

import (
	"encoding/base64"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBinaryHeader(t *testing.T) {
	t.Parallel()

	Convey("from headers", t, func() {
		Convey("regular", func() {
			k, v, err := headerToMeta("K", "V")
			So(err, ShouldBeNil)
			So(k, ShouldEqual, "k")
			So(v, ShouldEqual, "V")
		})

		Convey("binary", func() {
			data := []byte{10}
			k, v, err := headerToMeta("K-Bin", base64.StdEncoding.EncodeToString(data))
			So(err, ShouldBeNil)
			So(k, ShouldEqual, "k")
			So(v, ShouldEqual, string(data))
		})

		Convey("binary invalid", func() {
			_, _, err := headerToMeta("k-bin", "Z")
			So(err, ShouldErrLike, "illegal base64 data at input byte 0")
		})
	})

	Convey("to headers", t, func() {
		Convey("regular", func() {
			k, v := metaToHeader("k", "V")
			So(k, ShouldEqual, "K")
			So(v, ShouldEqual, "V")
		})

		Convey("binary", func() {
			data := []byte{10}
			k, v := metaToHeader("K", string(data))
			So(k, ShouldEqual, "K-Bin")
			So(v, ShouldEqual, base64.StdEncoding.EncodeToString(data))
		})
	})
}
