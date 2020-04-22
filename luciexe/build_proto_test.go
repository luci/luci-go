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

package luciexe

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestParseOutputFlag(t *testing.T) {
	t.Parallel()

	Convey(`ParseOutputFlag`, t, func() {
		Convey(`good`, func() {
			Convey(`empty`, func() {
				out, rest, err := ParseOutputFlag(nil)
				So(err, ShouldBeNil)
				So(out, ShouldBeEmpty)
				So(rest, ShouldBeNil)
			})

			Convey(`equal`, func() {
				out, rest, err := ParseOutputFlag([]string{"a", OutputCLIArg + "=out.textpb", "b"})
				So(err, ShouldBeNil)
				So(out, ShouldResemble, "out.textpb")
				So(rest, ShouldResemble, []string{"a", "b"})
			})

			Convey(`two val`, func() {
				out, rest, err := ParseOutputFlag([]string{"a", OutputCLIArg, "out.json", "b"})
				So(err, ShouldBeNil)
				So(out, ShouldResemble, "out.json")
				So(rest, ShouldResemble, []string{"a", "b"})
			})

			Convey(`missing`, func() {
				out, rest, err := ParseOutputFlag([]string{"other", "stuff"})
				So(err, ShouldBeNil)
				So(out, ShouldBeEmpty)
				So(rest, ShouldResemble, []string{"other", "stuff"})
			})

			Convey(`no equal (some other arg)`, func() {
				out, rest, err := ParseOutputFlag([]string{"arg", OutputCLIArg + "junk"})
				So(err, ShouldBeNil)
				So(out, ShouldBeEmpty)
				So(rest, ShouldResemble, []string{"arg", OutputCLIArg + "junk"})
			})

			Convey(`no equal, keeps searching`, func() {
				out, rest, err := ParseOutputFlag([]string{"arg", OutputCLIArg + "junk", OutputCLIArg, "out.pb", "stuff"})
				So(err, ShouldBeNil)
				So(out, ShouldResemble, "out.pb")
				So(rest, ShouldResemble, []string{"arg", OutputCLIArg + "junk", "stuff"})
			})

		})

		Convey(`bad`, func() {
			Convey(`incomplete`, func() {
				out, rest, err := ParseOutputFlag([]string{"arg", OutputCLIArg})
				So(err, ShouldErrLike, "malformed")
				So(out, ShouldBeEmpty)
				So(rest, ShouldResemble, []string{"arg", OutputCLIArg})
			})

			Convey(`bad extension`, func() {
				out, rest, err := ParseOutputFlag([]string{"arg", OutputCLIArg + "=nope"})
				So(err, ShouldErrLike, "bad extension")
				So(out, ShouldResemble, "nope")
				So(rest, ShouldResemble, []string{"arg", OutputCLIArg + "=nope"})
			})
		})
	})
}
