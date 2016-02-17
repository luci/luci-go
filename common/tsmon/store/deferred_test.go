// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package store

import (
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/tsmon/distribution"
	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/target"
	"github.com/luci/luci-go/common/tsmon/types"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func TestDeferredBase(t *testing.T) {
	ctx := context.Background()
	RunStoreImplementationTests(t, ctx, TestOptions{
		Factory: func() Store {
			return NewDeferred(NewInMemory(&target.Task{ServiceName: proto.String("default target")}))
		},
		RegistrationFinished: func(Store) {},
		GetNumRegisteredMetrics: func(s Store) int {
			return len(s.(*DeferredStore).Store.(*inMemoryStore).data)
		},
	})
}

func TestDeferred(t *testing.T) {
	ctx := context.Background()

	Convey("Deferred set", t, func() {
		s := NewDeferred(NewInMemory(&target.Task{ServiceName: proto.String("default target")}))
		m := &fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType}
		s.Register(m)

		c := s.Start(ctx)

		So(s.Set(c, m, time.Time{}, []interface{}{}, int64(123)), ShouldBeNil)
		v, err := s.Get(c, m, time.Time{}, []interface{}{})
		So(v, ShouldBeNil)
		So(err, ShouldBeNil)

		So(s.Finalize(c), ShouldBeNil)
		v, err = s.Get(c, m, time.Time{}, []interface{}{})
		So(v, ShouldEqual, 123)
		So(err, ShouldBeNil)
	})

	Convey("Deferred incr", t, func() {
		s := NewDeferred(NewInMemory(&target.Task{ServiceName: proto.String("default target")}))
		m := &fakeMetric{"m", []field.Field{}, types.CumulativeIntType}
		s.Register(m)

		c := s.Start(ctx)

		So(s.Incr(c, m, time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)
		v, err := s.Get(c, m, time.Time{}, []interface{}{})
		So(v, ShouldBeNil)
		So(err, ShouldBeNil)

		So(s.Finalize(c), ShouldBeNil)
		v, err = s.Get(c, m, time.Time{}, []interface{}{})
		So(v, ShouldEqual, 1)
		So(err, ShouldBeNil)
	})

	Convey("Deferred set then set", t, func() {
		s := NewDeferred(NewInMemory(&target.Task{ServiceName: proto.String("default target")}))
		m := &fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType}
		s.Register(m)

		c := s.Start(ctx)

		So(s.Set(c, m, time.Time{}, []interface{}{}, int64(42)), ShouldBeNil)
		So(s.Set(c, m, time.Time{}, []interface{}{}, int64(45)), ShouldBeNil)
		So(s.Finalize(c), ShouldBeNil)

		v, err := s.Get(c, m, time.Time{}, []interface{}{})
		So(v, ShouldEqual, 45)
		So(err, ShouldBeNil)
	})

	Convey("Deferred incr then incr", t, func() {
		s := NewDeferred(NewInMemory(&target.Task{ServiceName: proto.String("default target")}))
		m := &fakeMetric{"m", []field.Field{}, types.CumulativeIntType}
		s.Register(m)

		c := s.Start(ctx)

		So(s.Incr(c, m, time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)
		So(s.Incr(c, m, time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)
		So(s.Finalize(c), ShouldBeNil)

		v, err := s.Get(c, m, time.Time{}, []interface{}{})
		So(v, ShouldEqual, 2)
		So(err, ShouldBeNil)
	})

	Convey("Deferred set with fields", t, func() {
		s := NewDeferred(NewInMemory(&target.Task{ServiceName: proto.String("default target")}))
		m := &fakeMetric{"m", []field.Field{field.String("f")}, types.NonCumulativeIntType}
		s.Register(m)

		c := s.Start(ctx)

		So(s.Set(c, m, time.Time{}, makeInterfaceSlice("foo"), int64(1)), ShouldBeNil)
		So(s.Set(c, m, time.Time{}, makeInterfaceSlice("bar"), int64(2)), ShouldBeNil)
		So(s.Finalize(c), ShouldBeNil)

		v, err := s.Get(c, m, time.Time{}, makeInterfaceSlice("foo"))
		So(v, ShouldEqual, 1)
		So(err, ShouldBeNil)

		v, err = s.Get(c, m, time.Time{}, makeInterfaceSlice("bar"))
		So(v, ShouldEqual, 2)
		So(err, ShouldBeNil)
	})

	Convey("Deferred distribution incr", t, func() {
		s := NewDeferred(NewInMemory(&target.Task{ServiceName: proto.String("default target")}))
		m := &fakeDistributionMetric{
			fakeMetric{"m", []field.Field{}, types.CumulativeDistributionType},
			distribution.DefaultBucketer,
		}
		s.Register(m)

		c := s.Start(ctx)

		So(s.Incr(c, m, time.Time{}, []interface{}{}, float64(6)), ShouldBeNil)
		So(s.Finalize(c), ShouldBeNil)

		v, err := s.Get(c, m, time.Time{}, []interface{}{})
		So(err, ShouldBeNil)

		dist := v.(*distribution.Distribution)
		So(dist.Count(), ShouldEqual, 1)
		So(dist.Sum(), ShouldEqual, 6)
	})

	Convey("Deferred distribution incr then incr", t, func() {
		s := NewDeferred(NewInMemory(&target.Task{ServiceName: proto.String("default target")}))
		m := &fakeDistributionMetric{
			fakeMetric{"m", []field.Field{}, types.CumulativeDistributionType},
			distribution.DefaultBucketer,
		}
		s.Register(m)

		c := s.Start(ctx)

		So(s.Incr(c, m, time.Time{}, []interface{}{}, float64(4)), ShouldBeNil)
		So(s.Incr(c, m, time.Time{}, []interface{}{}, float64(1)), ShouldBeNil)
		So(s.Finalize(c), ShouldBeNil)

		v, err := s.Get(c, m, time.Time{}, []interface{}{})
		So(err, ShouldBeNil)

		dist := v.(*distribution.Distribution)
		So(dist.Count(), ShouldEqual, 2)
		So(dist.Sum(), ShouldEqual, 5)
		So(dist.Buckets(), ShouldResemble, []int64{0, 0, 1, 0, 0, 1})
	})
}
