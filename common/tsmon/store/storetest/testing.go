// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package storetest is imported exclusively by tests for Store implementations.
package storetest

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"

	. "github.com/smartystreets/goconvey/convey"
	pb "go.chromium.org/luci/common/tsmon/ts_mon_proto"
)

// Store is a store under test.
//
// It is a copy of store.Store interface to break module dependency cycle.
type Store interface {
	DefaultTarget() types.Target
	SetDefaultTarget(t types.Target)

	Get(c context.Context, m types.Metric, resetTime time.Time, fieldVals []interface{}) interface{}
	Set(c context.Context, m types.Metric, resetTime time.Time, fieldVals []interface{}, value interface{})
	Incr(c context.Context, m types.Metric, resetTime time.Time, fieldVals []interface{}, delta interface{})

	GetAll(c context.Context) []types.Cell

	Reset(c context.Context, m types.Metric)
}

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

	distOne := distribution.New(distribution.DefaultBucketer)
	distOne.Add(4.2)

	distTwo := distribution.New(distribution.DefaultBucketer)
	distTwo.Add(5.6)
	distTwo.Add(1.0)

	tests := []struct {
		typ      types.ValueType
		bucketer *distribution.Bucketer

		values           []interface{}
		wantSetValidator func(interface{}, interface{})

		deltas            []interface{}
		wantIncrPanic     bool
		wantIncrValue     interface{}
		wantIncrValidator func(interface{})

		wantStartTimestamp bool
	}{
		{
			typ:                types.CumulativeIntType,
			values:             makeInterfaceSlice(int64(3), int64(4)),
			deltas:             makeInterfaceSlice(int64(3), int64(4)),
			wantIncrValue:      int64(7),
			wantStartTimestamp: true,
		},
		{
			typ:                types.CumulativeFloatType,
			values:             makeInterfaceSlice(float64(3.2), float64(4.3)),
			deltas:             makeInterfaceSlice(float64(3.2), float64(4.3)),
			wantIncrValue:      float64(7.5),
			wantStartTimestamp: true,
		},
		{
			typ:      types.CumulativeDistributionType,
			bucketer: distribution.DefaultBucketer,
			values:   makeInterfaceSlice(distOne, distTwo),
			deltas:   makeInterfaceSlice(float64(3.2), float64(5.3)),
			wantIncrValidator: func(v interface{}) {
				d := v.(*distribution.Distribution)
				So(d.Buckets(), ShouldResemble, []int64{0, 0, 0, 1, 1})
			},
			wantStartTimestamp: true,
		},
		{
			typ:           types.NonCumulativeIntType,
			values:        makeInterfaceSlice(int64(3), int64(4)),
			deltas:        makeInterfaceSlice(int64(3), int64(4)),
			wantIncrPanic: true,
		},
		{
			typ:           types.NonCumulativeFloatType,
			values:        makeInterfaceSlice(float64(3.2), float64(4.3)),
			deltas:        makeInterfaceSlice(float64(3.2), float64(4.3)),
			wantIncrPanic: true,
		},
		{
			typ:           types.NonCumulativeDistributionType,
			bucketer:      distribution.DefaultBucketer,
			values:        makeInterfaceSlice(distOne, distTwo),
			deltas:        makeInterfaceSlice(float64(3.2), float64(5.3)),
			wantIncrPanic: true,
			wantSetValidator: func(got, want interface{}) {
				// The distribution might be serialized/deserialized and lose sums and
				// counts.
				So(got.(*distribution.Distribution).Buckets(), ShouldResemble,
					want.(*distribution.Distribution).Buckets())
			},
		},
		{
			typ:           types.StringType,
			values:        makeInterfaceSlice("hello", "world"),
			deltas:        makeInterfaceSlice("hello", "world"),
			wantIncrPanic: true,
		},
		{
			typ:           types.BoolType,
			values:        makeInterfaceSlice(true, false),
			deltas:        makeInterfaceSlice(true, false),
			wantIncrPanic: true,
		},
	}

	Convey("Set and get", t, func() {
		Convey("With no fields", func() {
			for i, test := range tests {
				Convey(fmt.Sprintf("%d. %s", i, test.typ), func() {
					var m types.Metric
					if test.bucketer != nil {
						m = &fakeDistributionMetric{FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ},
							types.MetricMetadata{}}, test.bucketer}
					} else {
						m = &FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ},
							types.MetricMetadata{},
						}
					}

					s := opts.Factory()

					// Value should be nil initially.
					v := s.Get(ctx, m, time.Time{}, []interface{}{})
					So(v, ShouldBeNil)

					// Set and get the value.
					s.Set(ctx, m, time.Time{}, []interface{}{}, test.values[0])
					v = s.Get(ctx, m, time.Time{}, []interface{}{})
					if test.wantSetValidator != nil {
						test.wantSetValidator(v, test.values[0])
					} else {
						So(v, ShouldEqual, test.values[0])
					}
				})
			}
		})

		Convey("With fields", func() {
			for i, test := range tests {
				Convey(fmt.Sprintf("%d. %s", i, test.typ), func() {
					var m types.Metric
					if test.bucketer != nil {
						m = &fakeDistributionMetric{FakeMetric{
							types.MetricInfo{"m", "", []field.Field{field.String("f")}, test.typ},
							types.MetricMetadata{}}, test.bucketer}
					} else {
						m = &FakeMetric{
							types.MetricInfo{"m", "", []field.Field{field.String("f")}, test.typ},
							types.MetricMetadata{}}
					}

					s := opts.Factory()

					// Values should be nil initially.
					v := s.Get(ctx, m, time.Time{}, makeInterfaceSlice("one"))
					So(v, ShouldBeNil)
					v = s.Get(ctx, m, time.Time{}, makeInterfaceSlice("two"))
					So(v, ShouldBeNil)

					// Set and get the values.
					s.Set(ctx, m, time.Time{}, makeInterfaceSlice("one"), test.values[0])
					s.Set(ctx, m, time.Time{}, makeInterfaceSlice("two"), test.values[1])

					v = s.Get(ctx, m, time.Time{}, makeInterfaceSlice("one"))
					if test.wantSetValidator != nil {
						test.wantSetValidator(v, test.values[0])
					} else {
						So(v, ShouldEqual, test.values[0])
					}

					v = s.Get(ctx, m, time.Time{}, makeInterfaceSlice("two"))
					if test.wantSetValidator != nil {
						test.wantSetValidator(v, test.values[1])
					} else {
						So(v, ShouldEqual, test.values[1])
					}
				})
			}
		})

		Convey("With a fixed reset time", func() {
			for i, test := range tests {
				Convey(fmt.Sprintf("%d. %s", i, test.typ), func() {
					var m types.Metric
					if test.bucketer != nil {
						m = &fakeDistributionMetric{FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ},
							types.MetricMetadata{}}, test.bucketer}
					} else {
						m = &FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ},
							types.MetricMetadata{}}
					}

					s := opts.Factory()
					opts.RegistrationFinished(s)

					// Do the set with a fixed time.
					t := time.Date(1972, 5, 6, 7, 8, 9, 0, time.UTC)
					s.Set(ctx, m, t, []interface{}{}, test.values[0])

					v := s.Get(ctx, m, time.Time{}, []interface{}{})
					if test.wantSetValidator != nil {
						test.wantSetValidator(v, test.values[0])
					} else {
						So(v, ShouldEqual, test.values[0])
					}

					// Check the time in the Cell is the same.
					all := s.GetAll(ctx)
					So(len(all), ShouldEqual, 1)

					msg := monitor.SerializeValue(all[0], testclock.TestRecentTimeUTC)
					ts := msg.GetStartTimestamp()
					if test.wantStartTimestamp {
						So(time.Unix(ts.GetSeconds(), int64(ts.GetNanos())).UTC().String(), ShouldEqual, t.String())
					} else {
						So(time.Unix(ts.GetSeconds(), int64(ts.GetNanos())).UTC().String(), ShouldEqual, testclock.TestRecentTimeUTC.String())
					}
				})
			}
		})

		Convey("With a target set in the context", func() {
			for i, test := range tests {
				Convey(fmt.Sprintf("%d. %s", i, test.typ), func() {
					var m types.Metric
					if test.bucketer != nil {
						m = &fakeDistributionMetric{FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ},
							types.MetricMetadata{}}, test.bucketer}
					} else {
						m = &FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ},
							types.MetricMetadata{}}
					}

					s := opts.Factory()
					opts.RegistrationFinished(s)

					// Create a context with a different target.
					foo := target.Task{ServiceName: "foo"}
					ctxWithTarget := target.Set(ctx, &foo)

					// Set the first value on the default target, second value on the
					// different target.
					s.Set(ctx, m, time.Time{}, []interface{}{}, test.values[0])
					s.Set(ctxWithTarget, m, time.Time{}, []interface{}{}, test.values[1])

					// Get should return different values for different contexts.
					v := s.Get(ctx, m, time.Time{}, []interface{}{})
					if test.wantSetValidator != nil {
						test.wantSetValidator(v, test.values[0])
					} else {
						So(v, ShouldEqual, test.values[0])
					}

					v = s.Get(ctxWithTarget, m, time.Time{}, []interface{}{})
					if test.wantSetValidator != nil {
						test.wantSetValidator(v, test.values[1])
					} else {
						So(v, ShouldEqual, test.values[1])
					}

					// The targets should be set in the Cells.
					all := s.GetAll(ctx)
					So(len(all), ShouldEqual, 2)

					coll := monitor.SerializeCells(all, testclock.TestRecentTimeUTC)
					s0 := coll[0].TargetSchema.(*pb.MetricsCollection_Task).Task.GetServiceName()
					s1 := coll[1].TargetSchema.(*pb.MetricsCollection_Task).Task.GetServiceName()
					switch {
					case s0 == foo.ServiceName:
						So(s1, ShouldEqual, s.DefaultTarget().(*target.Task).ServiceName)
					case s1 == foo.ServiceName:
						So(s0, ShouldEqual, s.DefaultTarget().(*target.Task).ServiceName)
					default:
						t.Fail()
					}
				})
			}
		})

		Convey("With a decreasing value", func() {
			for i, test := range tests {
				if !test.typ.IsCumulative() || test.bucketer != nil {
					continue
				}
				Convey(fmt.Sprintf("%d. %s", i, test.typ), func() {
					m := &FakeMetric{
						types.MetricInfo{"m", "", []field.Field{}, test.typ},
						types.MetricMetadata{}}
					s := opts.Factory()

					// Set the bigger value.
					s.Set(ctx, m, time.Time{}, []interface{}{}, test.values[1])
					So(s.Get(ctx, m, time.Time{}, []interface{}{}), ShouldResemble, test.values[1])

					// Setting the smaller value should be ignored.
					s.Set(ctx, m, time.Time{}, []interface{}{}, test.values[0])
					So(s.Get(ctx, m, time.Time{}, []interface{}{}), ShouldResemble, test.values[1])
				})
			}
		})
	})

	Convey("Increment and get", t, func() {
		Convey("With no fields", func() {
			for i, test := range tests {
				Convey(fmt.Sprintf("%d. %s", i, test.typ), func() {
					var m types.Metric
					if test.bucketer != nil {
						m = &fakeDistributionMetric{FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ},
							types.MetricMetadata{}}, test.bucketer}

					} else {
						m = &FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ},
							types.MetricMetadata{}}
					}

					s := opts.Factory()

					// Value should be nil initially.
					v := s.Get(ctx, m, time.Time{}, []interface{}{})
					So(v, ShouldBeNil)

					// Increment the metric.
					for _, delta := range test.deltas {
						call := func() { s.Incr(ctx, m, time.Time{}, []interface{}{}, delta) }
						if test.wantIncrPanic {
							So(call, ShouldPanic)
						} else {
							call()
						}
					}

					// Get the final value.
					v = s.Get(ctx, m, time.Time{}, []interface{}{})
					if test.wantIncrValue != nil {
						So(v, ShouldEqual, test.wantIncrValue)
					} else if test.wantIncrValidator != nil {
						test.wantIncrValidator(v)
					}
				})
			}
		})

		Convey("With fields", func() {
			for i, test := range tests {
				Convey(fmt.Sprintf("%d. %s", i, test.typ), func() {
					var m types.Metric
					if test.bucketer != nil {
						m = &fakeDistributionMetric{FakeMetric{
							types.MetricInfo{"m", "", []field.Field{field.String("f")}, test.typ},
							types.MetricMetadata{}}, test.bucketer}
					} else {
						m = &FakeMetric{
							types.MetricInfo{"m", "", []field.Field{field.String("f")}, test.typ},
							types.MetricMetadata{}}
					}

					s := opts.Factory()

					// Values should be nil initially.
					v := s.Get(ctx, m, time.Time{}, makeInterfaceSlice("one"))
					So(v, ShouldBeNil)
					v = s.Get(ctx, m, time.Time{}, makeInterfaceSlice("two"))
					So(v, ShouldBeNil)

					// Increment one cell.
					for _, delta := range test.deltas {
						call := func() { s.Incr(ctx, m, time.Time{}, makeInterfaceSlice("one"), delta) }
						if test.wantIncrPanic {
							So(call, ShouldPanic)
						} else {
							call()
						}
					}

					// Get the final value.
					v = s.Get(ctx, m, time.Time{}, makeInterfaceSlice("one"))
					if test.wantIncrValue != nil {
						So(v, ShouldEqual, test.wantIncrValue)
					} else if test.wantIncrValidator != nil {
						test.wantIncrValidator(v)
					}

					// Another cell should still be nil.
					v = s.Get(ctx, m, time.Time{}, makeInterfaceSlice("two"))
					So(v, ShouldBeNil)
				})
			}
		})

		Convey("With a fixed reset time", func() {
			for i, test := range tests {
				if test.wantIncrPanic {
					continue
				}

				Convey(fmt.Sprintf("%d. %s", i, test.typ), func() {
					var m types.Metric
					if test.bucketer != nil {
						m = &fakeDistributionMetric{FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ},
							types.MetricMetadata{}}, test.bucketer}
					} else {
						m = &FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ},
							types.MetricMetadata{}}
					}

					s := opts.Factory()
					opts.RegistrationFinished(s)

					// Do the incr with a fixed time.
					t := time.Date(1972, 5, 6, 7, 8, 9, 0, time.UTC)
					s.Incr(ctx, m, t, []interface{}{}, test.deltas[0])

					// Check the time in the Cell is the same.
					all := s.GetAll(ctx)
					So(len(all), ShouldEqual, 1)

					msg := monitor.SerializeValue(all[0], testclock.TestRecentTimeUTC)
					if test.wantStartTimestamp {
						ts := msg.GetStartTimestamp()
						So(time.Unix(ts.GetSeconds(), int64(ts.GetNanos())).UTC().String(), ShouldEqual, t.String())
					} else {
						So(msg.StartTimestamp, ShouldBeNil)
					}
				})
			}
		})

		Convey("With a target set in the context", func() {
			for i, test := range tests {
				if test.wantIncrPanic {
					continue
				}

				Convey(fmt.Sprintf("%d. %s", i, test.typ), func() {
					var m types.Metric
					if test.bucketer != nil {
						m = &fakeDistributionMetric{
							FakeMetric{types.MetricInfo{"m", "", []field.Field{}, test.typ},
								types.MetricMetadata{}},
							test.bucketer}
					} else {
						m = &FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ},
							types.MetricMetadata{}}
					}

					s := opts.Factory()
					opts.RegistrationFinished(s)

					// Create a context with a different target.
					foo := target.Task{ServiceName: "foo"}
					ctxWithTarget := target.Set(ctx, &foo)

					// Incr the first delta on the default target, second delta on the
					// different target.
					s.Incr(ctx, m, time.Time{}, []interface{}{}, test.deltas[0])
					s.Incr(ctxWithTarget, m, time.Time{}, []interface{}{}, test.deltas[1])

					// Get should return different values for different contexts.
					v1 := s.Get(ctx, m, time.Time{}, []interface{}{})
					v2 := s.Get(ctxWithTarget, m, time.Time{}, []interface{}{})
					So(v1, ShouldNotEqual, v2)

					// The targets should be set in the Cells.
					all := s.GetAll(ctx)
					So(len(all), ShouldEqual, 2)

					coll := monitor.SerializeCells(all, testclock.TestRecentTimeUTC)
					s0 := coll[0].TargetSchema.(*pb.MetricsCollection_Task).Task.GetServiceName()
					s1 := coll[1].TargetSchema.(*pb.MetricsCollection_Task).Task.GetServiceName()
					switch {
					case s0 == foo.ServiceName:
						So(s1, ShouldEqual, s.DefaultTarget().(*target.Task).ServiceName)
					case s1 == foo.ServiceName:
						So(s0, ShouldEqual, s.DefaultTarget().(*target.Task).ServiceName)
					default:
						t.Fail()
					}
				})
			}
		})
	})

	Convey("GetAll", t, func() {
		ctx, tc := testclock.UseTime(context.Background(), testclock.TestRecentTimeUTC)

		s := opts.Factory()
		foo := &FakeMetric{
			types.MetricInfo{"foo", "", []field.Field{}, types.NonCumulativeIntType},
			types.MetricMetadata{}}
		bar := &FakeMetric{
			types.MetricInfo{"bar", "", []field.Field{field.String("f")}, types.StringType},
			types.MetricMetadata{}}
		baz := &FakeMetric{
			types.MetricInfo{"baz", "", []field.Field{field.String("f")}, types.NonCumulativeFloatType},
			types.MetricMetadata{}}
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
			s.Set(ctx, m.metric, time.Time{}, m.fieldvals, m.value)
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
				types.MetricMetadata{},
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
				types.MetricMetadata{},
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
				types.MetricMetadata{},
				types.CellData{
					FieldVals: makeInterfaceSlice("two"),
					Value:     "world",
				},
			},
			{
				types.MetricInfo{
					Name:      "baz",
					Fields:    []field.Field{field.String("f")},
					ValueType: types.NonCumulativeFloatType,
				},
				types.MetricMetadata{},
				types.CellData{
					FieldVals: makeInterfaceSlice("three"),
					Value:     1.23,
				},
			},
			{
				types.MetricInfo{
					Name:      "baz",
					Fields:    []field.Field{field.String("f")},
					ValueType: types.NonCumulativeFloatType,
				},
				types.MetricMetadata{},
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

	Convey("Concurrency", t, func() {
		const numIterations = 100
		const numGoroutines = 32

		Convey("Incr", func(c C) {
			s := opts.Factory()
			m := &FakeMetric{
				types.MetricInfo{"m", "", []field.Field{}, types.CumulativeIntType},
				types.MetricMetadata{}}

			wg := sync.WaitGroup{}
			f := func(n int) {
				defer wg.Done()
				for i := 0; i < numIterations; i++ {
					s.Incr(ctx, m, time.Time{}, []interface{}{}, int64(1))
				}
			}

			for n := 0; n < numGoroutines; n++ {
				wg.Add(1)
				go f(n)
			}
			wg.Wait()

			val := s.Get(ctx, m, time.Time{}, []interface{}{})
			So(val, ShouldEqual, numIterations*numGoroutines)
		})
	})

	Convey("Different targets", t, func() {
		Convey("Gets from context", func() {
			s := opts.Factory()
			m := &FakeMetric{
				types.MetricInfo{"m", "", []field.Field{}, types.NonCumulativeIntType},
				types.MetricMetadata{}}
			opts.RegistrationFinished(s)

			t := target.Task{ServiceName: "foo"}
			ctxWithTarget := target.Set(ctx, &t)

			s.Set(ctx, m, time.Time{}, []interface{}{}, int64(42))
			s.Set(ctxWithTarget, m, time.Time{}, []interface{}{}, int64(43))

			val := s.Get(ctx, m, time.Time{}, []interface{}{})
			So(val, ShouldEqual, 42)

			val = s.Get(ctxWithTarget, m, time.Time{}, []interface{}{})
			So(val, ShouldEqual, 43)

			all := s.GetAll(ctx)
			So(len(all), ShouldEqual, 2)

			// The order is undefined.
			if all[0].Value.(int64) == 42 {
				So(all[0].Target, ShouldResemble, s.DefaultTarget())
				So(all[1].Target, ShouldResemble, &t)
			} else {
				So(all[0].Target, ShouldResemble, &t)
				So(all[1].Target, ShouldResemble, s.DefaultTarget())
			}
		})
	})
}

func makeInterfaceSlice(v ...interface{}) []interface{} {
	return v
}

// FakeMetric is a fake Metric implementation.
type FakeMetric struct {
	types.MetricInfo
	types.MetricMetadata
}

// Info implements Metric.Info
func (m *FakeMetric) Info() types.MetricInfo { return m.MetricInfo }

// Metadata implements Metric.Metadata
func (m *FakeMetric) Metadata() types.MetricMetadata { return m.MetricMetadata }

// SetFixedResetTime implements Metric.SetFixedResetTime.
func (m *FakeMetric) SetFixedResetTime(t time.Time) {}

type fakeDistributionMetric struct {
	FakeMetric

	bucketer *distribution.Bucketer
}

func (m *fakeDistributionMetric) Bucketer() *distribution.Bucketer { return m.bucketer }

type sortableCellSlice []types.Cell

func (s sortableCellSlice) Len() int { return len(s) }
func (s sortableCellSlice) Less(i, j int) bool {
	return s[i].ResetTime.UnixNano() < s[j].ResetTime.UnixNano()
}
func (s sortableCellSlice) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
