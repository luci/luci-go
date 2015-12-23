// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package store

import (
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/target"
	"github.com/luci/luci-go/common/tsmon/types"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

func makeInterfaceSlice(v ...interface{}) []interface{} {
	return v
}

type fakeMetric struct {
	name   string
	fields []field.Field
	typ    types.ValueType
}

func (m *fakeMetric) Name() string                  { return m.name }
func (m *fakeMetric) Fields() []field.Field         { return m.fields }
func (m *fakeMetric) ValueType() types.ValueType    { return m.typ }
func (m *fakeMetric) SetFixedResetTime(t time.Time) {}

func TestRegisterSetGet(t *testing.T) {
	ctx := context.Background()

	Convey("Registered metric with no fields", t, func() {
		s := NewInMemory()
		h, _ := s.Register(&fakeMetric{"foo", []field.Field{}, types.NonCumulativeIntType})

		Convey("Initial Get should return nil", func() {
			v, err := s.Get(ctx, h, time.Time{}, []interface{}{})
			So(v, ShouldBeNil)
			So(err, ShouldBeNil)
		})

		Convey("Set and Get", func() {
			err := s.Set(ctx, h, time.Time{}, []interface{}{}, "value")
			So(err, ShouldBeNil)

			v, err := s.Get(ctx, h, time.Time{}, []interface{}{})
			So(v, ShouldEqual, "value")
			So(err, ShouldBeNil)
		})
	})

	Convey("Registered metric with a field", t, func() {
		s := NewInMemory()
		h, _ := s.Register(&fakeMetric{"foo", []field.Field{field.String("f")}, types.NonCumulativeIntType})

		Convey("Initial Get should return nil", func() {
			v, err := s.Get(ctx, h, time.Time{}, makeInterfaceSlice("one"))
			So(v, ShouldBeNil)
			So(err, ShouldBeNil)
		})

		Convey("Set and Get", func() {
			So(s.Set(ctx, h, time.Time{}, makeInterfaceSlice("one"), 111), ShouldBeNil)
			So(s.Set(ctx, h, time.Time{}, makeInterfaceSlice("two"), 222), ShouldBeNil)
			So(s.Set(ctx, h, time.Time{}, makeInterfaceSlice(""), 333), ShouldBeNil)

			v, err := s.Get(ctx, h, time.Time{}, makeInterfaceSlice("one"))
			So(v, ShouldEqual, 111)
			So(err, ShouldBeNil)

			v, err = s.Get(ctx, h, time.Time{}, makeInterfaceSlice("two"))
			So(v, ShouldEqual, 222)
			So(err, ShouldBeNil)

			v, err = s.Get(ctx, h, time.Time{}, makeInterfaceSlice(""))
			So(v, ShouldEqual, 333)
			So(err, ShouldBeNil)
		})
	})
}

func TestIncr(t *testing.T) {
	ctx := context.Background()

	Convey("Increments from 0 to 1", t, func() {
		Convey("Int64 type", func() {
			s := NewInMemory()
			h, _ := s.Register(&fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType})
			So(s.Incr(ctx, h, time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)

			v, err := s.Get(ctx, h, time.Time{}, []interface{}{})
			So(v, ShouldEqual, 1)
			So(err, ShouldBeNil)
		})

		Convey("Float64 type", func() {
			s := NewInMemory()
			h, _ := s.Register(&fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType})
			So(s.Incr(ctx, h, time.Time{}, []interface{}{}, float64(1)), ShouldBeNil)

			v, err := s.Get(ctx, h, time.Time{}, []interface{}{})
			So(v, ShouldEqual, 1.0)
			So(err, ShouldBeNil)
		})

		Convey("String type", func() {
			s := NewInMemory()
			h, _ := s.Register(&fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType})
			So(s.Incr(ctx, h, time.Time{}, []interface{}{}, "1"), ShouldNotBeNil)
		})
	})

	Convey("Increments from 42 to 43", t, func() {
		Convey("Int64 type", func() {
			s := NewInMemory()
			h, _ := s.Register(&fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType})
			So(s.Set(ctx, h, time.Time{}, []interface{}{}, int64(42)), ShouldBeNil)
			So(s.Incr(ctx, h, time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)

			v, err := s.Get(ctx, h, time.Time{}, []interface{}{})
			So(v, ShouldEqual, int64(43))
			So(err, ShouldBeNil)
		})

		Convey("Float64 type", func() {
			s := NewInMemory()
			h, _ := s.Register(&fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType})
			So(s.Set(ctx, h, time.Time{}, []interface{}{}, float64(42)), ShouldBeNil)
			So(s.Incr(ctx, h, time.Time{}, []interface{}{}, float64(1)), ShouldBeNil)

			v, err := s.Get(ctx, h, time.Time{}, []interface{}{})
			So(v, ShouldEqual, float64(43))
			So(err, ShouldBeNil)
		})
	})
}

