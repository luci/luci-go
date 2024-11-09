// Copyright 2015 The LUCI Authors.
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

package meta

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/gae/impl/memory"
	ds "go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
)

func TestNamespaces(t *testing.T) {
	t.Parallel()

	ftt.Run(`A testing datastore`, t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		// Call to add a datastore entry under the supplied namespace.
		addNamespace := func(ns string) {
			if ns != "" {
				ctx = info.MustNamespace(ctx, ns)
			}

			err := ds.Raw(ctx).PutMulti(
				[]*ds.Key{
					ds.NewKey(ctx, "Warblegarble", "", 1, nil),
				},
				[]ds.PropertyMap{
					make(ds.PropertyMap),
				},
				func(int, *ds.Key, error) {})
			if err != nil {
				panic(err)
			}

			ds.GetTestable(ctx).CatchupIndexes()
		}

		t.Run(`A datastore with no namespaces returns {}.`, func(t *ftt.Test) {
			var coll NamespacesCollector
			assert.Loosely(t, Namespaces(ctx, coll.Callback), should.BeNil)
			assert.Loosely(t, coll, should.Resemble(NamespacesCollector(nil)))
		})

		t.Run(`With namespaces {<default>, foo, bar, baz-a, baz-b}`, func(t *ftt.Test) {
			addNamespace("")
			addNamespace("foo")
			addNamespace("bar")
			addNamespace("baz-a")
			addNamespace("baz-b")

			t.Run(`Can collect all namespaces.`, func(t *ftt.Test) {
				var coll NamespacesCollector
				assert.Loosely(t, Namespaces(ctx, coll.Callback), should.BeNil)
				assert.Loosely(t, coll, should.Resemble(NamespacesCollector{"", "bar", "baz-a", "baz-b", "foo"}))
			})

			t.Run(`Can get namespaces with prefix "baz-".`, func(t *ftt.Test) {
				var coll NamespacesCollector
				assert.Loosely(t, NamespacesWithPrefix(ctx, "baz-", coll.Callback), should.BeNil)
				assert.Loosely(t, coll, should.Resemble(NamespacesCollector{"baz-a", "baz-b"}))
			})
		})
	})
}
