// Copyright 2015 The LUCI Authors.
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

package dm

import (
	"testing"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestExecutionState(t *testing.T) {
	t.Parallel()

	Convey("Evolve", t, func() {
		Convey("Identity", func() {
			s := Execution_SCHEDULING
			So(s.Evolve(Execution_SCHEDULING), ShouldBeNil)
			So(s, ShouldEqual, Execution_SCHEDULING)
		})

		Convey("Transition", func() {
			s := Execution_RUNNING
			So(s.Evolve(Execution_STOPPING), ShouldBeNil)
			So(s, ShouldEqual, Execution_STOPPING)
		})

		Convey("Invalid starting transistion", func() {
			s := Execution_FINISHED
			So(s.Evolve(Execution_SCHEDULING), ShouldErrLike, "invalid state transition FINISHED -> SCHEDULING")
			So(s, ShouldEqual, Execution_FINISHED)
		})

		Convey("Invalid ending transistion", func() {
			s := Execution_RUNNING
			So(s.Evolve(Execution_SCHEDULING), ShouldErrLike, "invalid state transition RUNNING -> SCHEDULING")
			So(s, ShouldEqual, Execution_RUNNING)
		})

		Convey("MustEvolve", func() {
			s := Execution_FINISHED
			So(func() { s.MustEvolve(Execution_SCHEDULING) }, ShouldPanic)
		})
	})
}
