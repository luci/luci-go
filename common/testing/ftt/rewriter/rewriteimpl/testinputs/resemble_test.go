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

package testinputs

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestResemble(t *testing.T) {
	t.Parallel()

	Convey("something", t, func() {
		So(0, ShouldResemble, 0)
		So(100, ShouldResemble, 100)

		So("", ShouldResemble, "")

		So(nil, ShouldResemble, nil)

		So("nerb", ShouldResemble, "nerb")

		type myType struct{ f int }
		So(myType{100}, ShouldResemble, myType{100})
	})
}

func TestNotResemble(t *testing.T) {
	t.Parallel()

	Convey("something", t, func() {
		So(1, ShouldNotResemble, 0)
		So(101, ShouldNotResemble, 100)

		So("1", ShouldNotResemble, "")

		So(&(struct{}{}), ShouldNotResemble, nil)

		So("nerb", ShouldNotResemble, "nerb1")

		type myType struct{ f int }
		So(myType{101}, ShouldNotResemble, myType{100})
	})
}
