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
	"github.com/luci/gae/service/mail"
	"github.com/luci/gae/service/memcache"
	"github.com/luci/gae/service/taskqueue"
	"github.com/luci/gae/service/user"
	. "github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

func shouldHaveSuccessesAndErrors(actual interface{}, expected ...interface{}) string {
	a := actual.(Entry)
	if len(expected) != 2 {
		panic("Invalid number of expected, should be 2 (successes, errors).")
	}
	s, e := expected[0].(int), expected[1].(int)

	if val := a.Successes(); val != s {
		return fmt.Sprintf("Actual successes (%d) don't match expected (%d)", val, s)
	}
	if val := a.Errors(); val != e {
		return fmt.Sprintf("Actual errors (%d) don't match expected (%d)", val, e)
	}
	return ""
}

func die(err error) {
	if err != nil {
		panic(err)
	}
}

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
			So(ctr.PutMulti.Successes(), ShouldEqual, 1)

			Convey("effects are cumulative", func() {
				So(ds.PutMulti(vals), ShouldBeNil)
				So(ctr.PutMulti.Successes(), ShouldEqual, 2)

				Convey("even within transactions", func() {
					die(ds.RunInTransaction(func(c context.Context) error {
						ds := datastore.Get(c)
						So(ds.PutMulti(append(vals, vals[0])), ShouldBeNil)
						return nil
					}, nil))
				})
			})
		})

		Convey("errors count against errors", func() {
			fb.BreakFeatures(nil, "GetMulti")

			So(ds.GetMulti(vals), ShouldErrLike, `"GetMulti" is broken`)
			So(ctr.GetMulti.Errors(), ShouldEqual, 1)

			fb.UnbreakFeatures("GetMulti")

			So(ds.PutMulti(vals), ShouldBeNil)

			die(ds.GetMulti(vals))
			So(ctr.GetMulti.Errors(), ShouldEqual, 1)
			So(ctr.GetMulti.Successes(), ShouldEqual, 1)
			So(ctr.GetMulti.Total(), ShouldEqual, 2)
		})
	})

	Convey("works for memcache", t, func() {
		c, ctr := FilterMC(memory.Use(context.Background()))
		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)
		mc := memcache.Get(c)

		die(mc.Set(mc.NewItem("hello").SetValue([]byte("sup"))))

		_, err := mc.Get("Wat")
		So(err, ShouldNotBeNil)

		_, err = mc.Get("hello")
		die(err)

		So(ctr.SetMulti, shouldHaveSuccessesAndErrors, 1, 0)
		So(ctr.GetMulti, shouldHaveSuccessesAndErrors, 2, 0)
		So(ctr.NewItem, shouldHaveSuccessesAndErrors, 3, 0)
	})

	Convey("works for taskqueue", t, func() {
		c, ctr := FilterTQ(memory.Use(context.Background()))
		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)
		tq := taskqueue.Get(c)

		die(tq.Add(&taskqueue.Task{Name: "wat"}, ""))
		So(tq.Add(&taskqueue.Task{Name: "wat"}, "DNE_QUEUE"),
			ShouldErrLike, "UNKNOWN_QUEUE")

		So(ctr.AddMulti, shouldHaveSuccessesAndErrors, 1, 1)
	})

	Convey("works for global info", t, func() {
		c, fb := featureBreaker.FilterGI(memory.Use(context.Background()), nil)
		c, ctr := FilterGI(c)
		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)

		gi := info.Get(c)

		_, err := gi.Namespace("foo")
		die(err)
		fb.BreakFeatures(nil, "Namespace")
		_, err = gi.Namespace("boom")
		So(err, ShouldErrLike, `"Namespace" is broken`)

		So(ctr.Namespace, shouldHaveSuccessesAndErrors, 1, 1)
	})

	Convey("works for user", t, func() {
		c, fb := featureBreaker.FilterUser(memory.Use(context.Background()), nil)
		c, ctr := FilterUser(c)
		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)

		u := user.Get(c)

		_, err := u.CurrentOAuth("foo")
		die(err)
		fb.BreakFeatures(nil, "CurrentOAuth")
		_, err = u.CurrentOAuth("foo")
		So(err, ShouldErrLike, `"CurrentOAuth" is broken`)

		So(ctr.CurrentOAuth, shouldHaveSuccessesAndErrors, 1, 1)
	})

	Convey("works for mail", t, func() {
		c, fb := featureBreaker.FilterMail(memory.Use(context.Background()), nil)
		c, ctr := FilterMail(c)
		So(c, ShouldNotBeNil)
		So(ctr, ShouldNotBeNil)

		m := mail.Get(c)

		err := m.Send(&mail.Message{
			Sender: "admin@example.com",
			To:     []string{"coolDood@example.com"},
			Body:   "hi",
		})
		die(err)

		fb.BreakFeatures(nil, "Send")
		err = m.Send(&mail.Message{
			Sender: "admin@example.com",
			To:     []string{"coolDood@example.com"},
			Body:   "hi",
		})
		So(err, ShouldErrLike, `"Send" is broken`)

		So(ctr.Send, shouldHaveSuccessesAndErrors, 1, 1)
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
	fmt.Printf("%d\n", counter.PutMulti.Successes())
	// Output:
	// 2
}
