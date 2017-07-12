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

package stringtemplate

import (
	"testing"

	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
)

func TestResolve(t *testing.T) {
	t.Parallel()

	Convey(`Testing substitution resolution`, t, func() {
		shouldResolve := func(v string, subst map[string]string) string {
			val, err := Resolve(v, subst)
			So(err, ShouldBeNil)
			return val
		}

		resolveErr := func(v string) error {
			_, err := Resolve(v, nil)
			return err
		}

		Convey(`Can resolve a string without any substitutions.`, func() {
			So(shouldResolve("", nil), ShouldEqual, "")
			So(shouldResolve("foo", nil), ShouldEqual, "foo")
			So(shouldResolve(`$${foo}`, nil), ShouldEqual, `${foo}`)
		})

		Convey(`Will error if there is an invalid substitution.`, func() {
			So(resolveErr("$"), ShouldErrLike, "invalid template")
			So(resolveErr("hello$"), ShouldErrLike, "invalid template")
			So(resolveErr("foo-${bar"), ShouldErrLike, "invalid template")
			So(resolveErr("${not valid}"), ShouldErrLike, "invalid template")

			So(resolveErr("${uhoh}"), ShouldErrLike, "no substitution for")
			So(resolveErr("$noooo"), ShouldErrLike, "no substitution for")
		})

		Convey(`With substitutions defined, can apply.`, func() {
			m := map[string]string{
				"pants":      "shirt",
				"wear_pants": "12345",
			}

			So(shouldResolve("", m), ShouldEqual, "")

			So(shouldResolve("$pants", m), ShouldEqual, "shirt")
			So(shouldResolve("foo/$pants", m), ShouldEqual, "foo/shirt")

			So(shouldResolve("${wear_pants}", m), ShouldEqual, "12345")
			So(shouldResolve("foo/${wear_pants}", m), ShouldEqual, "foo/12345")
			So(shouldResolve("foo/${wear_pants}/bar", m), ShouldEqual, "foo/12345/bar")
			So(shouldResolve("${wear_pants}/bar", m), ShouldEqual, "12345/bar")
			So(shouldResolve("foo/${wear_pants}/bar/${wear_pants}/baz", m), ShouldEqual, "foo/12345/bar/12345/baz")

			So(shouldResolve(`$$pants`, m), ShouldEqual, "$pants")
			So(shouldResolve(`$${pants}`, m), ShouldEqual, "${pants}")
			So(shouldResolve(`$$$pants`, m), ShouldEqual, "$shirt")
			So(shouldResolve(`$$${pants}`, m), ShouldEqual, "$shirt")
			So(shouldResolve(`foo/${wear_pants}/bar/$${wear_pants}/baz`, m), ShouldEqual, `foo/12345/bar/${wear_pants}/baz`)
		})
	})
}
