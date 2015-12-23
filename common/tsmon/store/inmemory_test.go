// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
package store

import (
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/tsmon/field"
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

	Convey("Unregistered metric", t, func() {
		s := InMemoryStore{}
		Convey("Get", func() {
			_, err := s.Get(ctx, "foo", time.Time{}, []interface{}{})
			So(err, ShouldNotBeNil)
		})
		Convey("Set", func() {
			err := s.Set(ctx, "foo", time.Time{}, []interface{}{}, nil)
			So(err, ShouldNotBeNil)
		})
	})

	Convey("Registered metric with no fields", t, func() {
		s := InMemoryStore{}
		s.Register(&fakeMetric{"foo", []field.Field{}, types.NonCumulativeIntType})

		Convey("Initial Get should return nil", func() {
			v, err := s.Get(ctx, "foo", time.Time{}, []interface{}{})
			So(v, ShouldBeNil)
			So(err, ShouldBeNil)
		})

		Convey("Set and Get", func() {
			err := s.Set(ctx, "foo", time.Time{}, []interface{}{}, "value")
			So(err, ShouldBeNil)

			v, err := s.Get(ctx, "foo", time.Time{}, []interface{}{})
			So(v, ShouldEqual, "value")
			So(err, ShouldBeNil)
		})
	})

	Convey("Registered metric with a field", t, func() {
		s := InMemoryStore{}
		s.Register(&fakeMetric{"foo", []field.Field{field.String("f")}, types.NonCumulativeIntType})

		Convey("Initial Get should return nil", func() {
			v, err := s.Get(ctx, "foo", time.Time{}, makeInterfaceSlice("one"))
			So(v, ShouldBeNil)
			So(err, ShouldBeNil)
		})

		Convey("Set and Get", func() {
			So(s.Set(ctx, "foo", time.Time{}, makeInterfaceSlice("one"), 111), ShouldBeNil)
			So(s.Set(ctx, "foo", time.Time{}, makeInterfaceSlice("two"), 222), ShouldBeNil)
			So(s.Set(ctx, "foo", time.Time{}, makeInterfaceSlice(""), 333), ShouldBeNil)

			v, err := s.Get(ctx, "foo", time.Time{}, makeInterfaceSlice("one"))
			So(v, ShouldEqual, 111)
			So(err, ShouldBeNil)

			v, err = s.Get(ctx, "foo", time.Time{}, makeInterfaceSlice("two"))
			So(v, ShouldEqual, 222)
			So(err, ShouldBeNil)

			v, err = s.Get(ctx, "foo", time.Time{}, makeInterfaceSlice(""))
			So(v, ShouldEqual, 333)
			So(err, ShouldBeNil)
		})
	})
}

func TestIncr(t *testing.T) {
	ctx := context.Background()

	Convey("Unregistered metric", t, func() {
		s := InMemoryStore{}
		So(s.Incr(ctx, "foo", time.Time{}, []interface{}{}, 1), ShouldNotBeNil)
	})

	Convey("Increments from 0 to 1", t, func() {
		Convey("Int64 type", func() {
			s := InMemoryStore{}
			s.Register(&fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType})
			So(s.Incr(ctx, "m", time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)

			v, err := s.Get(ctx, "m", time.Time{}, []interface{}{})
			So(v, ShouldEqual, 1)
			So(err, ShouldBeNil)
		})

		Convey("Float64 type", func() {
			s := InMemoryStore{}
			s.Register(&fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType})
			So(s.Incr(ctx, "m", time.Time{}, []interface{}{}, float64(1)), ShouldBeNil)

			v, err := s.Get(ctx, "m", time.Time{}, []interface{}{})
			So(v, ShouldEqual, 1.0)
			So(err, ShouldBeNil)
		})

		Convey("String type", func() {
			s := InMemoryStore{}
			s.Register(&fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType})
			So(s.Incr(ctx, "m", time.Time{}, []interface{}{}, "1"), ShouldNotBeNil)
		})
	})

	Convey("Increments from 42 to 43", t, func() {
		Convey("Int64 type", func() {
			s := InMemoryStore{}
			s.Register(&fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType})
			So(s.Set(ctx, "m", time.Time{}, []interface{}{}, int64(42)), ShouldBeNil)
			So(s.Incr(ctx, "m", time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)

			v, err := s.Get(ctx, "m", time.Time{}, []interface{}{})
			So(v, ShouldEqual, int64(43))
			So(err, ShouldBeNil)
		})

		Convey("Float64 type", func() {
			s := InMemoryStore{}
			s.Register(&fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType})
			So(s.Set(ctx, "m", time.Time{}, []interface{}{}, float64(42)), ShouldBeNil)
			So(s.Incr(ctx, "m", time.Time{}, []interface{}{}, float64(1)), ShouldBeNil)

			v, err := s.Get(ctx, "m", time.Time{}, []interface{}{})
			So(v, ShouldEqual, float64(43))
			So(err, ShouldBeNil)
		})
	})
}

