// Copyright 2017 The LUCI Authors.
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

package tasktemplate

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	Convey(`Testing substitution resolution`, t, func() {
		var p Params

		Convey(`With no parameters`, func() {
			v, err := p.Resolve("foo")
			So(err, ShouldBeNil)
			So(v, ShouldEqual, "foo")

			_, err = p.Resolve("${swarming_run_id}")
			So(err, ShouldNotBeNil)

			_, err = p.Resolve("${undefined}")
			So(err, ShouldNotBeNil)
		})

		Convey(`With a "swarming_run_id" parameter defined.`, func() {
			p.SwarmingRunID = "12345"

			v, err := p.Resolve("foo")
			So(err, ShouldBeNil)
			So(v, ShouldEqual, "foo")

			v, err = p.Resolve("${swarming_run_id}")
			So(err, ShouldBeNil)
			So(v, ShouldEqual, "12345")

			_, err = p.Resolve("${undefined}")
			So(err, ShouldNotBeNil)
		})
	})
}
