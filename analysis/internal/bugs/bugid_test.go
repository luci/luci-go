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

package bugs

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidate(t *testing.T) {
	Convey(`Validate`, t, func() {
		id := BugID{
			System: "monorail",
			ID:     "chromium/123",
		}
		Convey(`System missing`, func() {
			id.System = ""
			err := id.Validate()
			So(err, ShouldErrLike, `invalid bug tracking system`)
		})
		Convey("ID invalid", func() {
			id.ID = "!!!"
			err := id.Validate()
			So(err, ShouldErrLike, `invalid monorail bug ID`)
		})
	})
}
