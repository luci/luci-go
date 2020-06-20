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

package buildifier

import (
	"testing"

	"go.chromium.org/luci/common/data/stringset"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNormalizeLintChecks(t *testing.T) {
	t.Parallel()

	diff := func(s1, s2 stringset.Set) []string {
		var d []string
		for _, s := range s1.ToSortedSlice() {
			if !s2.Has(s) {
				d = append(d, s)
			}
		}
		return d
	}

	Convey("Default", t, func() {
		s, err := normalizeLintChecks(nil)
		So(err, ShouldBeNil)
		So(s.ToSortedSlice(), ShouldResemble, defaultChecks().ToSortedSlice())
	})

	Convey("Add some", t, func() {
		s, err := normalizeLintChecks([]string{
			"+some1",
			"+some2",
			"-some1",
		})
		So(err, ShouldBeNil)
		So(diff(s, defaultChecks()), ShouldResemble, []string{"some2"})
	})

	Convey("Remove some", t, func() {
		s, err := normalizeLintChecks([]string{
			"-module-docstring",
		})
		So(err, ShouldBeNil)
		So(diff(defaultChecks(), s), ShouldResemble, []string{"module-docstring"})
	})
}
