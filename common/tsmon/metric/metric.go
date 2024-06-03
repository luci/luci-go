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

// Package metric is the API for defining metrics and updating their values.
//
// When you define a metric you must also define the names and types of any
// fields on that metric. Additionally, you can specify the target type of
// the metric. It is an error to define two metrics with the same and
// target type. It will cause a panic if there are duplicated metrics.
//
// When setting metric values, you must pass correct number of fields, with
// correct types, or the setter function will panic.
//
// Example:
//
//	var (
//	  requests = metric.NewCounterWithTargetType("myapp/requests",
//	    "Number of requests",
//	    nil,
//	    field.String("status"),
//	    types.TaskType),
//	)
//	...
//	func handleRequest() {
//	  if success {
//	    requests.Add(1, "success")
//	  } else {
//	    requests.Add(1, "failure")
//	  }
//	}
package metric

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/registry"
	"go.chromium.org/luci/common/tsmon/target"
	"go.chromium.org/luci/common/tsmon/types"
)

// Int is a non-cumulative integer gauge metric.
type Int interface {
	types.Metric

	Get(ctx context.Context, fieldVals ...any) int64
	Set(ctx context.Context, v int64, fieldVals ...any)
}

// Counter is a cumulative integer metric.
type Counter interface {
	Int

	Add(ctx context.Context, n int64, fieldVals ...any)
}

// Float is a non-cumulative floating-point gauge metric.
type Float interface {
	types.Metric

	Get(ctx context.Context, fieldVals ...any) float64
	Set(ctx context.Context, v float64, fieldVals ...any)
}

// FloatCounter is a cumulative floating-point metric.
type FloatCounter interface {
	Float

	Add(ctx context.Context, n float64, fieldVals ...any)
}

// String is a string-valued metric.
type String interface {
	types.Metric

	Get(ctx context.Context, fieldVals ...any) string
	Set(ctx context.Context, v string, fieldVals ...any)
}

// Bool is a boolean-valued metric.
type Bool interface {
	types.Metric

	Get(ctx context.Context, fieldVals ...any) bool
	Set(ctx context.Context, v bool, fieldVals ...any)
}

// NonCumulativeDistribution is a non-cumulative-distribution-valued metric.
type NonCumulativeDistribution interface {
	types.DistributionMetric

	Get(ctx context.Context, fieldVals ...any) *distribution.Distribution
	Set(ctx context.Context, d *distribution.Distribution, fieldVals ...any)
}

// CumulativeDistribution is a cumulative-distribution-valued metric.
type CumulativeDistribution interface {
	NonCumulativeDistribution

	Add(ctx context.Context, v float64, fieldVals ...any)
}

// NewInt returns a new non-cumulative integer gauge metric.
func NewInt(name string, description string, metadata *types.MetricMetadata, fields ...field.Field) Int {
	return NewIntWithTargetType(name, target.NilType, description, metadata, fields...)
}

// NewIntWithTargetType returns a new non-cumulative integer gauge metric with a given TargetType.
func NewIntWithTargetType(name string, targetType types.TargetType, description string, metadata *types.MetricMetadata, fields ...field.Field) Int {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	m := &intMetric{metric{
		MetricInfo: types.MetricInfo{
			Name:        name,
			Description: description,
			Fields:      fields,
			ValueType:   types.NonCumulativeIntType,
			TargetType:  targetType,
		},
		MetricMetadata: *metadata,
	}}
	registry.Global.Add(m)
	return m
}

// NewCounter returns a new cumulative integer metric.
func NewCounter(name string, description string, metadata *types.MetricMetadata, fields ...field.Field) Counter {
	return NewCounterWithTargetType(name, target.NilType, description, metadata, fields...)
}

// NewCounterWithTargetType returns a new cumulative integer metric with a given TargetType.
func NewCounterWithTargetType(name string, targetType types.TargetType, description string, metadata *types.MetricMetadata, fields ...field.Field) Counter {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	m := &counter{intMetric{metric{
		MetricInfo: types.MetricInfo{
			Name:        name,
			Description: description,
			Fields:      fields,
			ValueType:   types.CumulativeIntType,
			TargetType:  targetType,
		},
		MetricMetadata: *metadata,
	}}}
	registry.Global.Add(m)
	return m
}

// NewFloat returns a new non-cumulative floating-point gauge metric.
func NewFloat(name string, description string, metadata *types.MetricMetadata, fields ...field.Field) Float {
	return NewFloatWithTargetType(name, target.NilType, description, metadata, fields...)
}