func TestRegisterTwice(t *testing.T) {
	Convey("Register a metric twice", t, func() {
		s := NewInMemory()
		h, err := s.Register(&fakeMetric{"foo", []field.Field{}, types.NonCumulativeIntType})
		So(err, ShouldBeNil)

		_, err = s.Register(&fakeMetric{"foo", []field.Field{}, types.NonCumulativeIntType})
		So(err, ShouldNotBeNil)

		s.Unregister(h)
		_, err = s.Register(&fakeMetric{"foo", []field.Field{}, types.NonCumulativeIntType})
		So(err, ShouldBeNil)
	})
}

type sortableCellSlice []types.Cell

func (s sortableCellSlice) Len() int { return len(s) }
func (s sortableCellSlice) Less(i, j int) bool {
	return s[i].ResetTime.UnixNano() < s[j].ResetTime.UnixNano()
}
func (s sortableCellSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }

func TestGetAll(t *testing.T) {
	Convey("GetAll", t, func() {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)

		s := NewInMemory()
		foo, err := s.Register(&fakeMetric{"foo", []field.Field{}, types.NonCumulativeIntType})
		So(err, ShouldBeNil)
		bar, err := s.Register(&fakeMetric{"bar", []field.Field{field.String("f")}, types.StringType})
		So(err, ShouldBeNil)
		baz, err := s.Register(&fakeMetric{"baz", []field.Field{field.String("f")}, types.CumulativeFloatType})
		So(err, ShouldBeNil)

		// Add test records. We increment the test clock each time so that the added
		// records sort deterministically using sortableCellSlice.
		for _, m := range []struct {
			handle    MetricHandle
			fieldvals []interface{}
			value     interface{}
		}{
			{foo, []interface{}{}, int64(42)},
			{bar, makeInterfaceSlice("one"), "hello"},
			{bar, makeInterfaceSlice("two"), "world"},
			{baz, makeInterfaceSlice("three"), 1.23},
			{baz, makeInterfaceSlice("four"), 4.56},
		} {
			So(s.Set(ctx, m.handle, time.Time{}, m.fieldvals, m.value), ShouldBeNil)
			tc.Add(time.Second)
		}

		got := s.GetAll(ctx)
		sort.Sort(sortableCellSlice(got))
		want := []types.Cell{
			{
				types.MetricInfo{
					MetricName: "foo",
					Fields:     []field.Field{},
					ValueType:  types.NonCumulativeIntType,
				},
				types.CellData{
					FieldVals: []interface{}{},
					Value:     int64(42),
				},
			},
			{
				types.MetricInfo{
					MetricName: "bar",
					Fields:     []field.Field{field.String("f")},
					ValueType:  types.StringType,
				},
				types.CellData{
					FieldVals: makeInterfaceSlice("one"),
					Value:     "hello",
				},
			},
			{
				types.MetricInfo{
					MetricName: "bar",
					Fields:     []field.Field{field.String("f")},
					ValueType:  types.StringType,
				},
				types.CellData{
					FieldVals: makeInterfaceSlice("two"),
					Value:     "world",
				},
			},
			{
				types.MetricInfo{
					MetricName: "baz",
					Fields:     []field.Field{field.String("f")},
					ValueType:  types.CumulativeFloatType,
				},
				types.CellData{
					FieldVals: makeInterfaceSlice("three"),
					Value:     1.23,
				},
			},
			{
				types.MetricInfo{
					MetricName: "baz",
					Fields:     []field.Field{field.String("f")},
					ValueType:  types.CumulativeFloatType,
				},
				types.CellData{
					FieldVals: makeInterfaceSlice("four"),
					Value:     4.56,
				},
			},
		}
		So(len(got), ShouldEqual, len(want))

		for i, g := range got {
			w := want[i]

			Convey(fmt.Sprintf("%d", i), func() {
				So(g.MetricName, ShouldEqual, w.MetricName)
				So(len(g.Fields), ShouldEqual, len(w.Fields))
				So(g.ValueType, ShouldEqual, w.ValueType)
				So(g.FieldVals, ShouldResemble, w.FieldVals)
				So(g.Value, ShouldEqual, w.Value)
			})
		}
	})
}

