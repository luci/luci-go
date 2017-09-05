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
	"testing"

	"go.chromium.org/gae/impl/memory"
	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/gae/service/info"

	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestNamespaces(t *testing.T) {
	t.Parallel()

	Convey(`A testing datastore`, t, func() {
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
				func(int, *ds.Key, error) error { return nil })
			if err != nil {
				panic(err)
			}

			ds.GetTestable(ctx).CatchupIndexes()
		}

		Convey(`A datastore with no namespaces returns {}.`, func() {
			var coll NamespacesCollector
			So(Namespaces(ctx, coll.Callback), ShouldBeNil)
			So(coll, ShouldResemble, NamespacesCollector(nil))
		})

		Convey(`With namespaces {<default>, foo, bar, baz-a, baz-b}`, func() {
			addNamespace("")
			addNamespace("foo")
			addNamespace("bar")
			addNamespace("baz-a")
			addNamespace("baz-b")

			Convey(`Can collect all namespaces.`, func() {
				var coll NamespacesCollector
				So(Namespaces(ctx, coll.Callback), ShouldBeNil)
				So(coll, ShouldResemble, NamespacesCollector{"", "bar", "baz-a", "baz-b", "foo"})
			})

			Convey(`Can get namespaces with prefix "baz-".`, func() {
				var coll NamespacesCollector
				So(NamespacesWithPrefix(ctx, "baz-", coll.Callback), ShouldBeNil)
				So(coll, ShouldResemble, NamespacesCollector{"baz-a", "baz-b"})
			})
		})
	})
}
