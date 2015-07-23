// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package count

import (
	"fmt"
	"testing"

	"github.com/luci/gae/filters/featureBreaker"
	"github.com/luci/gae/impl/memory"
	"github.com/luci/gae/service/info"
	"github.com/luci/gae/service/memcache"
	"github.com/luci/gae/service/rawdatastore"
	"github.com/luci/gae/service/taskqueue"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func TestCount(t *testing.T) {
	t.Parallel()

	Convey("Test Count filter", t, func() {
		c, ctr := FilterRDS(memory.Use(context.Background()))

		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)

		rds := rawdatastore.Get(c)

		Convey("Calling a rds function should reflect in counter", func() {
			p := rawdatastore.Property{}
			p.SetValue(100, false)
			_, err := rds.Put(rds.NewKey("Kind", "", 0, nil), &rawdatastore.PropertyMap{
				"Val": {p},
			})
			So(err, ShouldBeNil)
			So(ctr.NewKey.Successes, ShouldEqual, 1)
			So(ctr.Put.Successes, ShouldEqual, 1)

			Convey("effects are cumulative", func() {
				_, err := rds.Put(rds.NewKey("Kind", "", 0, nil), &rawdatastore.PropertyMap{
					"Val": {p},
				})
				So(err, ShouldBeNil)
				So(ctr.NewKey.Successes, ShouldEqual, 2)
				So(ctr.Put.Successes, ShouldEqual, 2)

				Convey("even within transactions", func() {
					rds.RunInTransaction(func(c context.Context) error {
						rds := rawdatastore.Get(c)
						k := rds.NewKey("Wat", "sup", 0, nil)
						rds.Put(k, &rawdatastore.PropertyMap{"Wat": {p}})
						rds.Put(k, &rawdatastore.PropertyMap{"Wat": {p}})
						return nil
					}, nil)
				})
			})
		})
		Convey("errors count against errors", func() {
			rds.Get(nil, nil)
			So(ctr.Get.Errors, ShouldEqual, 1)
			k, err := rds.Put(rds.NewKey("Kind", "", 0, nil), &rawdatastore.PropertyMap{
				"Val": {rawdatastore.Property{}},
			})
			So(err, ShouldBeNil)
			So(ctr.NewKey.Successes, ShouldEqual, 1)
			rds.Get(k, &rawdatastore.PropertyMap{})
			So(ctr.Get.Errors, ShouldEqual, 1)
			So(ctr.Get.Successes, ShouldEqual, 1)
			So(ctr.Get.Total(), ShouldEqual, 2)
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

	// functions use RDS from the context like normal... they don't need to know
	// that there are any filters at all.
	someCalledFunc := func(c context.Context) {
		rds := rawdatastore.Get(c)
		key := rds.NewKey("Kind", "", 1, nil)
		prop := rawdatastore.Property{}
		prop.SetValue(100, false)
		val := rawdatastore.PropertyMap{
			"FieldName": {prop},
		}
		rds.Put(key, &val)
	}

	// Using the other function.
	someCalledFunc(c)
	someCalledFunc(c)

	// Then we can see what happened!
	fmt.Printf("%#v\n", counter.NewKey)
	fmt.Printf("%d\n", counter.Put.Successes)
	// Output:
	// count.Entry{Successes:2, Errors:0}
	// 2
}