func TestRegisterTwice(t *testing.T) {
	Convey("Register a metric twice", t, func() {
		s := InMemoryStore{}
		So(s.Register(&fakeMetric{"foo", []field.Field{}, types.NonCumulativeIntType}), ShouldBeNil)
		So(s.Register(&fakeMetric{"foo", []field.Field{}, types.NonCumulativeIntType}), ShouldNotBeNil)
		s.Unregister("foo")
		So(s.Register(&fakeMetric{"foo", []field.Field{}, types.NonCumulativeIntType}), ShouldBeNil)
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

		s := InMemoryStore{}
		So(s.Register(&fakeMetric{"foo", []field.Field{}, types.NonCumulativeIntType}), ShouldBeNil)
		So(s.Register(&fakeMetric{"bar", []field.Field{field.String("f")}, types.StringType}), ShouldBeNil)
		So(s.Register(&fakeMetric{"baz", []field.Field{field.String("f")}, types.CumulativeFloatType}), ShouldBeNil)

		// Add test records. We increment the test clock each time so that the added
		// records sort deterministically using sortableCellSlice.
		for _, m := range []struct {
			name      string
			fieldvals []interface{}
			value     interface{}
		}{
			{"foo", []interface{}{}, int64(42)},
			{"bar", makeInterfaceSlice("one"), "hello"},
			{"bar", makeInterfaceSlice("two"), "world"},
			{"baz", makeInterfaceSlice("three"), 1.23},
			{"baz", makeInterfaceSlice("four"), 4.56},
		} {
			So(s.Set(ctx, m.name, time.Time{}, m.fieldvals, m.value), ShouldBeNil)
			tc.Add(time.Second)
		}

		got := s.GetAll(ctx)
		sort.Sort(sortableCellSlice(got))
		want := []types.Cell{
			{
				MetricName: "foo",
				Fields:     []field.Field{},
				ValueType:  types.NonCumulativeIntType,
				FieldVals:  []interface{}{},
				Value:      int64(42),
			},
			{
				MetricName: "bar",
				Fields:     []field.Field{field.String("f")},
				ValueType:  types.StringType,
				FieldVals:  makeInterfaceSlice("one"),
				Value:      "hello",
			},
			{
				MetricName: "bar",
				Fields:     []field.Field{field.String("f")},
				ValueType:  types.StringType,
				FieldVals:  makeInterfaceSlice("two"),
				Value:      "world",
			},
			{
				MetricName: "baz",
				Fields:     []field.Field{field.String("f")},
				ValueType:  types.CumulativeFloatType,
				FieldVals:  makeInterfaceSlice("three"),
				Value:      1.23,
			},
			{
				MetricName: "baz",
				Fields:     []field.Field{field.String("f")},
				ValueType:  types.CumulativeFloatType,
				FieldVals:  makeInterfaceSlice("four"),
				Value:      4.56,
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
		s := InMemoryStore{}
		s.Register(&fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType})

		t := time.Date(1234, 5, 6, 7, 8, 9, 10, time.UTC)
		v, err := s.Get(ctx, "m", t, []interface{}{})
		So(v, ShouldBeNil)
		So(err, ShouldBeNil)
		So(s.Incr(ctx, "m", time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)

		v, err = s.Get(ctx, "m", time.Time{}, []interface{}{})
		So(v, ShouldEqual, 1)
		So(err, ShouldBeNil)

		all := s.GetAll(ctx)
		So(len(all), ShouldEqual, 1)
		So(all[0].ResetTime.String(), ShouldEqual, t.String())
	})

	Convey("Incr", t, func() {
		s := InMemoryStore{}
		s.Register(&fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType})

		t := time.Date(1234, 5, 6, 7, 8, 9, 10, time.UTC)
		So(s.Incr(ctx, "m", t, []interface{}{}, int64(1)), ShouldBeNil)
		So(s.Incr(ctx, "m", time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)

		v, err := s.Get(ctx, "m", time.Time{}, []interface{}{})
		So(v, ShouldEqual, 2)
		So(err, ShouldBeNil)

		all := s.GetAll(ctx)
		So(len(all), ShouldEqual, 1)
		So(all[0].ResetTime.String(), ShouldEqual, t.String())
	})

	Convey("Set", t, func() {
		s := InMemoryStore{}
		s.Register(&fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType})

		t := time.Date(1234, 5, 6, 7, 8, 9, 10, time.UTC)
		So(s.Set(ctx, "m", t, []interface{}{}, int64(42)), ShouldBeNil)
		So(s.Incr(ctx, "m", time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)

		v, err := s.Get(ctx, "m", time.Time{}, []interface{}{})
		So(v, ShouldEqual, 43)
		So(err, ShouldBeNil)

		all := s.GetAll(ctx)
		So(len(all), ShouldEqual, 1)
		So(all[0].ResetTime.String(), ShouldEqual, t.String())
	})
}
