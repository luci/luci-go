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
	"net/http"
	"testing"

	"google.golang.org/grpc/metadata"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBinaryHeader(t *testing.T) {
	t.Parallel()

	Convey("from headers", t, func() {
		Convey("regular", func() {
			md, err := headersIntoMetadata(http.Header{
				"Key":         {"v1", "v2"},
				"Another-Key": {"v3"},
			})
			So(err, ShouldBeNil)
			So(md, ShouldResemble, metadata.MD{
				"key":         {"v1", "v2"},
				"another-key": {"v3"},
			})
		})

		Convey("binary", func() {
			data := []byte{10}
			md, err := headersIntoMetadata(http.Header{
				"Key-Bin": {base64.StdEncoding.EncodeToString(data)},
			})
			So(err, ShouldBeNil)
			So(md, ShouldResemble, metadata.MD{
				"key-bin": {string(data)},
			})
		})

		Convey("binary invalid", func() {
			_, err := headersIntoMetadata(http.Header{
				"Key-Bin": {"Z"},
			})
			So(err, ShouldErrLike, "illegal base64 data at input byte 0")
		})

		Convey("reserved", func() {
			md, err := headersIntoMetadata(http.Header{
				"Content-Type": {"zzz"},
				"X-Prpc-Zzz":   {"zzz"},
			})
			So(err, ShouldBeNil)
			So(md, ShouldResemble, metadata.MD{})
		})
	})

	Convey("to headers", t, func() {
		Convey("regular", func() {
			h := http.Header{}
			err := metaIntoHeaders(metadata.MD{
				"key":         {"v1", "v2"},
				"another-key": {"v3"},
			}, h)
			So(err, ShouldBeNil)
			So(h, ShouldResemble, http.Header{
				"Key":         {"v1", "v2"},
				"Another-Key": {"v3"},
			})
		})

		Convey("binary", func() {
			data := []byte{10}
			h := http.Header{}
			err := metaIntoHeaders(metadata.MD{
				"key-bin": {string(data)},
			}, h)
			So(err, ShouldBeNil)
			So(h, ShouldResemble, http.Header{
				"Key-Bin": {base64.StdEncoding.EncodeToString(data)},
			})
		})

		Convey("reserved", func() {
			h := http.Header{}
			err := metaIntoHeaders(metadata.MD{
				"content-type": {"zzz"},
			}, h)
			So(err, ShouldErrLike, "using reserved metadata key")
		})
	})
}
