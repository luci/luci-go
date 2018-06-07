// Copyright 2018 The LUCI Authors.
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
	"testing"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"

	. "github.com/smartystreets/goconvey/convey"
)

func TestListPackages(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		ctx := gaetesting.TestingContext()

		mk := func(name string, hidden bool) {
			So(datastore.Put(ctx, &Package{
				Name:   name,
				Hidden: hidden,
			}), ShouldBeNil)
		}

		list := func(prefix string, includeHidden bool) []string {
			p, err := ListPackages(ctx, prefix, includeHidden)
			So(err, ShouldBeNil)
			return p
		}

		const hidden = true
		const visible = false

		mk("a", visible)
		mk("c/a/b", visible)
		mk("c/a/d", visible)
		mk("c/a/h", hidden)
		mk("ca", visible)
		mk("d", visible)
		mk("d/a", visible)
		mk("h1", hidden)
		mk("h2/a", hidden)
		mk("h2/b", hidden)
		datastore.GetTestable(ctx).CatchupIndexes()

		Convey("Root listing, including hidden", func() {
			So(list("", true), ShouldResemble, []string{
				"a", "c/a/b", "c/a/d", "c/a/h", "ca", "d", "d/a", "h1", "h2/a", "h2/b",
			})
		})

		Convey("Root listing, skipping hidden", func() {
			So(list("", false), ShouldResemble, []string{
				"a", "c/a/b", "c/a/d", "ca", "d", "d/a",
			})
		})

		Convey("Subprefix listing, including hidden", func() {
			So(list("c", true), ShouldResemble, []string{
				"c/a/b", "c/a/d", "c/a/h",
			})
		})

		Convey("Subprefix listing, skipping hidden", func() {
			So(list("c", false), ShouldResemble, []string{
				"c/a/b", "c/a/d",
			})
		})

		Convey("Actual package is not a subprefix", func() {
			So(list("a", true), ShouldHaveLength, 0)
		})

		Convey("Completely hidden prefix is not listed", func() {
			So(list("h2", false), ShouldHaveLength, 0)
		})
	})
}
