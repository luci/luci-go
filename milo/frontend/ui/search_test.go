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

package ui

import (
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

func TestSearchSorting(t *testing.T) {
	t.Parallel()

	Convey("Sort search page by bucket & builder", t, func() {
		Convey("Sort CIService by builder group", func() {
			c := CIService{
				BuilderGroups: []BuilderGroup{
					{Name: "b"},
					{Name: "a"},
					{Name: "d"},
					{Name: "c"},
				},
			}
			c.Sort()
			So(c, ShouldResemble, CIService{
				BuilderGroups: []BuilderGroup{
					{Name: "a"},
					{Name: "b"},
					{Name: "c"},
					{Name: "d"},
				},
			})
		})

		Convey("Sort BuilderGroup by builder name", func() {
			link := func(n string) Link {
				return *NewLink(n, "https://example.com"+n, n)
			}
			b := BuilderGroup{Builders: []Link{
				link("win"),
				link("mac"),
				link("Ubuntu-14.04"),
				link("ubuntu-16.04"),
			}}
			b.Sort()
			So(b, ShouldResemble, BuilderGroup{Builders: []Link{
				link("mac"),
				link("Ubuntu-14.04"),
				link("ubuntu-16.04"),
				link("win"),
			}})
		})
	})
}
