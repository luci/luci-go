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

package permission

import (
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestValidateName(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		So(ValidateName("service.subject.verb"), ShouldBeNil)

		So(ValidateName("service.subject.verb.stuff"), ShouldNotBeNil)
		So(ValidateName("service.subject"), ShouldNotBeNil)
		So(ValidateName("service.subject."), ShouldNotBeNil)
		So(ValidateName("service..verb"), ShouldNotBeNil)
		So(ValidateName(".subject.verb"), ShouldNotBeNil)
		So(ValidateName(""), ShouldNotBeNil)
	})
}

func TestRegister(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		p1 := Register("luci.dev.testing1")
		So(p1.Name(), ShouldEqual, "luci.dev.testing1")
		So(fmt.Sprintf("%s", p1), ShouldEqual, "luci.dev.testing1")
		So(fmt.Sprintf("%q", p1), ShouldEqual, `"luci.dev.testing1"`)

		So(All(), ShouldResemble, []*Permission{p1})

		p1Alias := Register("luci.dev.testing1")
		So(p1Alias, ShouldEqual, p1)

		p2 := Register("luci.dev.testing2")
		So(All(), ShouldResemble, []*Permission{p1, p2})
	})

	Convey("Panics on bad name", t, func() {
		So(func() { Register(".bad.name") }, ShouldPanic)
	})
}
