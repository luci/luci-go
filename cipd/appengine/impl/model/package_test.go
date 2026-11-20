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
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"

	repopb "go.chromium.org/luci/cipd/api/cipd/v1/repopb"
	"go.chromium.org/luci/cipd/appengine/impl/testutil"
)

func TestListPackages(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		ctx, _, _ := testutil.TestingContext()

		mk := func(name string, hidden bool) {
			assert.NoErr(t, datastore.Put(ctx, &Package{
				Name:   name,
				Hidden: hidden,
			}))
		}

		list := func(prefix string, includeHidden bool, exclude ...string) []string {
			var p []string
			for pkg, err := range ListPackages(ctx, prefix, includeHidden, exclude) {
				assert.NoErr(t, err)
				p = append(p, pkg)
			}
			return p
		}

		listErr := func(prefix string, exclude ...string) error {
			for _, err := range ListPackages(ctx, prefix, true, exclude) {
				if err != nil {
					return err
				}
			}
			return nil
		}

		mk("a", Visible)
		mk("c/a", Visible)
		mk("c/a-b", Visible)
		mk("c/a-b/pkg", Visible)
		mk("c/a/b", Visible)
		mk("c/a/b/sub", Visible)
		mk("c/a/d", Visible)
		mk("c/a/h", Hidden)
		mk("c/z", Visible)
		mk("ca", Visible)
		mk("d", Visible)
		mk("d/a", Visible)
		mk("h1", Hidden)
		mk("h2/a", Hidden)
		mk("h2/b", Hidden)
		datastore.GetTestable(ctx).CatchupIndexes()

		t.Run("Root listing, including hidden", func(t *ftt.Test) {
			assert.That(t, list("", true), should.Match([]string{
				"a", "c/a", "c/a-b", "c/a-b/pkg", "c/a/b", "c/a/b/sub", "c/a/d", "c/a/h", "c/z", "ca", "d", "d/a", "h1", "h2/a", "h2/b",
			}))
		})

		t.Run("Root listing, skipping hidden", func(t *ftt.Test) {
			assert.That(t, list("", false), should.Match([]string{
				"a", "c/a", "c/a-b", "c/a-b/pkg", "c/a/b", "c/a/b/sub", "c/a/d", "c/z", "ca", "d", "d/a",
			}))
		})

		t.Run("Subprefix listing, including hidden", func(t *ftt.Test) {
			assert.That(t, list("c", true), should.Match([]string{
				"c/a", "c/a-b", "c/a-b/pkg", "c/a/b", "c/a/b/sub", "c/a/d", "c/a/h", "c/z",
			}))
		})

		t.Run("Subprefix listing, skipping hidden", func(t *ftt.Test) {
			assert.That(t, list("c", false), should.Match([]string{
				"c/a", "c/a-b", "c/a-b/pkg", "c/a/b", "c/a/b/sub", "c/a/d", "c/z",
			}))
		})

		t.Run("Actual package is not a subprefix", func(t *ftt.Test) {
			assert.Loosely(t, list("a", true), should.HaveLength(0))
		})

		t.Run("Completely hidden prefix is not listed", func(t *ftt.Test) {
			assert.Loosely(t, list("h2", false), should.HaveLength(0))
		})

		t.Run("Exclusions from root", func(t *ftt.Test) {
			// Excludes exact package "d" and child "d/a", plus subtree "c/a".
			assert.That(t, list("", false, "d", "c/a"), should.Match([]string{
				"a", "c/a-b", "c/a-b/pkg", "c/z", "ca",
			}))
		})

		t.Run("Exclusions from subprefix", func(t *ftt.Test) {
			// Excludes exact package "c/a" and its children, keeping "c/a-b", "c/a-b/pkg", and "c/z".
			assert.That(t, list("c", false, "c/a"), should.Match([]string{
				"c/a-b", "c/a-b/pkg", "c/z",
			}))

			// Excludes "c/a-b" and its children, keeping "c/a" and its children.
			assert.That(t, list("c", false, "c/a-b"), should.Match([]string{
				"c/a", "c/a/b", "c/a/b/sub", "c/a/d", "c/z",
			}))
		})

		t.Run("Exclusions with '-' vs '/' sort order and duplicates", func(t *ftt.Test) {
			// "c/a" < "c/a-b" as raw strings, but "c/a-b/" < "c/a/" in key order.
			assert.That(t, list("c", false, "c/a", "c/a-b", "c/a/"), should.Match([]string{
				"c/z",
			}))
			assert.That(t, list("c", false, "c/a-b", "c/a"), should.Match([]string{
				"c/z",
			}))
		})

		t.Run("Nested exclusions", func(t *ftt.Test) {
			// "c/a" already implies "c/a/b" and "c/a/d"; ensure nested siblings do
			// not cause ErrNullQuery or rewind lowKey.
			assert.That(t, list("c", false, "c/a/b", "c/a/d", "c/a"), should.Match([]string{
				"c/a-b", "c/a-b/pkg", "c/z",
			}))
		})

		t.Run("Early break", func(t *ftt.Test) {
			var got []string
			for pkg, err := range ListPackages(ctx, "c", false, []string{"c/a-b"}) {
				assert.NoErr(t, err)
				got = append(got, pkg)
				if len(got) == 2 {
					break
				}
			}
			assert.That(t, got, should.Match([]string{"c/a", "c/a/b"}))
		})

		t.Run("Validation errors", func(t *ftt.Test) {
			assert.ErrIsLike(t, listErr("BAD"), "invalid package prefix")
			assert.ErrIsLike(t, listErr("c", "c/BAD"), "invalid package prefix")
			assert.ErrIsLike(t, listErr("", ""), `excludeSubprefixes: "" does not have "" as a prefix`)
			assert.ErrIsLike(t, listErr("c", ""), `excludeSubprefixes: "" does not have "c/" as a prefix`)
			assert.ErrIsLike(t, listErr("c", "c"), `excludeSubprefixes: "c" does not have "c/" as a prefix`)
			assert.ErrIsLike(t, listErr("c", "c/"), `excludeSubprefixes: "c" does not have "c/" as a prefix`)
			assert.ErrIsLike(t, listErr("c", "ca"), `excludeSubprefixes: "ca" does not have "c/" as a prefix`)
			assert.ErrIsLike(t, listErr("c", "d/a"), `excludeSubprefixes: "d/a" does not have "c/" as a prefix`)
		})
	})
}

