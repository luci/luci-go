// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package count

import (
	"fmt"
	"testing"

	"github.com/luci/gae/filter/featureBreaker"
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

	pnil := func(_ rawdatastore.Key, err error) {
		So(err, ShouldBeNil)
	}

	gnil := func(_ rawdatastore.PropertyMap, err error) {
		So(err, ShouldBeNil)
	}

	Convey("Test Count filter", t, func() {
		c, fb := featureBreaker.FilterRDS(memory.Use(context.Background()), nil)
		c, ctr := FilterRDS(c)

		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)

		rds := rawdatastore.Get(c)

		Convey("Calling a rds function should reflect in counter", func() {
			p := rawdatastore.Property{}
			p.SetValue(100, false)
			keys := []rawdatastore.Key{rds.NewKey("Kind", "", 0, nil)}
			vals := []rawdatastore.PropertyLoadSaver{&rawdatastore.PropertyMap{"Val": {p}}}

			So(rds.PutMulti(keys, vals, pnil), ShouldBeNil)
			So(ctr.NewKey.Successes, ShouldEqual, 1)
			So(ctr.PutMulti.Successes, ShouldEqual, 1)

			Convey("effects are cumulative", func() {
				So(rds.PutMulti(keys, vals, pnil), ShouldBeNil)
				So(ctr.PutMulti.Successes, ShouldEqual, 2)

				Convey("even within transactions", func() {
					root := rds.NewKey("Root", "", 1, nil)
					rds.RunInTransaction(func(c context.Context) error {
						rds := rawdatastore.Get(c)
						keys := []rawdatastore.Key{
							rds.NewKey("Kind", "hi", 0, root),
							rds.NewKey("Kind", "there", 0, root),
						}
						vals = append(vals, vals[0])
						So(rds.PutMulti(keys, vals, pnil), ShouldBeNil)
						return nil
					}, nil)
				})
			})
		})

		Convey("errors count against errors", func() {
			keys := []rawdatastore.Key{rds.NewKey("Kind", "", 1, nil)}
			vals := []rawdatastore.PropertyLoadSaver{&rawdatastore.PropertyMap{"Val": {{}}}}

			fb.BreakFeatures(nil, "GetMulti")

			rds.GetMulti(keys, gnil)
			So(ctr.GetMulti.Errors, ShouldEqual, 1)

			fb.UnbreakFeatures("GetMulti")

			err := rds.PutMulti(keys, vals, func(k rawdatastore.Key, err error) {
				keys[0] = k
				So(err, ShouldBeNil)
			})
			So(err, ShouldBeNil)

			rds.GetMulti(keys, gnil)
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
		rds.PutMulti([]rawdatastore.Key{key}, []rawdatastore.PropertyLoadSaver{&val}, nil)
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
