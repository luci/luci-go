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

package utils

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestZip(t *testing.T) {
	Convey("ZlibCompress/ZlibDecompress roundtrip", t, func() {
		data := "blah-blah"
		for i := 0; i < 10; i++ {
			data += data
		}

		blob, err := ZlibCompress([]byte(data))
		So(err, ShouldBeNil)

		back, err := ZlibDecompress(blob)
		So(err, ShouldBeNil)

		So(back, ShouldResemble, []byte(data))
	})

	Convey("ZlibDecompress garbage", t, func() {
		_, err := ZlibDecompress([]byte("garbage"))
		So(err, ShouldNotBeNil)
	})
}
