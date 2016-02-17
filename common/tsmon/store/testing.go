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
	"github.com/luci/luci-go/common/tsmon/distribution"
	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/target"
	"github.com/luci/luci-go/common/tsmon/types"
	"golang.org/x/net/context"

	. "github.com/smartystreets/goconvey/convey"
)

// TestOptions contains options for RunStoreImplementationTests.
type TestOptions struct {
	// Factory creates and returns a new Store implementation.
	Factory func() Store

	// RegistrationFinished is called after all metrics have been registered.
	// Store implementations that need to do expensive initialization (that would
	// otherwise be done in iface.go) can do that here.
	RegistrationFinished func(Store)

	// GetNumRegisteredMetrics returns the number of metrics registered in the
	// store.
	GetNumRegisteredMetrics func(Store) int
}

// RunStoreImplementationTests runs all the standard tests that all store
// implementations are expected to pass.  When you write a new Store
// implementation you should ensure you run these tests against it.
func RunStoreImplementationTests(t *testing.T, ctx context.Context, opts TestOptions) {
	Convey("Register, set and get", t, func() {
		Convey("Registered metric with no fields", func() {
			s := opts.Factory()

			m := &fakeMetric{"foo", []field.Field{}, types.NonCumulativeIntType}
			s.Register(m)

			Convey("Initial Get should return nil", func() {
				v, err := s.Get(ctx, m, time.Time{}, []interface{}{})
				So(v, ShouldBeNil)
				So(err, ShouldBeNil)
			})

			Convey("Set and Get", func() {
				err := s.Set(ctx, m, time.Time{}, []interface{}{}, int64(42))
				So(err, ShouldBeNil)

				v, err := s.Get(ctx, m, time.Time{}, []interface{}{})
				So(v, ShouldEqual, int64(42))
				So(err, ShouldBeNil)
			})
		})

		Convey("Registered metric with a field", func() {
			s := opts.Factory()

			m := &fakeMetric{"foo", []field.Field{field.String("f")}, types.NonCumulativeIntType}
			s.Register(m)

			Convey("Initial Get should return nil", func() {
				v, err := s.Get(ctx, m, time.Time{}, makeInterfaceSlice("one"))
				So(v, ShouldBeNil)
				So(err, ShouldBeNil)
			})

			Convey("Set and Get", func() {
				So(s.Set(ctx, m, time.Time{}, makeInterfaceSlice("one"), int64(111)), ShouldBeNil)
				So(s.Set(ctx, m, time.Time{}, makeInterfaceSlice("two"), int64(222)), ShouldBeNil)
				So(s.Set(ctx, m, time.Time{}, makeInterfaceSlice(""), int64(333)), ShouldBeNil)

				v, err := s.Get(ctx, m, time.Time{}, makeInterfaceSlice("one"))
				So(v, ShouldEqual, int64(111))
				So(err, ShouldBeNil)

				v, err = s.Get(ctx, m, time.Time{}, makeInterfaceSlice("two"))
				So(v, ShouldEqual, int64(222))
				So(err, ShouldBeNil)

				v, err = s.Get(ctx, m, time.Time{}, makeInterfaceSlice(""))
				So(v, ShouldEqual, int64(333))
				So(err, ShouldBeNil)
			})
		})
	})

	Convey("Increment", t, func() {
		Convey("Increments from 0 to 1", func() {
			Convey("Int64 type", func() {
				s := opts.Factory()
				m := &fakeMetric{"m", []field.Field{}, types.CumulativeIntType}
				s.Register(m)

				So(s.Incr(ctx, m, time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)

				v, err := s.Get(ctx, m, time.Time{}, []interface{}{})
				So(v, ShouldEqual, 1)
				So(err, ShouldBeNil)
			})

			Convey("Float64 type", func() {
				s := opts.Factory()
				m := &fakeMetric{"m", []field.Field{}, types.CumulativeFloatType}
				s.Register(m)
				So(s.Incr(ctx, m, time.Time{}, []interface{}{}, float64(1)), ShouldBeNil)

				v, err := s.Get(ctx, m, time.Time{}, []interface{}{})
				So(v, ShouldEqual, 1.0)
				So(err, ShouldBeNil)
			})

			Convey("String type", func() {
				s := opts.Factory()
				m := &fakeMetric{"m", []field.Field{}, types.StringType}
				s.Register(m)
				So(s.Incr(ctx, m, time.Time{}, []interface{}{}, "1"), ShouldNotBeNil)
			})

			Convey("Bool type", func() {
				s := opts.Factory()
				m := &fakeMetric{"m", []field.Field{}, types.BoolType}
				s.Register(m)
				So(s.Incr(ctx, m, time.Time{}, []interface{}{}, "1"), ShouldNotBeNil)
			})

			Convey("Non-cumulative int type", func() {
				s := opts.Factory()
				m := &fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType}
				s.Register(m)
				So(s.Incr(ctx, m, time.Time{}, []interface{}{}, int64(1)), ShouldNotBeNil)
			})

			Convey("Non-cumulative float type", func() {
				s := opts.Factory()
				m := &fakeMetric{"m", []field.Field{}, types.NonCumulativeFloatType}
				s.Register(m)
				So(s.Incr(ctx, m, time.Time{}, []interface{}{}, float64(1)), ShouldNotBeNil)
			})
		})

		Convey("Increments from 42 to 43", func() {
			Convey("Int64 type", func() {
				s := opts.Factory()
				m := &fakeMetric{"m", []field.Field{}, types.CumulativeIntType}
				s.Register(m)
				So(s.Set(ctx, m, time.Time{}, []interface{}{}, int64(42)), ShouldBeNil)
				So(s.Incr(ctx, m, time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)

				v, err := s.Get(ctx, m, time.Time{}, []interface{}{})
				So(v, ShouldEqual, int64(43))
				So(err, ShouldBeNil)
			})

			Convey("Float64 type", func() {
				s := opts.Factory()
				m := &fakeMetric{"m", []field.Field{}, types.CumulativeFloatType}
				s.Register(m)
				So(s.Set(ctx, m, time.Time{}, []interface{}{}, float64(42)), ShouldBeNil)
				So(s.Incr(ctx, m, time.Time{}, []interface{}{}, float64(1)), ShouldBeNil)

				v, err := s.Get(ctx, m, time.Time{}, []interface{}{})
				So(v, ShouldEqual, float64(43))
				So(err, ShouldBeNil)
			})
		})
	})

	Convey("GetAll", t, func() {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)

		s := opts.Factory()
		foo := &fakeMetric{"foo", []field.Field{}, types.NonCumulativeIntType}
		bar := &fakeMetric{"bar", []field.Field{field.String("f")}, types.StringType}
		baz := &fakeMetric{"baz", []field.Field{field.String("f")}, types.CumulativeFloatType}
		s.Register(foo)
		s.Register(bar)
		s.Register(baz)
		opts.RegistrationFinished(s)

		// Add test records. We increment the test clock each time so that the added
		// records sort deterministically using sortableCellSlice.
		for _, m := range []struct {
			metric    types.Metric
			fieldvals []interface{}
			value     interface{}
		}{
			{foo, []interface{}{}, int64(42)},
			{bar, makeInterfaceSlice("one"), "hello"},
			{bar, makeInterfaceSlice("two"), "world"},
			{baz, makeInterfaceSlice("three"), 1.23},
			{baz, makeInterfaceSlice("four"), 4.56},
		} {
			So(s.Set(ctx, m.metric, time.Time{}, m.fieldvals, m.value), ShouldBeNil)
			tc.Add(time.Second)
		}

		got := s.GetAll(ctx)
		sort.Sort(sortableCellSlice(got))
		want := []types.Cell{
			{
				types.MetricInfo{
					Name:      "foo",
					Fields:    []field.Field{},
					ValueType: types.NonCumulativeIntType,
				},
				types.CellData{
					FieldVals: []interface{}{},
					Value:     int64(42),
				},
			},
			{
				types.MetricInfo{
					Name:      "bar",
					Fields:    []field.Field{field.String("f")},
					ValueType: types.StringType,
				},
				types.CellData{
					FieldVals: makeInterfaceSlice("one"),
					Value:     "hello",
				},
			},
			{
				types.MetricInfo{
					Name:      "bar",
					Fields:    []field.Field{field.String("f")},
					ValueType: types.StringType,
				},
				types.CellData{
					FieldVals: makeInterfaceSlice("two"),
					Value:     "world",
				},
			},
			{
				types.MetricInfo{
					Name:      "baz",
					Fields:    []field.Field{field.String("f")},
					ValueType: types.CumulativeFloatType,
				},
				types.CellData{
					FieldVals: makeInterfaceSlice("three"),
					Value:     1.23,
				},
			},
			{
				types.MetricInfo{
					Name:      "baz",
					Fields:    []field.Field{field.String("f")},
					ValueType: types.CumulativeFloatType,
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
				So(g.Name, ShouldEqual, w.Name)
				So(len(g.Fields), ShouldEqual, len(w.Fields))
				So(g.ValueType, ShouldEqual, w.ValueType)
				So(g.FieldVals, ShouldResemble, w.FieldVals)
				So(g.Value, ShouldEqual, w.Value)
			})
		}
	})

	Convey("Fixed reset time", t, func() {
		Convey("Incr", func() {
			s := opts.Factory()
			m := &fakeMetric{"m", []field.Field{}, types.CumulativeIntType}
			s.Register(m)
			opts.RegistrationFinished(s)

			t := time.Date(1234, 5, 6, 7, 8, 9, 10, time.UTC)
			So(s.Incr(ctx, m, t, []interface{}{}, int64(1)), ShouldBeNil)
			So(s.Incr(ctx, m, time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)

			v, err := s.Get(ctx, m, time.Time{}, []interface{}{})
			So(v, ShouldEqual, 2)
			So(err, ShouldBeNil)

			all := s.GetAll(ctx)
			So(len(all), ShouldEqual, 1)
			So(all[0].ResetTime.String(), ShouldEqual, t.String())
		})

		Convey("Set", func() {
			s := opts.Factory()
			m := &fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType}
			s.Register(m)
			opts.RegistrationFinished(s)

			t := time.Date(1234, 5, 6, 7, 8, 9, 10, time.UTC)
			So(s.Set(ctx, m, t, []interface{}{}, int64(42)), ShouldBeNil)
			So(s.Set(ctx, m, time.Time{}, []interface{}{}, int64(43)), ShouldBeNil)

			v, err := s.Get(ctx, m, time.Time{}, []interface{}{})
			So(v, ShouldEqual, 43)
			So(err, ShouldBeNil)

			all := s.GetAll(ctx)
			So(len(all), ShouldEqual, 1)
			So(all[0].ResetTime.String(), ShouldEqual, t.String())
		})
	})

	Convey("Concurrency", t, func() {
		const numIterations = 100
		const numGoroutines = 32

		Convey("Incr", func(c C) {
			s := opts.Factory()
			m := &fakeMetric{"m", []field.Field{}, types.CumulativeIntType}
			s.Register(m)

			wg := sync.WaitGroup{}
			f := func(n int) {
				defer wg.Done()
				for i := 0; i < numIterations; i++ {
					c.So(s.Incr(ctx, m, time.Time{}, []interface{}{}, int64(1)), ShouldBeNil)
				}
			}

			for n := 0; n < numGoroutines; n++ {
				wg.Add(1)
				go f(n)
			}
			wg.Wait()

			val, err := s.Get(ctx, m, time.Time{}, []interface{}{})
			So(val, ShouldEqual, numIterations*numGoroutines)
			So(err, ShouldBeNil)
		})
	})

	Convey("Different targets", t, func() {
		Convey("Gets from context", func() {
			s := opts.Factory()
			m := &fakeMetric{"m", []field.Field{}, types.NonCumulativeIntType}
			s.Register(m)
			opts.RegistrationFinished(s)

			t := target.Task{}
			t.AsProto().ServiceName = proto.String("foo")
			ctxWithTarget := target.Set(ctx, &t)

			So(s.Set(ctx, m, time.Time{}, []interface{}{}, int64(42)), ShouldBeNil)
			So(s.Set(ctxWithTarget, m, time.Time{}, []interface{}{}, int64(43)), ShouldBeNil)

			val, err := s.Get(ctx, m, time.Time{}, []interface{}{})
			So(err, ShouldBeNil)
			So(val, ShouldEqual, 42)

			val, err = s.Get(ctxWithTarget, m, time.Time{}, []interface{}{})
			So(err, ShouldBeNil)
			So(val, ShouldEqual, 43)

			all := s.GetAll(ctx)
			So(len(all), ShouldEqual, 2)

			// The order is undefined.
			if all[0].Value.(int64) == 42 {
				So(all[0].Target, ShouldEqual, s.DefaultTarget())
				So(all[1].Target, ShouldEqual, &t)
			} else {
				So(all[0].Target, ShouldEqual, &t)
				So(all[1].Target, ShouldEqual, s.DefaultTarget())
			}
		})
	})
}

func makeInterfaceSlice(v ...interface{}) []interface{} {
	return v
}

type fakeMetric types.MetricInfo

func (m *fakeMetric) Info() types.MetricInfo        { return types.MetricInfo(*m) }
func (m *fakeMetric) SetFixedResetTime(t time.Time) {}

type fakeDistributionMetric struct {
	fakeMetric

	bucketer *distribution.Bucketer
}

func (m *fakeDistributionMetric) Bucketer() *distribution.Bucketer { return m.bucketer }

type sortableCellSlice []types.Cell

func (s sortableCellSlice) Len() int { return len(s) }
func (s sortableCellSlice) Less(i, j int) bool {
	return s[i].ResetTime.UnixNano() < s[j].ResetTime.UnixNano()
}
func (s sortableCellSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
