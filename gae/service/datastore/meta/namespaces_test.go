// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package meta

import (
	"testing"

	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
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
				ctx = info.Get(ctx).MustNamespace(ns)
			}

			err := datastore.Get(ctx).Raw().PutMulti(
				[]*datastore.Key{
					datastore.Get(ctx).NewKey("Warblegarble", "", 1, nil),
				},
				[]datastore.PropertyMap{
					make(datastore.PropertyMap),
				},
				func(*datastore.Key, error) error { return nil })
			if err != nil {
				panic(err)
			}

			datastore.Get(ctx).Testable().CatchupIndexes()
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
