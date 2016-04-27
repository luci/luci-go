// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tumble

import (
	"testing"

	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	. "github.com/smartystreets/goconvey/convey"
)

func TestGetDatastoreNamespaces(t *testing.T) {
	t.Parallel()

	Convey(`A testing datastore`, t, func() {
		tt := &Testing{}
		ctx := tt.Context()

		// Call to add a datastore entry under the supplied namespace.
		addNamespace := func(ns string) {
			c := info.Get(ctx).MustNamespace(ns)

			err := datastore.Get(c).Raw().PutMulti(
				[]*datastore.Key{
					datastore.Get(c).NewKey("Warblegarble", "", 1, nil),
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
			namespaces, err := getDatastoreNamespaces(ctx)
			So(err, ShouldBeNil)
			So(namespaces, ShouldResemble, []string{})
		})

		Convey(`A datastore with namespaces {"foo", "bar"} will return {"bar", "foo"}.`, func() {
			addNamespace("foo")
			addNamespace("bar")

			namespaces, err := getDatastoreNamespaces(ctx)
			So(err, ShouldBeNil)
			So(namespaces, ShouldResemble, []string{"bar", "foo"})
		})
	})
}
