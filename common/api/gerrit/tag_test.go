// Copyright 2020 The LUCI Authors.
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

package gerrit

import (
	"testing"

	"go.chromium.org/luci/common/data/strpair"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseCLTags(t *testing.T) {
	t.Parallel()

	Convey("ParseCLTags", t, func() {
		Convey("Complete", func() {
			actual := ParseCLTags(`message

commit details...
TAG_ONE=1111
more details...

tag-invalid=invalid_key
Footer: Cool
TAG_TWO=2222
Footer: Awesome
`)
			So(actual, ShouldResemble, strpair.ParseMap([]string{
				"TAG_ONE:1111",
				"TAG_TWO:2222",
			}))
		})
		Convey("Honor tag order", func() {
			actual := ParseCLTags(`TAG_FOO=foo
TAG_FOO=bar
TAG_FOO=baz
`)
			So(actual["TAG_FOO"], ShouldResemble, []string{
				"baz",
				"bar",
				"foo",
			})
		})
	})
}
