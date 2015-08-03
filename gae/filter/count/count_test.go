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

	Convey("Test Count filter", t, func() {
		c, fb := featureBreaker.FilterRDS(memory.Use(context.Background()), nil)
		c, ctr := FilterRDS(c)

		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)

		ds := datastore.Get(c)
		vals := []datastore.PropertyMap{{
			"Val":  {datastore.MkProperty(100)},
			"$key": {datastore.MkPropertyNI(ds.NewKey("Kind", "", 1, nil))},
		}}

		Convey("Calling a ds function should reflect in counter", func() {
			So(ds.PutMulti(vals), ShouldBeNil)
			So(ctr.NewKey.Successes, ShouldEqual, 1)
			So(ctr.PutMulti.Successes, ShouldEqual, 1)

			Convey("effects are cumulative", func() {
				So(ds.PutMulti(vals), ShouldBeNil)
				So(ctr.PutMulti.Successes, ShouldEqual, 2)

				Convey("even within transactions", func() {
					ds.RunInTransaction(func(c context.Context) error {
						ds := datastore.Get(c)
						So(ds.PutMulti(append(vals, vals[0])), ShouldBeNil)
						return nil
					}, nil)
				})
			})
		})

		Convey("errors count against errors", func() {
			fb.BreakFeatures(nil, "GetMulti")

			ds.GetMulti(vals)
			So(ctr.GetMulti.Errors, ShouldEqual, 1)

			fb.UnbreakFeatures("GetMulti")

			So(ds.PutMulti(vals), ShouldBeNil)

			ds.GetMulti(vals)
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
		vals := []datastore.PropertyMap{{
			"FieldName": {datastore.MkProperty(100)},
			"$key":      {datastore.MkProperty(ds.NewKey("Kind", "", 1, nil))}},
		}
		if err := ds.PutMulti(vals); err != nil {
			panic(err)
		}
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