func TestCheckPackages(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		ctx, _, _ := testutil.TestingContext()

		mk := func(name string, hidden bool) {
			assert.NoErr(t, datastore.Put(ctx, &Package{
				Name:   name,
				Hidden: hidden,
			}))
		}

		check := func(names []string, includeHidden bool) []string {
			p, err := CheckPackages(ctx, names, includeHidden)
			assert.NoErr(t, err)
			return p
		}

		mk("a", Visible)
		mk("b", Hidden)
		mk("c", Visible)

		t.Run("Empty list", func(t *ftt.Test) {
			assert.Loosely(t, check(nil, true), should.HaveLength(0))
		})

		t.Run("One visible package", func(t *ftt.Test) {
			assert.That(t, check([]string{"a"}, true), should.Match([]string{"a"}))
		})

		t.Run("One hidden package", func(t *ftt.Test) {
			assert.That(t, check([]string{"b"}, true), should.Match([]string{"b"}))
			assert.That(t, check([]string{"b"}, false), should.Match([]string{}))
		})

		t.Run("One missing package", func(t *ftt.Test) {
			assert.That(t, check([]string{"zzz"}, true), should.Match([]string{}))
		})

		t.Run("Skips missing", func(t *ftt.Test) {
			assert.That(t, check([]string{"zzz", "a", "c", "b"}, true), should.Match([]string{"a", "c", "b"}))
		})

		t.Run("Skips hidden", func(t *ftt.Test) {
			assert.That(t, check([]string{"a", "b", "c"}, false), should.Match([]string{"a", "c"}))
		})

		t.Run("CheckPackageExists also works", func(t *ftt.Test) {
			t.Run("Visible pkg", func(t *ftt.Test) {
				assert.NoErr(t, CheckPackageExists(ctx, "a"))
			})
			t.Run("Hidden pkg", func(t *ftt.Test) {
				assert.NoErr(t, CheckPackageExists(ctx, "b"))
			})
			t.Run("Missing pkg", func(t *ftt.Test) {
				err := CheckPackageExists(ctx, "zzz")
				assert.That(t, grpcutil.Code(err), should.Equal(codes.NotFound))
				assert.ErrIsLike(t, err, "no such package")
			})
		})
	})
}

func TestSetPackageHidden(t *testing.T) {
	t.Parallel()

	ftt.Run("Works", t, func(t *ftt.Test) {
		ctx, tc, _ := testutil.TestingContext()

		isHidden := func(p string) bool {
			pkg := Package{Name: p}
			assert.NoErr(t, datastore.Get(ctx, &pkg))
			return pkg.Hidden
		}
		hide := func(p string) error { return SetPackageHidden(ctx, p, Hidden) }
		show := func(p string) error { return SetPackageHidden(ctx, p, Visible) }

		assert.That(t, hide("a"), should.Equal(datastore.ErrNoSuchEntity))

		datastore.Put(ctx, &Package{Name: "a"})
		assert.That(t, isHidden("a"), should.BeFalse)

		assert.NoErr(t, hide("a"))
		assert.That(t, isHidden("a"), should.BeTrue)

		tc.Add(time.Second)
		assert.NoErr(t, show("a"))
		assert.That(t, isHidden("a"), should.BeFalse)

		// To test pre-txn check.
		assert.NoErr(t, show("a"))
		assert.That(t, isHidden("a"), should.BeFalse)

		assert.That(t, GetEvents(ctx), should.Match([]*repopb.Event{
			{
				Kind:    repopb.EventKind_PACKAGE_UNHIDDEN,
				Package: "a",
				Who:     string(testutil.TestUser),
				When:    timestamppb.New(testutil.TestTime.Add(time.Second)),
			},
			{
				Kind:    repopb.EventKind_PACKAGE_HIDDEN,
				Package: "a",
				Who:     string(testutil.TestUser),
				When:    timestamppb.New(testutil.TestTime),
			},
		}))
	})
}
