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

package tumble

import (
	"testing"

	"golang.org/x/net/context"

	ds "github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	tq "github.com/luci/gae/service/taskqueue"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetDatastoreNamespaces(t *testing.T) {
	t.Parallel()

	Convey(`A testing datastore`, t, func() {
		tt := &Testing{}
		ctx := tt.Context()

		// Call to add a datastore entry under the supplied namespace.
		addNamespace := func(ns string) {
			c := info.MustNamespace(ctx, ns)

			err := ds.Raw(c).PutMulti(
				[]*ds.Key{
					ds.NewKey(c, "Warblegarble", "", 1, nil),
				},
				[]ds.PropertyMap{
					make(ds.PropertyMap),
				},
				func(*ds.Key, error) error { return nil })
			if err != nil {
				panic(err)
			}

			ds.GetTestable(ctx).CatchupIndexes()
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

func TestFireAllTasks(t *testing.T) {
	t.Parallel()

	Convey("FireAllTasks", t, func() {
		tt := &Testing{}
		c := tt.Context()
		s := &Service{}

		Convey("with no work is a noop", func() {
			So(s.FireAllTasks(c), ShouldBeNil)

			for _, tsks := range tq.GetTestable(c).GetScheduledTasks() {
				So(tsks, ShouldBeEmpty)
			}
		})

		Convey("with some work emits a task", func() {
			So(ds.Put(c, &realMutation{ID: "bogus", Parent: ds.MakeKey(c, "Parent", 1)}), ShouldBeNil)

			So(s.FireAllTasks(c), ShouldBeNil)
			So(tq.GetTestable(c).GetScheduledTasks()["tumble"], ShouldHaveLength, 1)
		})

		Convey("with some work in a different namespaces emits a task for each namespace", func() {
			namespaces := []string{"first", "other"}
			s.Namespaces = func(context.Context) ([]string, error) { return namespaces, nil }

			for _, ns := range namespaces {
				c := info.MustNamespace(c, ns)
				So(ds.Put(c, &realMutation{ID: "bogus", Parent: ds.MakeKey(c, "Parent", 1)}), ShouldBeNil)
			}

			So(s.FireAllTasks(c), ShouldBeNil)
			for _, ns := range namespaces {
				So(tq.GetTestable(info.MustNamespace(c, ns)).GetScheduledTasks()["tumble"], ShouldHaveLength, 1)
			}
		})
	})
}