// NewFloatWithTargetType returns a new non-cumulative floating-point gauge metric with a given TargetType.
func NewFloatWithTargetType(name string, targetType types.TargetType, description string, metadata *types.MetricMetadata, fields ...field.Field) Float {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	m := &floatMetric{metric{
		MetricInfo: types.MetricInfo{
			Name:        name,
			Description: description,
			Fields:      fields,
			ValueType:   types.NonCumulativeFloatType,
			TargetType:  targetType,
		},
		MetricMetadata: *metadata,
	}}
	registry.Global.Add(m)
	return m
}

// NewFloatCounter returns a new cumulative floating-point metric.
func NewFloatCounter(name string, description string, metadata *types.MetricMetadata, fields ...field.Field) FloatCounter {
	return NewFloatCounterWithTargetType(name, target.NilType, description, metadata, fields...)
}

// NewFloatCounterWithTargetType returns a new cumulative floating-point metric with a given TargetType.
func NewFloatCounterWithTargetType(name string, targetType types.TargetType, description string, metadata *types.MetricMetadata, fields ...field.Field) FloatCounter {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	m := &floatCounter{floatMetric{metric{
		MetricInfo: types.MetricInfo{
			Name:        name,
			Description: description,
			Fields:      fields,
			ValueType:   types.CumulativeFloatType,
			TargetType:  targetType,
		},
		MetricMetadata: *metadata,
	}}}
	registry.Global.Add(m)
	return m
}

// NewString returns a new string-valued metric.
func NewString(name string, description string, metadata *types.MetricMetadata, fields ...field.Field) String {
	return NewStringWithTargetType(name, target.NilType, description, metadata, fields...)
}

// NewStringWithTargetType returns a new string-valued metric with a given TargetType
//
// Panics if metadata is set with a timestamp unit.
func NewStringWithTargetType(name string, targetType types.TargetType, description string, metadata *types.MetricMetadata, fields ...field.Field) String {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	if metadata.Units.IsTime() {
		panic(fmt.Errorf(
			"timeunit %q cannot be given to string-valued metric %q",
			metadata.Units, name))
	}
	m := &stringMetric{metric{
		MetricInfo: types.MetricInfo{
			Name:        name,
			Description: description,
			Fields:      fields,
			ValueType:   types.StringType,
			TargetType:  targetType,
		},
		MetricMetadata: *metadata,
	}}
	registry.Global.Add(m)
	return m
}

// NewBool returns a new bool-valued metric.
func NewBool(name string, description string, metadata *types.MetricMetadata, fields ...field.Field) Bool {
	return NewBoolWithTargetType(name, target.NilType, description, metadata, fields...)
}

// NewBoolWithTargetType returns a new bool-valued metric with a given TargetType.
//
// Panics if metadata is set with a timestamp unit.
func NewBoolWithTargetType(name string, targetType types.TargetType, description string, metadata *types.MetricMetadata, fields ...field.Field) Bool {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	if metadata.Units.IsTime() {
		panic(fmt.Errorf(
			"timeunit %q cannot be given to bool-valued metric %q",
			metadata.Units, name))
	}
	m := &boolMetric{metric{
		MetricInfo: types.MetricInfo{
			Name:        name,
			Description: description,
			Fields:      fields,
			ValueType:   types.BoolType,
			TargetType:  targetType,
		},
		MetricMetadata: *metadata,
	}}
	registry.Global.Add(m)
	return m
}

// NewCumulativeDistribution returns a new cumulative-distribution-valued
// metric.
func NewCumulativeDistribution(name string, description string, metadata *types.MetricMetadata, bucketer *distribution.Bucketer, fields ...field.Field) CumulativeDistribution {
	return NewCumulativeDistributionWithTargetType(name, target.NilType, description, metadata, bucketer, fields...)
}

// NewCumulativeDistributionWithTargetType returns a new cumulative-distribution-valued
// metric with a given TargetType
func NewCumulativeDistributionWithTargetType(name string, targetType types.TargetType, description string, metadata *types.MetricMetadata, bucketer *distribution.Bucketer, fields ...field.Field) CumulativeDistribution {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	m := &cumulativeDistributionMetric{
		nonCumulativeDistributionMetric{
			metric: metric{
				MetricInfo: types.MetricInfo{
					Name:        name,
					Description: description,
					Fields:      fields,
					ValueType:   types.CumulativeDistributionType,
					TargetType:  targetType,
				},
				MetricMetadata: *metadata,
			},
			bucketer: bucketer,
		},
	}
	registry.Global.Add(m)
	return m
}

// NewNonCumulativeDistribution returns a new non-cumulative-distribution-valued
// metric.
func NewNonCumulativeDistribution(name string, description string, metadata *types.MetricMetadata, bucketer *distribution.Bucketer, fields ...field.Field) NonCumulativeDistribution {
	return NewNonCumulativeDistributionWithTargetType(name, target.NilType, description, metadata, bucketer, fields...)
}

