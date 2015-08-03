// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package count

import (
	"fmt"
	"testing"

	"github.com/luci/gae/filter/featureBreaker"
	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/datastore"
	"github.com/luci/gae/service/info"
	"github.com/luci/gae/service/memcache"
	"github.com/luci/gae/service/taskqueue"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestCount(t *testing.T) {
	t.Parallel()

	pnil := func(_ datastore.Key, err error) {
		So(err, ShouldBeNil)
	}

	gnil := func(_ datastore.PropertyMap, err error) {
		So(err, ShouldBeNil)
	}

	Convey("Test Count filter", t, func() {
		c, fb := featureBreaker.FilterRDS(memory.Use(context.Background()), nil)
		c, ctr := FilterRDS(c)

		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)

		ds := datastore.Get(c)

		Convey("Calling a ds function should reflect in counter", func() {
			p := datastore.Property{}
			p.SetValue(100, false)
			keys := []datastore.Key{ds.NewKey("Kind", "", 0, nil)}
			vals := []datastore.PropertyLoadSaver{&datastore.PropertyMap{"Val": {p}}}

			So(ds.PutMulti(keys, vals, pnil), ShouldBeNil)
			So(ctr.NewKey.Successes, ShouldEqual, 1)
			So(ctr.PutMulti.Successes, ShouldEqual, 1)

			Convey("effects are cumulative", func() {
				So(ds.PutMulti(keys, vals, pnil), ShouldBeNil)
				So(ctr.PutMulti.Successes, ShouldEqual, 2)

				Convey("even within transactions", func() {
					root := ds.NewKey("Root", "", 1, nil)
					ds.RunInTransaction(func(c context.Context) error {
						ds := datastore.Get(c)
						keys := []datastore.Key{
							ds.NewKey("Kind", "hi", 0, root),
							ds.NewKey("Kind", "there", 0, root),
						}
						vals = append(vals, vals[0])
						So(ds.PutMulti(keys, vals, pnil), ShouldBeNil)
						return nil
					}, nil)
				})
			})
		})

		Convey("errors count against errors", func() {
			keys := []datastore.Key{ds.NewKey("Kind", "", 1, nil)}
			vals := []datastore.PropertyLoadSaver{&datastore.PropertyMap{"Val": {{}}}}

			fb.BreakFeatures(nil, "GetMulti")

			ds.GetMulti(keys, gnil)
			So(ctr.GetMulti.Errors, ShouldEqual, 1)

			fb.UnbreakFeatures("GetMulti")

			err := ds.PutMulti(keys, vals, func(k datastore.Key, err error) {
				keys[0] = k
				So(err, ShouldBeNil)
			})
			So(err, ShouldBeNil)

			ds.GetMulti(keys, gnil)
			So(ctr.GetMulti.Errors, ShouldEqual, 1)
			So(ctr.GetMulti.Successes, ShouldEqual, 1)
			So(ctr.GetMulti.Total(), ShouldEqual, 2)
		})
	})

	Convey("works for memcache", t, func() {
		c, ctr := FilterMC(memory.Use(context.Background()))
		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)
		mc := memcache.Get(c)

		mc.Set(mc.NewItem("hello").SetValue([]byte("sup")))
		mc.Get("Wat")
		mc.Get("hello")

		So(ctr.Set, ShouldResemble, Entry{1, 0})
		So(ctr.Get, ShouldResemble, Entry{1, 1})
		So(ctr.NewItem, ShouldResemble, Entry{1, 0})
	})

	Convey("works for taskqueue", t, func() {
		c, ctr := FilterTQ(memory.Use(context.Background()))
		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)
		tq := taskqueue.Get(c)

		tq.Add(&taskqueue.Task{Name: "wat"}, "")
		tq.Add(&taskqueue.Task{Name: "wat"}, "DNE_QUEUE")

		So(ctr.Add, ShouldResemble, Entry{1, 1})
	})

	Convey("works for global info", t, func() {
		c, fb := featureBreaker.FilterGI(memory.Use(context.Background()), nil)
		c, ctr := FilterGI(c)
		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)

		gi := info.Get(c)

		gi.Namespace("foo")
		fb.BreakFeatures(nil, "Namespace")
		gi.Namespace("boom")

		So(ctr.Namespace, ShouldResemble, Entry{1, 1})
	})
}

func ExampleFilterRDS() {
	// Set up your context using a base service implementation (memory or prod)
	c := memory.Use(context.Background())

	// Apply the counter.FilterRDS
	c, counter := FilterRDS(c)

	// functions use ds from the context like normal... they don't need to know
	// that there are any filters at all.
	someCalledFunc := func(c context.Context) {
		ds := datastore.Get(c)
		key := ds.NewKey("Kind", "", 1, nil)
		prop := datastore.Property{}
		prop.SetValue(100, false)
		val := datastore.PropertyMap{
			"FieldName": {prop},
		}
		ds.PutMulti([]datastore.Key{key}, []datastore.PropertyLoadSaver{&val}, nil)
	}

	// Using the other function.
	someCalledFunc(c)
	someCalledFunc(c)

	// Then we can see what happened!
	fmt.Printf("%#v\n", counter.NewKey)
	fmt.Printf("%d\n", counter.PutMulti.Successes)
	// Output:
	// count.Entry{Successes:2, Errors:0}
	// 2
}
