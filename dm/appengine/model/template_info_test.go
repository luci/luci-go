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

package model

import (
	"sort"
	"testing"

	"go.chromium.org/luci/dm/api/service/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTemplateInfo(t *testing.T) {
	t.Parallel()

	Convey("Test TemplateInfo", t, func() {
		ti := TemplateInfo{}

		Convey("empty", func() {
			So(ti.EqualsData(nil), ShouldBeTrue)
			sort.Sort(ti)

			Convey("can add", func() {
				ti.Add(*dm.NewTemplateSpec("proj", "ref", "vers", "name"))
				So(ti[0], ShouldResemble, *dm.NewTemplateSpec("proj", "ref", "vers", "name"))
			})
		})

		Convey("can uniq", func() {
			ti.Add(
				*dm.NewTemplateSpec("a", "b", "c", "d"),
				*dm.NewTemplateSpec("a", "b", "c", "d"),
				*dm.NewTemplateSpec("a", "b", "c", "d"),
				*dm.NewTemplateSpec("a", "b", "c", "d"),
			)
			So(len(ti), ShouldEqual, 1)
		})

		Convey("can sort", func() {
			ti.Add(
				*dm.NewTemplateSpec("z", "b", "c", "d"),
				*dm.NewTemplateSpec("a", "b", "z", "d"),
				*dm.NewTemplateSpec("a", "z", "c", "d"),
				*dm.NewTemplateSpec("a", "b", "c", "z"),
			)
			So(ti, ShouldResemble, TemplateInfo{
				*dm.NewTemplateSpec("a", "b", "c", "z"),
				*dm.NewTemplateSpec("a", "b", "z", "d"),
				*dm.NewTemplateSpec("a", "z", "c", "d"),
				*dm.NewTemplateSpec("z", "b", "c", "d"),
			})
		})

		Convey("equivalence", func() {
			other := []*dm.Quest_TemplateSpec{
				dm.NewTemplateSpec("a", "b", "c", "z"),
				dm.NewTemplateSpec("a", "b", "z", "d"),
				dm.NewTemplateSpec("a", "z", "c", "d"),
				dm.NewTemplateSpec("z", "b", "c", "d"),
			}

			So(ti.EqualsData(other), ShouldBeFalse)

			ti.Add(
				*dm.NewTemplateSpec("z", "b", "c", "d"),
				*dm.NewTemplateSpec("a", "b", "z", "d"),
				*dm.NewTemplateSpec("a", "z", "c", "d"),
				*dm.NewTemplateSpec("a", "b", "c", "z"),
			)
			So(ti.EqualsData(other), ShouldBeTrue)

			ti[3].Project = "Z"
			So(ti.EqualsData(other), ShouldBeFalse)
		})
	})
}
