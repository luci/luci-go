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
			assert.Loosely(t, datastore.Put(ctx, &Package{
				Name:   name,
				Hidden: hidden,
			}), should.BeNil)
		}

		list := func(prefix string, includeHidden bool) []string {
			p, err := ListPackages(ctx, prefix, includeHidden)
			assert.Loosely(t, err, should.BeNil)
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

		t.Run("Root listing, including hidden", func(t *ftt.Test) {
			assert.Loosely(t, list("", true), should.Match([]string{
				"a", "c/a/b", "c/a/d", "c/a/h", "ca", "d", "d/a", "h1", "h2/a", "h2/b",
			}))
		})

		t.Run("Root listing, skipping hidden", func(t *ftt.Test) {
			assert.Loosely(t, list("", false), should.Match([]string{
				"a", "c/a/b", "c/a/d", "ca", "d", "d/a",
			}))
		})

		t.Run("Subprefix listing, including hidden", func(t *ftt.Test) {
			assert.Loosely(t, list("c", true), should.Match([]string{
				"c/a/b", "c/a/d", "c/a/h",
			}))
		})

		t.Run("Subprefix listing, skipping hidden", func(t *ftt.Test) {
			assert.Loosely(t, list("c", false), should.Match([]string{
				"c/a/b", "c/a/d",
			}))
		})

		t.Run("Actual package is not a subprefix", func(t *ftt.Test) {
			assert.Loosely(t, list("a", true), should.HaveLength(0))
		})

		t.Run("Completely hidden prefix is not listed", func(t *ftt.Test) {
			assert.Loosely(t, list("h2", false), should.HaveLength(0))
		})
	})
}

func TestCheckPackages(t *testing.T) {
	t.Parallel()

	ftt.Run("With datastore", t, func(t *ftt.Test) {
		ctx, _, _ := testutil.TestingContext()

		mk := func(name string, hidden bool) {
			assert.Loosely(t, datastore.Put(ctx, &Package{
				Name:   name,
				Hidden: hidden,
			}), should.BeNil)
		}

		check := func(names []string, includeHidden bool) []string {
			p, err := CheckPackages(ctx, names, includeHidden)
			assert.Loosely(t, err, should.BeNil)
			return p
		}

		mk("a", Visible)
		mk("b", Hidden)
		mk("c", Visible)

		t.Run("Empty list", func(t *ftt.Test) {
			assert.Loosely(t, check(nil, true), should.HaveLength(0))
		})

		t.Run("One visible package", func(t *ftt.Test) {
			assert.Loosely(t, check([]string{"a"}, true), should.Match([]string{"a"}))
		})

		t.Run("One hidden package", func(t *ftt.Test) {
			assert.Loosely(t, check([]string{"b"}, true), should.Match([]string{"b"}))
			assert.Loosely(t, check([]string{"b"}, false), should.Match([]string{}))
		})

		t.Run("One missing package", func(t *ftt.Test) {
			assert.Loosely(t, check([]string{"zzz"}, true), should.Match([]string{}))
		})

		t.Run("Skips missing", func(t *ftt.Test) {
			assert.Loosely(t, check([]string{"zzz", "a", "c", "b"}, true), should.Match([]string{"a", "c", "b"}))
		})

		t.Run("Skips hidden", func(t *ftt.Test) {
			assert.Loosely(t, check([]string{"a", "b", "c"}, false), should.Match([]string{"a", "c"}))
		})

		t.Run("CheckPackageExists also works", func(t *ftt.Test) {
			t.Run("Visible pkg", func(t *ftt.Test) {
				assert.Loosely(t, CheckPackageExists(ctx, "a"), should.BeNil)
			})
			t.Run("Hidden pkg", func(t *ftt.Test) {
				assert.Loosely(t, CheckPackageExists(ctx, "b"), should.BeNil)
			})
			t.Run("Missing pkg", func(t *ftt.Test) {
				err := CheckPackageExists(ctx, "zzz")
				assert.Loosely(t, grpcutil.Code(err), should.Equal(codes.NotFound))
				assert.Loosely(t, err, should.ErrLike("no such package"))
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
			assert.Loosely(t, datastore.Get(ctx, &pkg), should.BeNil)
			return pkg.Hidden
		}
		hide := func(p string) error { return SetPackageHidden(ctx, p, Hidden) }
		show := func(p string) error { return SetPackageHidden(ctx, p, Visible) }

		assert.Loosely(t, hide("a"), should.Equal(datastore.ErrNoSuchEntity))

		datastore.Put(ctx, &Package{Name: "a"})
		assert.Loosely(t, isHidden("a"), should.BeFalse)

		assert.Loosely(t, hide("a"), should.BeNil)
		assert.Loosely(t, isHidden("a"), should.BeTrue)

		tc.Add(time.Second)
		assert.Loosely(t, show("a"), should.BeNil)
		assert.Loosely(t, isHidden("a"), should.BeFalse)

		// To test pre-txn check.
		assert.Loosely(t, show("a"), should.BeNil)
		assert.Loosely(t, isHidden("a"), should.BeFalse)

		assert.Loosely(t, GetEvents(ctx), should.Match([]*repopb.Event{
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
