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

	"google.golang.org/grpc/codes"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/grpc/grpcutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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

		mk("a", Visible)
		mk("c/a/b", Visible)
		mk("c/a/d", Visible)
		mk("c/a/h", Hidden)
		mk("ca", Visible)
		mk("d", Visible)
		mk("d/a", Visible)
		mk("h1", Hidden)
		mk("h2/a", Hidden)
		mk("h2/b", Hidden)
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

func TestCheckPackages(t *testing.T) {
	t.Parallel()

	Convey("With datastore", t, func() {
		ctx := gaetesting.TestingContext()

		mk := func(name string, hidden bool) {
			So(datastore.Put(ctx, &Package{
				Name:   name,
				Hidden: hidden,
			}), ShouldBeNil)
		}

		check := func(names []string, includeHidden bool) []string {
			p, err := CheckPackages(ctx, names, includeHidden)
			So(err, ShouldBeNil)
			return p
		}

		mk("a", Visible)
		mk("b", Hidden)
		mk("c", Visible)

		Convey("Empty list", func() {
			So(check(nil, true), ShouldHaveLength, 0)
		})

		Convey("One visible package", func() {
			So(check([]string{"a"}, true), ShouldResemble, []string{"a"})
		})

		Convey("One hidden package", func() {
			So(check([]string{"b"}, true), ShouldResemble, []string{"b"})
			So(check([]string{"b"}, false), ShouldResemble, []string{})
		})

		Convey("One missing package", func() {
			So(check([]string{"zzz"}, true), ShouldResemble, []string{})
		})

		Convey("Skips missing", func() {
			So(check([]string{"zzz", "a", "c", "b"}, true), ShouldResemble, []string{"a", "c", "b"})
		})

		Convey("Skips hidden", func() {
			So(check([]string{"a", "b", "c"}, false), ShouldResemble, []string{"a", "c"})
		})

		Convey("CheckPackageExists also works", func() {
			Convey("Visible pkg", func() {
				So(CheckPackageExists(ctx, "a"), ShouldBeNil)
			})
			Convey("Hidden pkg", func() {
				So(CheckPackageExists(ctx, "b"), ShouldBeNil)
			})
			Convey("Missing pkg", func() {
				err := CheckPackageExists(ctx, "zzz")
				So(grpcutil.Code(err), ShouldEqual, codes.NotFound)
				So(err, ShouldErrLike, "no such package")
			})
		})
	})
}

func TestSetPackageHidden(t *testing.T) {
	t.Parallel()

	Convey("Works", t, func() {
		ctx := gaetesting.TestingContext()

		isHidden := func(p string) bool {
			pkg := Package{Name: p}
			So(datastore.Get(ctx, &pkg), ShouldBeNil)
			return pkg.Hidden
		}
		hide := func(p string) error { return SetPackageHidden(ctx, p, Hidden) }
		show := func(p string) error { return SetPackageHidden(ctx, p, Visible) }

		So(hide("a"), ShouldEqual, datastore.ErrNoSuchEntity)

		datastore.Put(ctx, &Package{Name: "a"})
		So(isHidden("a"), ShouldBeFalse)

		So(hide("a"), ShouldBeNil)
		So(isHidden("a"), ShouldBeTrue)

		So(show("a"), ShouldBeNil)
		So(isHidden("a"), ShouldBeFalse)

		// To test pre-txn check.
		So(show("a"), ShouldBeNil)
		So(isHidden("a"), ShouldBeFalse)
	})
}
