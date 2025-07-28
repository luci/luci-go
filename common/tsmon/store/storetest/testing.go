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

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/target"
	pb "go.chromium.org/luci/common/tsmon/ts_mon_proto"
	"go.chromium.org/luci/common/tsmon/types"
)

// Store is a store under test.
//
// It is a copy of store.Store interface to break module dependency cycle.
type Store interface {
	DefaultTarget() types.Target
	SetDefaultTarget(t types.Target)

	Get(ctx context.Context, m types.Metric, fieldVals []any) any
	Set(ctx context.Context, m types.Metric, fieldVals []any, value any)
	Del(ctx context.Context, m types.Metric, fieldVals []any)
	Incr(ctx context.Context, m types.Metric, fieldVals []any, delta any)

	GetAll(ctx context.Context) []types.Cell

	Reset(ctx context.Context, m types.Metric)
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

		values           []any
		wantSetValidator func(any, any)

		deltas            []any
		wantIncrPanic     bool
		wantIncrValue     any
		wantIncrValidator func(any)

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
			wantIncrValidator: func(v any) {
				d := v.(*distribution.Distribution)
				assert.Loosely(t, d.Buckets(), should.Resemble([]int64{0, 0, 0, 1, 1}))
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
			wantSetValidator: func(got, want any) {
				// The distribution might be serialized/deserialized and lose sums and
				// counts.
				assert.Loosely(t, got.(*distribution.Distribution).Buckets(), should.Resemble(
					want.(*distribution.Distribution).Buckets()))
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

	ftt.Run("Set and get", t, func(t *ftt.Test) {
		t.Run("With no fields", func(t *ftt.Test) {
			for i, test := range tests {
				t.Run(fmt.Sprintf("%d. %s", i, test.typ), func(t *ftt.Test) {
					var m types.Metric
					if test.bucketer != nil {
						m = &fakeDistributionMetric{FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ, target.NilType},
							types.MetricMetadata{}}, test.bucketer}
					} else {
						m = &FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ, target.NilType},
							types.MetricMetadata{},
						}
					}

					s := opts.Factory()

					// Value should be nil initially.
					v := s.Get(ctx, m, []any{})
					assert.Loosely(t, v, should.BeNil)

					// Set and get the value.
					s.Set(ctx, m, []any{}, test.values[0])
					v = s.Get(ctx, m, []any{})
					if test.wantSetValidator != nil {
						test.wantSetValidator(v, test.values[0])
					} else {
						assert.Loosely(t, v, should.Equal(test.values[0]))
					}
				})
			}
		})

		t.Run("With fields", func(t *ftt.Test) {
			for i, test := range tests {
				t.Run(fmt.Sprintf("%d. %s", i, test.typ), func(t *ftt.Test) {
					var m types.Metric
					if test.bucketer != nil {
						m = &fakeDistributionMetric{FakeMetric{
							types.MetricInfo{"m", "", []field.Field{field.String("f")}, test.typ, target.NilType},
							types.MetricMetadata{}}, test.bucketer}
					} else {
						m = &FakeMetric{
							types.MetricInfo{"m", "", []field.Field{field.String("f")}, test.typ, target.NilType},
							types.MetricMetadata{}}
					}

					s := opts.Factory()

					// Values should be nil initially.
					v := s.Get(ctx, m, makeInterfaceSlice("one"))
					assert.Loosely(t, v, should.BeNil)
					v = s.Get(ctx, m, makeInterfaceSlice("two"))
					assert.Loosely(t, v, should.BeNil)

					// Set and get the values.
					s.Set(ctx, m, makeInterfaceSlice("one"), test.values[0])
					s.Set(ctx, m, makeInterfaceSlice("two"), test.values[1])

					v = s.Get(ctx, m, makeInterfaceSlice("one"))
					if test.wantSetValidator != nil {
						test.wantSetValidator(v, test.values[0])
					} else {
						assert.Loosely(t, v, should.Equal(test.values[0]))
					}

					v = s.Get(ctx, m, makeInterfaceSlice("two"))
					if test.wantSetValidator != nil {
						test.wantSetValidator(v, test.values[1])
					} else {
						assert.Loosely(t, v, should.Equal(test.values[1]))
					}
				})
			}
		})

		t.Run("With a target set in the context", func(t *ftt.Test) {
			for i, test := range tests {
				t.Run(fmt.Sprintf("%d. %s", i, test.typ), func(t *ftt.Test) {
					var m types.Metric
					if test.bucketer != nil {
						m = &fakeDistributionMetric{FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ, target.NilType},
							types.MetricMetadata{}}, test.bucketer}
					} else {
						m = &FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ, target.NilType},
							types.MetricMetadata{}}
					}

					s := opts.Factory()
					opts.RegistrationFinished(s)

					// Create a context with a different target.
					foo := target.Task{ServiceName: "foo"}
					ctxWithTarget := target.Set(ctx, &foo)

					// Set the first value on the default target, second value on the
					// different target.
					s.Set(ctx, m, []any{}, test.values[0])
					s.Set(ctxWithTarget, m, []any{}, test.values[1])

					// Get should return different values for different contexts.
					v := s.Get(ctx, m, []any{})
					if test.wantSetValidator != nil {
						test.wantSetValidator(v, test.values[0])
					} else {
						assert.Loosely(t, v, should.Equal(test.values[0]))
					}

					v = s.Get(ctxWithTarget, m, []any{})
					if test.wantSetValidator != nil {
						test.wantSetValidator(v, test.values[1])
					} else {
						assert.Loosely(t, v, should.Equal(test.values[1]))
					}

					// The targets should be set in the Cells.
					all := s.GetAll(ctx)
					assert.Loosely(t, len(all), should.Equal(2))

					coll := monitor.SerializeCells(all, testclock.TestRecentTimeUTC)

					s0 := coll[0].RootLabels[3]
					s1 := coll[1].RootLabels[3]

					serviceName := &pb.MetricsCollection_RootLabels{
						Key: proto.String("service_name"),
						Value: &pb.MetricsCollection_RootLabels_StringValue{
							StringValue: foo.ServiceName,
						},
					}
					defaultServiceName := &pb.MetricsCollection_RootLabels{
						Key: proto.String("service_name"),
						Value: &pb.MetricsCollection_RootLabels_StringValue{
							StringValue: s.DefaultTarget().(*target.Task).ServiceName,
						},
					}

					if proto.Equal(s0, serviceName) {
						assert.Loosely(t, s1, should.Resemble(defaultServiceName))
					} else if proto.Equal(s1, serviceName) {
						assert.Loosely(t, s0, should.Resemble(defaultServiceName))
					} else {
						t.Fail()
					}
				})
			}
		})

		t.Run("With a decreasing value", func(t *ftt.Test) {
			for i, test := range tests {
				if !test.typ.IsCumulative() || test.bucketer != nil {
					continue
				}
				t.Run(fmt.Sprintf("%d. %s", i, test.typ), func(t *ftt.Test) {
					m := &FakeMetric{
						types.MetricInfo{"m", "", []field.Field{}, test.typ, target.NilType},
						types.MetricMetadata{}}
					s := opts.Factory()

					// Set the bigger value.
					s.Set(ctx, m, []any{}, test.values[1])
					assert.Loosely(t, s.Get(ctx, m, []any{}), should.Resemble(test.values[1]))

					// Setting the smaller value should be ignored.
					s.Set(ctx, m, []any{}, test.values[0])
					assert.Loosely(t, s.Get(ctx, m, []any{}), should.Resemble(test.values[1]))
				})
			}
		})
	})

	ftt.Run("Increment and get", t, func(t *ftt.Test) {
		t.Run("With no fields", func(t *ftt.Test) {
			for i, test := range tests {
				t.Run(fmt.Sprintf("%d. %s", i, test.typ), func(t *ftt.Test) {
					var m types.Metric
					if test.bucketer != nil {
						m = &fakeDistributionMetric{FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ, target.NilType},
							types.MetricMetadata{}}, test.bucketer}
					} else {
						m = &FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ, target.NilType},
							types.MetricMetadata{}}
					}

					s := opts.Factory()

					// Value should be nil initially.
					v := s.Get(ctx, m, []any{})
					assert.Loosely(t, v, should.BeNil)

					// Increment the metric.
					for _, delta := range test.deltas {
						call := func() { s.Incr(ctx, m, []any{}, delta) }
						if test.wantIncrPanic {
							assert.Loosely(t, call, should.Panic)
						} else {
							call()
						}
					}

					// Get the final value.
					v = s.Get(ctx, m, []any{})
					if test.wantIncrValue != nil {
						assert.Loosely(t, v, should.Equal(test.wantIncrValue))
					} else if test.wantIncrValidator != nil {
						test.wantIncrValidator(v)
					}
				})
			}
		})

		t.Run("With fields", func(t *ftt.Test) {
			for i, test := range tests {
				t.Run(fmt.Sprintf("%d. %s", i, test.typ), func(t *ftt.Test) {
					var m types.Metric
					if test.bucketer != nil {
						m = &fakeDistributionMetric{FakeMetric{
							types.MetricInfo{"m", "", []field.Field{field.String("f")}, test.typ, target.NilType},
							types.MetricMetadata{}}, test.bucketer}
					} else {
						m = &FakeMetric{
							types.MetricInfo{"m", "", []field.Field{field.String("f")}, test.typ, target.NilType},
							types.MetricMetadata{}}
					}

					s := opts.Factory()

					// Values should be nil initially.
					v := s.Get(ctx, m, makeInterfaceSlice("one"))
					assert.Loosely(t, v, should.BeNil)
					v = s.Get(ctx, m, makeInterfaceSlice("two"))
					assert.Loosely(t, v, should.BeNil)

					// Increment one cell.
					for _, delta := range test.deltas {
						call := func() { s.Incr(ctx, m, makeInterfaceSlice("one"), delta) }
						if test.wantIncrPanic {
							assert.Loosely(t, call, should.Panic)
						} else {
							call()
						}
					}

					// Get the final value.
					v = s.Get(ctx, m, makeInterfaceSlice("one"))
					if test.wantIncrValue != nil {
						assert.Loosely(t, v, should.Equal(test.wantIncrValue))
					} else if test.wantIncrValidator != nil {
						test.wantIncrValidator(v)
					}

					// Another cell should still be nil.
					v = s.Get(ctx, m, makeInterfaceSlice("two"))
					assert.Loosely(t, v, should.BeNil)
				})
			}
		})

		t.Run("With a target set in the context", func(t *ftt.Test) {
			for i, test := range tests {
				if test.wantIncrPanic {
					continue
				}

				t.Run(fmt.Sprintf("%d. %s", i, test.typ), func(t *ftt.Test) {
					var m types.Metric
					if test.bucketer != nil {
						m = &fakeDistributionMetric{
							FakeMetric{types.MetricInfo{"m", "", []field.Field{}, test.typ, target.NilType},
								types.MetricMetadata{}},
							test.bucketer}
					} else {
						m = &FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ, target.NilType},
							types.MetricMetadata{}}
					}

					s := opts.Factory()
					opts.RegistrationFinished(s)

					// Create a context with a different target.
					foo := target.Task{ServiceName: "foo"}
					ctxWithTarget := target.Set(ctx, &foo)

					// Incr the first delta on the default target, second delta on the
					// different target.
					s.Incr(ctx, m, []any{}, test.deltas[0])
					s.Incr(ctxWithTarget, m, []any{}, test.deltas[1])

					// Get should return different values for different contexts.
					v1 := s.Get(ctx, m, []any{})
					v2 := s.Get(ctxWithTarget, m, []any{})
					assert.Loosely(t, v1, should.NotEqual(v2))

					// The targets should be set in the Cells.
					all := s.GetAll(ctx)
					assert.Loosely(t, len(all), should.Equal(2))

					coll := monitor.SerializeCells(all, testclock.TestRecentTimeUTC)

					s0 := coll[0].RootLabels[3]
					s1 := coll[1].RootLabels[3]

					serviceName := &pb.MetricsCollection_RootLabels{
						Key: proto.String("service_name"),
						Value: &pb.MetricsCollection_RootLabels_StringValue{
							StringValue: foo.ServiceName,
						},
					}
					defaultServiceName := &pb.MetricsCollection_RootLabels{
						Key: proto.String("service_name"),
						Value: &pb.MetricsCollection_RootLabels_StringValue{
							StringValue: s.DefaultTarget().(*target.Task).ServiceName,
						},
					}

					if proto.Equal(s0, serviceName) {
						assert.Loosely(t, s1, should.Resemble(defaultServiceName))
					} else if proto.Equal(s1, serviceName) {
						assert.Loosely(t, s0, should.Resemble(defaultServiceName))
					} else {
						t.Fail()
					}
				})
			}
		})
	})

	ftt.Run("GetAll", t, func(t *ftt.Test) {
		ctx, tc := testclock.UseTime(ctx, testclock.TestRecentTimeUTC)

		s := opts.Factory()
		foo := &FakeMetric{
			types.MetricInfo{Name: "foo", Description: "", Fields: []field.Field{},
				ValueType: types.NonCumulativeIntType, TargetType: target.NilType},
			types.MetricMetadata{Units: types.Seconds}}
		bar := &FakeMetric{
			types.MetricInfo{Name: "bar", Description: "", Fields: []field.Field{field.String("f")},
				ValueType: types.StringType, TargetType: target.NilType},
			types.MetricMetadata{Units: types.Bytes}}
		baz := &FakeMetric{
			types.MetricInfo{Name: "baz", Description: "", Fields: []field.Field{field.String("f")},
				ValueType: types.NonCumulativeFloatType, TargetType: target.NilType},
			types.MetricMetadata{}}
		qux := &FakeMetric{
			types.MetricInfo{Name: "qux", Description: "", Fields: []field.Field{field.String("f")},
				ValueType: types.CumulativeDistributionType, TargetType: target.NilType},
			types.MetricMetadata{}}
		opts.RegistrationFinished(s)

		// Add test records. We increment the test clock each time so that the added
		// records sort deterministically using sortableCellSlice.
		for _, m := range []struct {
			metric    types.Metric
			fieldvals []any
			value     any
		}{
			{foo, []any{}, int64(42)},
			{bar, makeInterfaceSlice("one"), "hello"},
			{bar, makeInterfaceSlice("two"), "world"},
			{baz, makeInterfaceSlice("three"), 1.23},
			{baz, makeInterfaceSlice("four"), 4.56},
			{qux, makeInterfaceSlice("five"), distribution.New(nil)},
		} {
			s.Set(ctx, m.metric, m.fieldvals, m.value)
			tc.Add(time.Second)
		}

		got := s.GetAll(ctx)

		// Store operations made after GetAll should not be visible in the snapshot.
		s.Set(ctx, baz, makeInterfaceSlice("four"), 3.14)
		s.Incr(ctx, qux, makeInterfaceSlice("five"), float64(10.0))

		sort.Sort(sortableCellSlice(got))
		want := []types.Cell{
			{
				types.MetricInfo{
					Name:       "foo",
					Fields:     []field.Field{},
					ValueType:  types.NonCumulativeIntType,
					TargetType: target.NilType,
				},
				types.MetricMetadata{Units: types.Seconds},
				types.CellData{
					FieldVals: []any{},
					Value:     int64(42),
				},
			},
			{
				types.MetricInfo{
					Name:       "bar",
					Fields:     []field.Field{field.String("f")},
					ValueType:  types.StringType,
					TargetType: target.NilType,
				},
				types.MetricMetadata{Units: types.Bytes},
				types.CellData{
					FieldVals: makeInterfaceSlice("one"),
					Value:     "hello",
				},
			},
			{
				types.MetricInfo{
					Name:       "bar",
					Fields:     []field.Field{field.String("f")},
					ValueType:  types.StringType,
					TargetType: target.NilType,
				},
				types.MetricMetadata{Units: types.Bytes},
				types.CellData{
					FieldVals: makeInterfaceSlice("two"),
					Value:     "world",
				},
			},
			{
				types.MetricInfo{
					Name:       "baz",
					Fields:     []field.Field{field.String("f")},
					ValueType:  types.NonCumulativeFloatType,
					TargetType: target.NilType,
				},
				types.MetricMetadata{},
				types.CellData{
					FieldVals: makeInterfaceSlice("three"),
					Value:     1.23,
				},
			},
			{
				types.MetricInfo{
					Name:       "baz",
					Fields:     []field.Field{field.String("f")},
					ValueType:  types.NonCumulativeFloatType,
					TargetType: target.NilType,
				},
				types.MetricMetadata{},
				types.CellData{
					FieldVals: makeInterfaceSlice("four"),
					Value:     4.56,
				},
			},
			{
				types.MetricInfo{
					Name:       "qux",
					Fields:     []field.Field{field.String("f")},
					ValueType:  types.CumulativeDistributionType,
					TargetType: target.NilType,
				},
				types.MetricMetadata{},
				types.CellData{
					FieldVals: makeInterfaceSlice("five"),
					Value:     distribution.New(nil),
				},
			},
		}
		assert.Loosely(t, len(got), should.Equal(len(want)))

		for i, g := range got {
			w := want[i]

			t.Run(fmt.Sprintf("%d", i), func(t *ftt.Test) {
				assert.Loosely(t, g.Name, should.Equal(w.Name))
				assert.Loosely(t, len(g.Fields), should.Equal(len(w.Fields)))
				assert.Loosely(t, g.ValueType, should.Equal(w.ValueType))
				assert.Loosely(t, g.FieldVals, should.Resemble(w.FieldVals))
				assert.Loosely(t, g.Value, should.Resemble(w.Value))
				assert.Loosely(t, g.Units, should.Equal(w.Units))
			})
		}
	})

	ftt.Run("Set and del", t, func(t *ftt.Test) {
		t.Run("With no fields", func(t *ftt.Test) {
			for i, test := range tests {
				t.Run(fmt.Sprintf("%d. %s", i, test.typ), func(t *ftt.Test) {
					var m types.Metric
					if test.bucketer != nil {
						m = &fakeDistributionMetric{FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ, target.NilType},
							types.MetricMetadata{}}, test.bucketer}
					} else {
						m = &FakeMetric{
							types.MetricInfo{"m", "", []field.Field{}, test.typ, target.NilType},
							types.MetricMetadata{}}
					}

					s := opts.Factory()

					// Value should be nil initially.
					assert.Loosely(t, s.Get(ctx, m, []any{}), should.BeNil)

					// Set and get the value.
					s.Set(ctx, m, []any{}, test.values[0])
					v := s.Get(ctx, m, []any{})
					if test.wantSetValidator != nil {
						test.wantSetValidator(v, test.values[0])
					} else {
						assert.Loosely(t, v, should.Equal(test.values[0]))
					}

					// Delete the cell. Then, get should return nil.
					s.Del(ctx, m, []any{})
					assert.Loosely(t, s.Get(ctx, m, []any{}), should.BeNil)
				})
			}
		})

		t.Run("With fields", func(t *ftt.Test) {
			for i, test := range tests {
				t.Run(fmt.Sprintf("%d. %s", i, test.typ), func(t *ftt.Test) {
					var m types.Metric
					if test.bucketer != nil {
						m = &fakeDistributionMetric{FakeMetric{
							types.MetricInfo{"m", "", []field.Field{field.String("f")}, test.typ, target.NilType},
							types.MetricMetadata{}}, test.bucketer}
					} else {
						m = &FakeMetric{
							types.MetricInfo{"m", "", []field.Field{field.String("f")}, test.typ, target.NilType},
							types.MetricMetadata{}}
					}

					s := opts.Factory()

					// Values should be nil initially.
					assert.Loosely(t, s.Get(ctx, m, makeInterfaceSlice("one")), should.BeNil)
					assert.Loosely(t, s.Get(ctx, m, makeInterfaceSlice("two")), should.BeNil)

					// Set both and then delete "one".
					s.Set(ctx, m, makeInterfaceSlice("one"), test.values[0])
					s.Set(ctx, m, makeInterfaceSlice("two"), test.values[1])
					s.Del(ctx, m, makeInterfaceSlice("one"))

					// Get should return nil for "one", but the value for "two".
					assert.Loosely(t, s.Get(ctx, m, makeInterfaceSlice("one")), should.BeNil)
					v := s.Get(ctx, m, makeInterfaceSlice("two"))
					if test.wantSetValidator != nil {
						test.wantSetValidator(v, test.values[1])
					} else {
						assert.Loosely(t, v, should.Equal(test.values[1]))
					}
				})
			}
		})

		t.Run("With a target set in the context", func(t *ftt.Test) {
			for i, test := range tests {
				if test.wantIncrPanic {
					continue
				}

				t.Run(fmt.Sprintf("%d. %s", i, test.typ), func(t *ftt.Test) {
					var m types.Metric
					if test.bucketer != nil {
						m = &fakeDistributionMetric{FakeMetric{
							types.MetricInfo{"m", "", []field.Field{field.String("f")}, test.typ, target.NilType},
							types.MetricMetadata{}}, test.bucketer}
					} else {
						m = &FakeMetric{
							types.MetricInfo{"m", "", []field.Field{field.String("f")}, test.typ, target.NilType},
							types.MetricMetadata{}}
					}

					s := opts.Factory()
					opts.RegistrationFinished(s)

					// Create a context with a different target.
					foo := target.Task{ServiceName: "foo"}
					ctxWithTarget := target.Set(ctx, &foo)

					// Set the first value on the default target, second value on the
					// different target. Note that both are set with the same field value,
					// "one".
					fvs := func() []any { return makeInterfaceSlice("one") }
					s.Set(ctx, m, fvs(), test.values[0])
					s.Set(ctxWithTarget, m, fvs(), test.values[1])

					// Get should return different values for different contexts.
					v1 := s.Get(ctx, m, fvs())
					v2 := s.Get(ctxWithTarget, m, fvs())
					assert.Loosely(t, v1, should.NotEqual(v2))

					// Delete the cell with the custom target. Then, get should return
					// the value for the default target, and nil for the custom target.
					s.Del(ctxWithTarget, m, fvs())
					assert.Loosely(t, s.Get(ctx, m, fvs()), should.Equal(test.values[0]))
					assert.Loosely(t, s.Get(ctxWithTarget, m, fvs()), should.BeNil)
				})
			}
		})
	})

	ftt.Run("Concurrency", t, func(t *ftt.Test) {
		const numIterations = 100
		const numGoroutines = 32

		t.Run("Incr", func(c *ftt.Test) {
			s := opts.Factory()
			m := &FakeMetric{
				types.MetricInfo{"m", "", []field.Field{}, types.CumulativeIntType, target.NilType},
				types.MetricMetadata{}}

			wg := sync.WaitGroup{}
			f := func(n int) {
				defer wg.Done()
				for range numIterations {
					s.Incr(ctx, m, []any{}, int64(1))
				}
			}

			for n := range numGoroutines {
				wg.Add(1)
				go f(n)
			}
			wg.Wait()

			val := s.Get(ctx, m, []any{})
			assert.Loosely(c, val, should.Equal(numIterations*numGoroutines))
		})
	})

	ftt.Run("Multiple targets with the same TargetType", t, func(c *ftt.Test) {
		c.Run("Gets from context", func(c *ftt.Test) {
			s := opts.Factory()
			m := &FakeMetric{
				types.MetricInfo{"m", "", []field.Field{}, types.NonCumulativeIntType, target.NilType},
				types.MetricMetadata{}}
			opts.RegistrationFinished(s)

			t := target.Task{ServiceName: "foo"}
			ctxWithTarget := target.Set(ctx, &t)

			s.Set(ctx, m, []any{}, int64(42))
			s.Set(ctxWithTarget, m, []any{}, int64(43))

			val := s.Get(ctx, m, []any{})
			assert.Loosely(c, val, should.Equal(42))

			val = s.Get(ctxWithTarget, m, []any{})
			assert.Loosely(c, val, should.Equal(43))

			all := s.GetAll(ctx)
			assert.Loosely(c, len(all), should.Equal(2))

			// The order is undefined.
			if all[0].Value.(int64) == 42 {
				assert.Loosely(c, all[0].Target, should.Resemble(s.DefaultTarget()))
				assert.Loosely(c, all[1].Target, should.Resemble(&t))
			} else {
				assert.Loosely(c, all[0].Target, should.Resemble(&t))
				assert.Loosely(c, all[1].Target, should.Resemble(s.DefaultTarget()))
			}
		})
	})

	ftt.Run("Multiple targets with multiple TargetTypes", t, func(c *ftt.Test) {
		c.Run("Gets from context", func(c *ftt.Test) {
			s := opts.Factory()
			// Two metrics with the same metric name, but different types.
			mTask := &FakeMetric{
				types.MetricInfo{"m", "", []field.Field{}, types.NonCumulativeIntType, target.TaskType},
				types.MetricMetadata{}}
			mDevice := &FakeMetric{
				types.MetricInfo{"m", "", []field.Field{}, types.NonCumulativeIntType, target.DeviceType},
				types.MetricMetadata{}}
			opts.RegistrationFinished(s)

			taskTarget := target.Task{ServiceName: "foo"}
			deviceTarget := target.NetworkDevice{Hostname: "bar"}
			ctxWithTarget := target.Set(target.Set(ctx, &taskTarget), &deviceTarget)

			s.Set(ctxWithTarget, mTask, []any{}, int64(42))
			s.Set(ctxWithTarget, mDevice, []any{}, int64(43))

			val := s.Get(ctxWithTarget, mTask, []any{})
			assert.Loosely(c, val, should.Equal(42))

			val = s.Get(ctxWithTarget, mDevice, []any{})
			assert.Loosely(c, val, should.Equal(43))

			all := s.GetAll(ctx)
			assert.Loosely(c, len(all), should.Equal(2))

			// The order is undefined.
			if all[0].Value.(int64) == 42 {
				assert.Loosely(c, all[0].Target, should.Resemble(&taskTarget))
				assert.Loosely(c, all[1].Target, should.Resemble(&deviceTarget))
			} else {
				assert.Loosely(c, all[0].Target, should.Resemble(&deviceTarget))
				assert.Loosely(c, all[1].Target, should.Resemble(&taskTarget))
			}
		})
	})
}

func makeInterfaceSlice(v ...any) []any {
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
