// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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

		Convey("with some work in a different namespace emits a task", func() {
			c = info.MustNamespace(c, "other")
			So(ds.Put(c, &realMutation{ID: "bogus", Parent: ds.MakeKey(c, "Parent", 1)}), ShouldBeNil)

			cfg := tt.GetConfig(c)
			cfg.Namespaced = true
			tt.UpdateSettings(c, cfg)
			s.Namespaces = func(context.Context) ([]string, error) {
				return []string{"other"}, nil
			}
			So(s.FireAllTasks(c), ShouldBeNil)
			So(tq.GetTestable(info.MustNamespace(c, TaskNamespace)).GetScheduledTasks()["tumble"], ShouldHaveLength, 1)
		})
	})
}
