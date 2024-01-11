// Copyright 2024 The LUCI Authors.
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

package zlib

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCodec(t *testing.T) {
	t.Parallel()

	Convey("decompressing compressed data works", t, func() {
		data := []byte("this is test data")

		// Check compressing executes.
		compressed, err := Compress(data)
		So(err, ShouldBeNil)

		// Check decompressing executes.
		decompressed, err := Decompress(compressed)
		So(err, ShouldBeNil)

		// Check the decompressed compressed data matches the original.
		So(decompressed, ShouldEqual, data)
	})
}
