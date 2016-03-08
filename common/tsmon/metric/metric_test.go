// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metric

import (
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/tsmon"
	"github.com/luci/luci-go/common/tsmon/distribution"
	"github.com/luci/luci-go/common/tsmon/store"
	"github.com/luci/luci-go/common/tsmon/target"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestMetrics(t *testing.T) {
	ctx := context.Background()
	defaultTarget := &target.Task{ServiceName: proto.String("default target")}

	Convey("Int", t, func() {
		tsmon.SetStore(store.NewInMemory(defaultTarget))

		m := NewInt("foo", "description")
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

		So(func() { NewInt("foo", "description") }, ShouldPanic)
	})

	Convey("Counter", t, func() {
		tsmon.SetStore(store.NewInMemory(defaultTarget))

		m := NewCounter("foo", "description")
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

		So(func() { NewCounter("foo", "description") }, ShouldPanic)
	})

	Convey("Float", t, func() {
		tsmon.SetStore(store.NewInMemory(defaultTarget))

		m := NewFloat("foo", "description")
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

		So(func() { NewFloat("foo", "description") }, ShouldPanic)
	})

	Convey("FloatCounter", t, func() {
		tsmon.SetStore(store.NewInMemory(defaultTarget))

		m := NewFloatCounter("foo", "description")
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

		So(func() { NewFloatCounter("foo", "description") }, ShouldPanic)
	})

	Convey("String", t, func() {
		tsmon.SetStore(store.NewInMemory(defaultTarget))

		m := NewString("foo", "description")
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

		So(func() { NewString("foo", "description") }, ShouldPanic)
	})

	Convey("Bool", t, func() {
		tsmon.SetStore(store.NewInMemory(defaultTarget))

		m := NewBool("foo", "description")
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

		So(func() { NewBool("foo", "description") }, ShouldPanic)
	})

	Convey("CumulativeDistribution", t, func() {
		tsmon.SetStore(store.NewInMemory(defaultTarget))

		m := NewCumulativeDistribution("foo", "description", distribution.FixedWidthBucketer(10, 20))
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

		So(func() { NewCumulativeDistribution("foo", "description", m.Bucketer()) }, ShouldPanic)
	})

	Convey("NonCumulativeDistribution", t, func() {
		tsmon.SetStore(store.NewInMemory(defaultTarget))

		m := NewNonCumulativeDistribution("foo", "description", distribution.FixedWidthBucketer(10, 20))
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

		So(func() { NewNonCumulativeDistribution("foo", "description", m.Bucketer()) }, ShouldPanic)
	})
}
