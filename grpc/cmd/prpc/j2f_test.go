// Copyright 2016 The LUCI Authors.
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

package main

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestEscape(t *testing.T) {
	t.Parallel()

	Convey("escape", t, func() {
		test := func(s, expected string) {
			Convey(s, func() {
				So(escapeFlag(s), ShouldEqual, expected)
			})
		}

		test("a", `a`)
		test("a b", `'a b'`)
		test("a\nb", "a\\\nb")
		test("a'b", `a\'b`)
		test("a' b", `a\'\ b`)
	})
}
