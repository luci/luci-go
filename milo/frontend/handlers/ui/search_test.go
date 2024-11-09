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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestSearchSorting(t *testing.T) {
	t.Parallel()

	ftt.Run("Sort search page by bucket & builder", t, func(t *ftt.Test) {
		t.Run("Sort CIService by builder group", func(t *ftt.Test) {
			c := CIService{
				BuilderGroups: []BuilderGroup{
					{Name: "b"},
					{Name: "a"},
					{Name: "d"},
					{Name: "c"},
				},
			}
			c.Sort()
			assert.Loosely(t, c, should.Resemble(CIService{
				BuilderGroups: []BuilderGroup{
					{Name: "a"},
					{Name: "b"},
					{Name: "c"},
					{Name: "d"},
				},
			}))
		})

		t.Run("Sort BuilderGroup by builder name", func(t *ftt.Test) {
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
			assert.Loosely(t, b, should.Resemble(BuilderGroup{Builders: []Link{
				link("mac"),
				link("Ubuntu-14.04"),
				link("ubuntu-16.04"),
				link("win"),
			}}))
		})
	})
}
