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
		c := &internal.Cursor{Version: curVersion, Offset: 5}
		tok, err := Token(c)
		So(err, ShouldBeNil)
		So(tok, ShouldResemble, "EAU=")
	})

	Convey(`Offset works`, t, func() {
		offset, err := Offset("EAU=")
		So(err, ShouldBeNil)
		So(offset, ShouldEqual, 5)

		Convey(`and fails for unknown version`, func() {
			_, err := Offset("CAEQBQ==")
			So(err, ShouldErrLike, "unknown cursor version")
		})
	})
}
