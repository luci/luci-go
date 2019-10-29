// Copyright 2019 The LUCI Authors.
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

package internal

import (
	"testing"

	"go.chromium.org/luci/resultdb/internal/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestCursor(t *testing.T) {
	t.Parallel()

	Convey(`Token works`, t, func() {
		c := &internalpb.Cursor{PageSize: 5, Offset: 10}
		tok, err := Token(c)
		So(err, ShouldBeNil)
		So(tok, ShouldResemble, "AAQQBRgK")

		c, err = Cursor("AAQQBRgK")
		So(err, ShouldBeNil)
		So(c, ShouldResembleProto, &internalpb.Cursor{PageSize: 5, Offset: 10})

		Convey(`and errors with wrong version`, func() {
			_, err = Cursor("AQQQBRgK")
			So(err, ShouldErrLike, "unknown cursor version 1")
		})
	})

	Convey(`Fresh cursor works`, t, func() {
		c := NewCursor([]byte("req_0"), 5)
		So(c.Offset, ShouldEqual, 0)

		Convey(`for validating against the correct request`, func() {
			So(ValidateCursor(c, []byte("req_0")), ShouldBeTrue)
			So(ValidateCursor(c, []byte("req_1")), ShouldBeFalse)
		})
	})
}
