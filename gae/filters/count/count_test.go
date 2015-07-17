// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package count

import (
	"fmt"
	"golang.org/x/net/context"
	"testing"

	"infra/gae/libs/gae"
	"infra/gae/libs/gae/filters/featureBreaker"
	"infra/gae/libs/gae/memory"

	. "github.com/smartystreets/goconvey/convey"
)

func TestCount(t *testing.T) {
	t.Parallel()

	Convey("Test Count filter", t, func() {
		c, ctr := FilterRDS(memory.Use(context.Background()))

		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)

		rds := gae.GetRDS(c)

		Convey("Calling a rds function should reflect in counter", func() {
			p := gae.DSProperty{}
			p.SetValue(100, false)
			_, err := rds.Put(rds.NewKey("Kind", "", 0, nil), &gae.DSPropertyMap{
				"Val": {p},
			})
			So(err, ShouldBeNil)
			So(ctr.NewKey.Successes, ShouldEqual, 1)
			So(ctr.Put.Successes, ShouldEqual, 1)

			Convey("effects are cumulative", func() {
				_, err := rds.Put(rds.NewKey("Kind", "", 0, nil), &gae.DSPropertyMap{
					"Val": {p},
				})
				So(err, ShouldBeNil)
				So(ctr.NewKey.Successes, ShouldEqual, 2)
				So(ctr.Put.Successes, ShouldEqual, 2)

				Convey("even within transactions", func() {
					rds.RunInTransaction(func(c context.Context) error {
						rds := gae.GetRDS(c)
						k := rds.NewKey("Wat", "sup", 0, nil)
						rds.Put(k, &gae.DSPropertyMap{"Wat": {p}})
						rds.Put(k, &gae.DSPropertyMap{"Wat": {p}})
						return nil
					}, nil)
				})
			})
		})
		Convey("errors count against errors", func() {
			rds.Get(nil, nil)
			So(ctr.Get.Errors, ShouldEqual, 1)
			k, err := rds.Put(rds.NewKey("Kind", "", 0, nil), &gae.DSPropertyMap{
				"Val": {gae.DSProperty{}},
			})
			So(err, ShouldBeNil)
			So(ctr.NewKey.Successes, ShouldEqual, 1)
			rds.Get(k, &gae.DSPropertyMap{})
			So(ctr.Get.Errors, ShouldEqual, 1)
			So(ctr.Get.Successes, ShouldEqual, 1)
			So(ctr.Get.Total(), ShouldEqual, 2)
		})
	})

	Convey("works for memcache", t, func() {
		c, ctr := FilterMC(memory.Use(context.Background()))
		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)
		mc := gae.GetMC(c)

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
		tq := gae.GetTQ(c)

		tq.Add(&gae.TQTask{Name: "wat"}, "")
		tq.Add(&gae.TQTask{Name: "wat"}, "DNE_QUEUE")

		So(ctr.Add, ShouldResemble, Entry{1, 1})
	})

	Convey("works for global info", t, func() {
		c, fb := featureBreaker.FilterGI(memory.Use(context.Background()), nil)
		c, ctr := FilterGI(c)
		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)

		gi := gae.GetGI(c)

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
		rds := gae.GetRDS(c)
		key := rds.NewKey("Kind", "", 1, nil)
		prop := gae.DSProperty{}
		prop.SetValue(100, false)
		val := gae.DSPropertyMap{
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