// NewNonCumulativeDistributionWithTargetType returns a new non-cumulative-distribution-valued
// metric with a given TargetType.
func NewNonCumulativeDistributionWithTargetType(name string, targetType types.TargetType, description string, metadata *types.MetricMetadata, bucketer *distribution.Bucketer, fields ...field.Field) NonCumulativeDistribution {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	m := &nonCumulativeDistributionMetric{
		metric: metric{
			MetricInfo: types.MetricInfo{
				Name:        name,
				Description: description,
				Fields:      fields,
				ValueType:   types.NonCumulativeDistributionType,
				TargetType:  targetType,
			},
			MetricMetadata: *metadata,
		},
		bucketer: bucketer,
	}
	registry.Global.Add(m)
	return m
}

// genericGet is a convenience function that tries to get a metric value from
// the store and returns the zero value if it didn't exist.
func (m *metric) genericGet(zero any, ctx context.Context, fieldVals []any) any {
	if ret := tsmon.Store(ctx).Get(ctx, m, m.fixedResetTime, fieldVals); ret != nil {
		return ret
	}
	return zero
}

type metric struct {
	types.MetricInfo
	types.MetricMetadata
	fixedResetTime time.Time
}

func (m *metric) Info() types.MetricInfo         { return m.MetricInfo }
func (m *metric) Metadata() types.MetricMetadata { return m.MetricMetadata }
func (m *metric) SetFixedResetTime(t time.Time)  { m.fixedResetTime = t }

type intMetric struct{ metric }

func (m *intMetric) Get(ctx context.Context, fieldVals ...any) int64 {
	return m.genericGet(int64(0), ctx, fieldVals).(int64)
}

func (m *intMetric) Set(ctx context.Context, v int64, fieldVals ...any) {
	tsmon.Store(ctx).Set(ctx, m, m.fixedResetTime, fieldVals, v)
}

type counter struct{ intMetric }

func (m *counter) Add(ctx context.Context, n int64, fieldVals ...any) {
	tsmon.Store(ctx).Incr(ctx, m, m.fixedResetTime, fieldVals, n)
}

type floatMetric struct{ metric }

func (m *floatMetric) Get(ctx context.Context, fieldVals ...any) float64 {
	return m.genericGet(float64(0.0), ctx, fieldVals).(float64)
}

func (m *floatMetric) Set(ctx context.Context, v float64, fieldVals ...any) {
	tsmon.Store(ctx).Set(ctx, m, m.fixedResetTime, fieldVals, v)
}

type floatCounter struct{ floatMetric }

func (m *floatCounter) Add(ctx context.Context, n float64, fieldVals ...any) {
	tsmon.Store(ctx).Incr(ctx, m, m.fixedResetTime, fieldVals, n)
}

type stringMetric struct{ metric }

func (m *stringMetric) Get(ctx context.Context, fieldVals ...any) string {
	return m.genericGet("", ctx, fieldVals).(string)
}

func (m *stringMetric) Set(ctx context.Context, v string, fieldVals ...any) {
	tsmon.Store(ctx).Set(ctx, m, m.fixedResetTime, fieldVals, v)
}

type boolMetric struct{ metric }

func (m *boolMetric) Get(ctx context.Context, fieldVals ...any) bool {
	return m.genericGet(false, ctx, fieldVals).(bool)
}

func (m *boolMetric) Set(ctx context.Context, v bool, fieldVals ...any) {
	tsmon.Store(ctx).Set(ctx, m, m.fixedResetTime, fieldVals, v)
}

type nonCumulativeDistributionMetric struct {
	metric
	bucketer *distribution.Bucketer
}

func (m *nonCumulativeDistributionMetric) Bucketer() *distribution.Bucketer { return m.bucketer }

func (m *nonCumulativeDistributionMetric) Get(ctx context.Context, fieldVals ...any) *distribution.Distribution {
	ret := m.genericGet((*distribution.Distribution)(nil), ctx, fieldVals)
	return ret.(*distribution.Distribution)
}

func (m *nonCumulativeDistributionMetric) Set(ctx context.Context, v *distribution.Distribution, fieldVals ...any) {
	tsmon.Store(ctx).Set(ctx, m, m.fixedResetTime, fieldVals, v)
}

type cumulativeDistributionMetric struct {
	nonCumulativeDistributionMetric
}

func (m *cumulativeDistributionMetric) Add(ctx context.Context, v float64, fieldVals ...any) {
	tsmon.Store(ctx).Incr(ctx, m, m.fixedResetTime, fieldVals, v)
}