func TestFixedResetTime(t *testing.T) {
	ctx := context.Background()

	Convey("Get", t, func() {
		s := NewInMemory()
		h, err := s.Register(&fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType})

		t := time.Date(1234, 5, 6, 7, 8, 9, 10, time.UTC)
		v, err := s.Get(ctx, h, t, []interface{}{})
		So(v, ShouldBeNil)
		So(err, ShouldBeNil)
		So(s.Incr(ctx, h, time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)

		v, err = s.Get(ctx, h, time.Time{}, []interface{}{})
		So(v, ShouldEqual, 1)
		So(err, ShouldBeNil)

		all := s.GetAll(ctx)
		So(len(all), ShouldEqual, 1)
		So(all[0].ResetTime.String(), ShouldEqual, t.String())
	})

	Convey("Incr", t, func() {
		s := NewInMemory()
		h, err := s.Register(&fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType})

		t := time.Date(1234, 5, 6, 7, 8, 9, 10, time.UTC)
		So(s.Incr(ctx, h, t, []interface{}{}, int64(1)), ShouldBeNil)
		So(s.Incr(ctx, h, time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)

		v, err := s.Get(ctx, h, time.Time{}, []interface{}{})
		So(v, ShouldEqual, 2)
		So(err, ShouldBeNil)

		all := s.GetAll(ctx)
		So(len(all), ShouldEqual, 1)
		So(all[0].ResetTime.String(), ShouldEqual, t.String())
	})

	Convey("Set", t, func() {
		s := NewInMemory()
		h, err := s.Register(&fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType})

		t := time.Date(1234, 5, 6, 7, 8, 9, 10, time.UTC)
		So(s.Set(ctx, h, t, []interface{}{}, int64(42)), ShouldBeNil)
		So(s.Incr(ctx, h, time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)

		v, err := s.Get(ctx, h, time.Time{}, []interface{}{})
		So(v, ShouldEqual, 43)
		So(err, ShouldBeNil)

		all := s.GetAll(ctx)
		So(len(all), ShouldEqual, 1)
		So(all[0].ResetTime.String(), ShouldEqual, t.String())
	})
}

func TestConcurrency(t *testing.T) {
	const numIterations = 100
	const numGoroutines = 32

	ctx := context.Background()

	Convey("Register", t, func(c C) {
		s := NewInMemory()

		wg := sync.WaitGroup{}
		f := func(n int) {
			defer wg.Done()
			for i := 0; i < numIterations; i++ {
				name := fmt.Sprintf("%d-%d", n, i)
				_, err := s.Register(&fakeMetric{name, []field.Field{}, types.NonCumulativeIntType})
				c.So(err, ShouldBeNil)
			}
		}

		for n := 0; n < numGoroutines; n++ {
			wg.Add(1)
			go f(n)
		}
		wg.Wait()

		So(len(s.(*inMemoryStore).data), ShouldEqual, numGoroutines*numIterations)
	})

	Convey("Incr", t, func(c C) {
		s := NewInMemory()
		h, err := s.Register(&fakeMetric{"m", []field.Field{}, types.CumulativeIntType})
		So(err, ShouldBeNil)

		wg := sync.WaitGroup{}
		f := func(n int) {
			defer wg.Done()
			for i := 0; i < numIterations; i++ {
				c.So(s.Incr(ctx, h, time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)
			}
		}

		for n := 0; n < numGoroutines; n++ {
			wg.Add(1)
			go f(n)
		}
		wg.Wait()

		val, err := s.Get(ctx, h, time.Time{}, []interface{}{})
		So(val, ShouldEqual, numIterations*numGoroutines)
		So(err, ShouldBeNil)
	})
}

func TestDifferentTargets(t *testing.T) {
	ctx := context.Background()

	Convey("Gets from context", t, func() {
		s := NewInMemory()
		h, _ := s.Register(&fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType})

		t := target.Task{}
		t.AsProto().ServiceName = proto.String("foo")
		ctxWithTarget := target.Set(ctx, &t)

		So(s.Set(ctx, h, time.Time{}, []interface{}{}, int64(42)), ShouldBeNil)
		So(s.Set(ctxWithTarget, h, time.Time{}, []interface{}{}, int64(43)), ShouldBeNil)

		val, err := s.Get(ctx, h, time.Time{}, []interface{}{})
		So(err, ShouldBeNil)
		So(val, ShouldEqual, 42)

		val, err = s.Get(ctxWithTarget, h, time.Time{}, []interface{}{})
		So(err, ShouldBeNil)
		So(val, ShouldEqual, 43)

		all := s.GetAll(ctx)
		So(len(all), ShouldEqual, 2)

		// The order is undefined.
		if all[0].Value.(int64) == 42 {
			So(all[0].Target, ShouldBeNil)
			So(all[1].Target, ShouldEqual, &t)
		} else {
			So(all[0].Target, ShouldEqual, &t)
			So(all[1].Target, ShouldBeNil)
		}
	})
}
