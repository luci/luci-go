// Copyright 2022 The LUCI Authors.
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

package clustering

import (
	"encoding/hex"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidate(t *testing.T) {
	Convey(`Validate`, t, func() {
		id := ClusterID{
			Algorithm: "blah-v2",
			ID:        hex.EncodeToString([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}),
		}
		Convey(`Algorithm missing`, func() {
			id.Algorithm = ""
			err := id.Validate()
			So(err, ShouldErrLike, `algorithm not valid`)
		})
		Convey("Algorithm invalid", func() {
			id.Algorithm = "!!!"
			err := id.Validate()
			So(err, ShouldErrLike, `algorithm not valid`)
		})
		Convey("ID missing", func() {
			id.ID = ""
			err := id.Validate()
			So(err, ShouldErrLike, `ID is empty`)
		})
		Convey("ID invalid", func() {
			id.ID = "!!!"
			err := id.Validate()
			So(err, ShouldErrLike, `ID is not valid lowercase hexadecimal bytes`)
		})
		Convey("ID not lowercase", func() {
			id.ID = "AA"
			err := id.Validate()
			So(err, ShouldErrLike, `ID is not valid lowercase hexadecimal bytes`)
		})
		Convey("ID too long", func() {
			id.ID = hex.EncodeToString([]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17})
			err := id.Validate()
			So(err, ShouldErrLike, `ID is too long (got 17 bytes, want at most 16 bytes)`)
		})
	})
}
