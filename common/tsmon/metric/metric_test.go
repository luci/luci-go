// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metric

import (
	"testing"

	"github.com/luci/luci-go/common/tsmon"
	"github.com/luci/luci-go/common/tsmon/distribution"
	"github.com/luci/luci-go/common/tsmon/store"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMetrics(t *testing.T) {
	ctx := context.Background()

	Convey("Int", t, func() {
		tsmon.SetStore(store.NewInMemory())

		m := NewInt("foo")
		defer tsmon.Unregister(m)

		v, err := m.Get(ctx)
		So(v, ShouldEqual, 0)
		So(err, ShouldBeNil)

		v, err = m.Get(ctx, "field")
		So(err, ShouldNotBeNil)

		err = m.Set(ctx, 42)
		So(err, ShouldBeNil)

		v, err = m.Get(ctx)
		So(v, ShouldEqual, 42)
		So(err, ShouldBeNil)

		err = m.Set(ctx, 42, "field")
		So(err, ShouldNotBeNil)

		So(func() { NewInt("foo") }, ShouldPanic)
	})

	Convey("Counter", t, func() {
		tsmon.SetStore(store.NewInMemory())

		m := NewCounter("foo")
		defer tsmon.Unregister(m)

		v, err := m.Get(ctx)
		So(v, ShouldEqual, 0)
		So(err, ShouldBeNil)

		err = m.Add(ctx, 3)
		So(err, ShouldBeNil)

		v, err = m.Get(ctx)
		So(v, ShouldEqual, 3)
		So(err, ShouldBeNil)

		err = m.Add(ctx, 2)
		So(err, ShouldBeNil)

		v, err = m.Get(ctx)
		So(v, ShouldEqual, 5)
		So(err, ShouldBeNil)

		So(func() { NewCounter("foo") }, ShouldPanic)
	})

	Convey("Float", t, func() {
		tsmon.SetStore(store.NewInMemory())

		m := NewFloat("foo")
		defer tsmon.Unregister(m)

		v, err := m.Get(ctx)
		So(v, ShouldAlmostEqual, 0.0)
		So(err, ShouldBeNil)

		v, err = m.Get(ctx, "field")
		So(err, ShouldNotBeNil)

		err = m.Set(ctx, 42.3)
		So(err, ShouldBeNil)

		v, err = m.Get(ctx)
		So(v, ShouldAlmostEqual, 42.3)
		So(err, ShouldBeNil)

		err = m.Set(ctx, 42.3, "field")
		So(err, ShouldNotBeNil)

		So(func() { NewFloat("foo") }, ShouldPanic)
	})

	Convey("FloatCounter", t, func() {
		tsmon.SetStore(store.NewInMemory())

		m := NewFloatCounter("foo")
		defer tsmon.Unregister(m)

		v, err := m.Get(ctx)
		So(v, ShouldAlmostEqual, 0.0)
		So(err, ShouldBeNil)

		err = m.Add(ctx, 3.1)
		So(err, ShouldBeNil)

		v, err = m.Get(ctx)
		So(v, ShouldAlmostEqual, 3.1)
		So(err, ShouldBeNil)

		err = m.Add(ctx, 2.2)
		So(err, ShouldBeNil)

		v, err = m.Get(ctx)
		So(v, ShouldAlmostEqual, 5.3)
		So(err, ShouldBeNil)

		So(func() { NewFloatCounter("foo") }, ShouldPanic)
	})

	Convey("String", t, func() {
		tsmon.SetStore(store.NewInMemory())

		m := NewString("foo")
		defer tsmon.Unregister(m)

		v, err := m.Get(ctx)
		So(v, ShouldEqual, "")
		So(err, ShouldBeNil)

		v, err = m.Get(ctx, "field")
		So(err, ShouldNotBeNil)

		err = m.Set(ctx, "hello")
		So(err, ShouldBeNil)

		v, err = m.Get(ctx)
		So(v, ShouldEqual, "hello")
		So(err, ShouldBeNil)

		err = m.Set(ctx, "hello", "field")
		So(err, ShouldNotBeNil)

		So(func() { NewString("foo") }, ShouldPanic)
	})

	Convey("Bool", t, func() {
		tsmon.SetStore(store.NewInMemory())

		m := NewBool("foo")
		defer tsmon.Unregister(m)

		v, err := m.Get(ctx)
		So(v, ShouldEqual, false)
		So(err, ShouldBeNil)

		v, err = m.Get(ctx, "field")
		So(err, ShouldNotBeNil)

		err = m.Set(ctx, true)
		So(err, ShouldBeNil)

		v, err = m.Get(ctx)
		So(v, ShouldEqual, true)
		So(err, ShouldBeNil)

		err = m.Set(ctx, true, "field")
		So(err, ShouldNotBeNil)

		So(func() { NewBool("foo") }, ShouldPanic)
	})

	Convey("CumulativeDistribution", t, func() {
		tsmon.SetStore(store.NewInMemory())

		m := NewCumulativeDistribution("foo", distribution.FixedWidthBucketer(10, 20))
		defer tsmon.Unregister(m)

		v, err := m.Get(ctx)
		So(v, ShouldBeNil)
		So(err, ShouldBeNil)

		So(m.Bucketer().GrowthFactor(), ShouldEqual, 0)
		So(m.Bucketer().Width(), ShouldEqual, 10)
		So(m.Bucketer().NumFiniteBuckets(), ShouldEqual, 20)

		v, err = m.Get(ctx, "field")
		So(err, ShouldNotBeNil)

		err = m.Add(ctx, 5)
		So(err, ShouldBeNil)

		v, err = m.Get(ctx)
		So(v.Bucketer().GrowthFactor(), ShouldEqual, 0)
		So(v.Bucketer().Width(), ShouldEqual, 10)
		So(v.Bucketer().NumFiniteBuckets(), ShouldEqual, 20)
		So(v.Sum(), ShouldEqual, 5)
		So(v.Count(), ShouldEqual, 1)
		So(err, ShouldBeNil)

		So(func() { NewCumulativeDistribution("foo", m.Bucketer()) }, ShouldPanic)
	})

	Convey("NonCumulativeDistribution", t, func() {
		tsmon.SetStore(store.NewInMemory())

		m := NewNonCumulativeDistribution("foo", distribution.FixedWidthBucketer(10, 20))
		defer tsmon.Unregister(m)

		v, err := m.Get(ctx)
		So(v, ShouldBeNil)
		So(err, ShouldBeNil)

		So(m.Bucketer().GrowthFactor(), ShouldEqual, 0)
		So(m.Bucketer().Width(), ShouldEqual, 10)
		So(m.Bucketer().NumFiniteBuckets(), ShouldEqual, 20)

		v, err = m.Get(ctx, "field")
		So(err, ShouldNotBeNil)

		d := distribution.New(m.Bucketer())
		d.Add(15)
		err = m.Set(ctx, d)
		So(err, ShouldBeNil)

		v, err = m.Get(ctx)
		So(v.Bucketer().GrowthFactor(), ShouldEqual, 0)
		So(v.Bucketer().Width(), ShouldEqual, 10)
		So(v.Bucketer().NumFiniteBuckets(), ShouldEqual, 20)
		So(v.Sum(), ShouldEqual, 15)
		So(v.Count(), ShouldEqual, 1)
		So(err, ShouldBeNil)

		So(func() { NewNonCumulativeDistribution("foo", m.Bucketer()) }, ShouldPanic)
	})
}
