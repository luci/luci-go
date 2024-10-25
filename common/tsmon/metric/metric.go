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
//	  requests = metric.NewCounterWithOptions("myapp/requests",
//	    &metric.Options{TargetType: types.TaskType},
//	    "Number of requests",
//	    nil,
//	    field.String("status"),
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

// Options holds options to create a new metric.
type Options struct {
	TargetType types.TargetType
	Registry   *registry.Registry
}

// applyDefaultOptions returns opt with default values for unspecifed options.
func applyDefaultOptions(opt *Options) *Options {
	defaultOpt := &Options{
		TargetType: target.NilType,
		Registry:   registry.Global,
	}

	if opt == nil {
		return defaultOpt
	}

	if opt.Registry == nil {
		opt.Registry = defaultOpt.Registry
	}
	return opt
}

// NewInt returns a new non-cumulative integer gauge metric.
func NewInt(name string, description string, metadata *types.MetricMetadata, fields ...field.Field) Int {
	return NewIntWithOptions(name, nil, description, metadata, fields...)
}

// NewIntWithOptions creates a new non-cumulative integer gauge metric with given Options.
func NewIntWithOptions(name string, opt *Options, description string, metadata *types.MetricMetadata, fields ...field.Field) Int {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	opt = applyDefaultOptions(opt)

	m := &intMetric{metric{
		MetricInfo: types.MetricInfo{
			Name:        name,
			Description: description,
			Fields:      fields,
			ValueType:   types.NonCumulativeIntType,
			TargetType:  opt.TargetType,
		},
		MetricMetadata: *metadata,
	}}
	opt.Registry.Add(m)
	return m
}

// NewCounter returns a new cumulative integer metric.
func NewCounter(name string, description string, metadata *types.MetricMetadata, fields ...field.Field) Counter {
	return NewCounterWithOptions(name, nil, description, metadata, fields...)
}

// NewCounterWithOptions creates a new cumulative integer metric with given Options.
func NewCounterWithOptions(name string, opt *Options, description string, metadata *types.MetricMetadata, fields ...field.Field) Counter {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	opt = applyDefaultOptions(opt)
	m := &counter{intMetric{metric{
		MetricInfo: types.MetricInfo{
			Name:        name,
			Description: description,
			Fields:      fields,
			ValueType:   types.CumulativeIntType,
			TargetType:  opt.TargetType,
		},
		MetricMetadata: *metadata,
	}}}
	opt.Registry.Add(m)
	return m
}

// NewFloat returns a new non-cumulative floating-point gauge metric.
func NewFloat(name string, description string, metadata *types.MetricMetadata, fields ...field.Field) Float {
	return NewFloatWithOptions(name, nil, description, metadata, fields...)
}

// NewFloatWithOptions creates a new non-cumulative floating-point gauge metric with given Options.
func NewFloatWithOptions(name string, opt *Options, description string, metadata *types.MetricMetadata, fields ...field.Field) Float {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	opt = applyDefaultOptions(opt)
	m := &floatMetric{metric{
		MetricInfo: types.MetricInfo{
			Name:        name,
			Description: description,
			Fields:      fields,
			ValueType:   types.NonCumulativeFloatType,
			TargetType:  opt.TargetType,
		},
		MetricMetadata: *metadata,
	}}
	opt.Registry.Add(m)
	return m
}

// NewFloatCounter returns a new cumulative floating-point metric.
func NewFloatCounter(name string, description string, metadata *types.MetricMetadata, fields ...field.Field) FloatCounter {
	return NewFloatCounterWithOptions(name, nil, description, metadata, fields...)
}

// NewFloatCounterWithOptions creates a new cumulative floating-point metric with given Options.
func NewFloatCounterWithOptions(name string, opt *Options, description string, metadata *types.MetricMetadata, fields ...field.Field) FloatCounter {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	opt = applyDefaultOptions(opt)
	m := &floatCounter{floatMetric{metric{
		MetricInfo: types.MetricInfo{
			Name:        name,
			Description: description,
			Fields:      fields,
			ValueType:   types.CumulativeFloatType,
			TargetType:  opt.TargetType,
		},
		MetricMetadata: *metadata,
	}}}
	opt.Registry.Add(m)
	return m
}

// NewString returns a new string-valued metric.
func NewString(name string, description string, metadata *types.MetricMetadata, fields ...field.Field) String {
	return NewStringWithOptions(name, nil, description, metadata, fields...)
}

// NewStringWithOptions creates a new string-valued metric with given Options
//
// Panics if metadata is set with a timestamp unit.
func NewStringWithOptions(name string, opt *Options, description string, metadata *types.MetricMetadata, fields ...field.Field) String {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	if metadata.Units.IsTime() {
		panic(fmt.Errorf(
			"timeunit %q cannot be given to string-valued metric %q",
			metadata.Units, name))
	}
	opt = applyDefaultOptions(opt)
	m := &stringMetric{metric{
		MetricInfo: types.MetricInfo{
			Name:        name,
			Description: description,
			Fields:      fields,
			ValueType:   types.StringType,
			TargetType:  opt.TargetType,
		},
		MetricMetadata: *metadata,
	}}
	opt.Registry.Add(m)
	return m
}

// NewBool returns a new bool-valued metric.
func NewBool(name string, description string, metadata *types.MetricMetadata, fields ...field.Field) Bool {
	return NewBoolWithOptions(name, nil, description, metadata, fields...)
}

// NewBoolWithOptions creates a new bool-valued metric with given Options.
//
// Panics if metadata is set with a timestamp unit.
func NewBoolWithOptions(name string, opt *Options, description string, metadata *types.MetricMetadata, fields ...field.Field) Bool {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	if metadata.Units.IsTime() {
		panic(fmt.Errorf(
			"timeunit %q cannot be given to bool-valued metric %q",
			metadata.Units, name))
	}
	opt = applyDefaultOptions(opt)
	m := &boolMetric{metric{
		MetricInfo: types.MetricInfo{
			Name:        name,
			Description: description,
			Fields:      fields,
			ValueType:   types.BoolType,
			TargetType:  opt.TargetType,
		},
		MetricMetadata: *metadata,
	}}
	opt.Registry.Add(m)
	return m
}

// NewCumulativeDistribution returns a new cumulative-distribution-valued
// metric.
func NewCumulativeDistribution(name string, description string, metadata *types.MetricMetadata, bucketer *distribution.Bucketer, fields ...field.Field) CumulativeDistribution {
	return NewCumulativeDistributionWithOptions(name, nil, description, metadata, bucketer, fields...)
}

// NewCumulativeDistributionWithOptions creates a new cumulative-distribution-valued
// metric with given Options
func NewCumulativeDistributionWithOptions(name string, opt *Options, description string, metadata *types.MetricMetadata, bucketer *distribution.Bucketer, fields ...field.Field) CumulativeDistribution {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	opt = applyDefaultOptions(opt)
	m := &cumulativeDistributionMetric{
		nonCumulativeDistributionMetric{
			metric: metric{
				MetricInfo: types.MetricInfo{
					Name:        name,
					Description: description,
					Fields:      fields,
					ValueType:   types.CumulativeDistributionType,
					TargetType:  opt.TargetType,
				},
				MetricMetadata: *metadata,
			},
			bucketer: bucketer,
		},
	}
	opt.Registry.Add(m)
	return m
}

// NewNonCumulativeDistribution returns a new non-cumulative-distribution-valued
// metric.
func NewNonCumulativeDistribution(name string, description string, metadata *types.MetricMetadata, bucketer *distribution.Bucketer, fields ...field.Field) NonCumulativeDistribution {
	return NewNonCumulativeDistributionWithOptions(name, nil, description, metadata, bucketer, fields...)
}

// NewNonCumulativeDistributionWithOptions creates a new non-cumulative-distribution-valued
// metric with given Options.
func NewNonCumulativeDistributionWithOptions(name string, opt *Options, description string, metadata *types.MetricMetadata, bucketer *distribution.Bucketer, fields ...field.Field) NonCumulativeDistribution {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	opt = applyDefaultOptions(opt)
	m := &nonCumulativeDistributionMetric{
		metric: metric{
			MetricInfo: types.MetricInfo{
				Name:        name,
				Description: description,
				Fields:      fields,
				ValueType:   types.NonCumulativeDistributionType,
				TargetType:  opt.TargetType,
			},
			MetricMetadata: *metadata,
		},
		bucketer: bucketer,
	}
	opt.Registry.Add(m)
	return m
}

// genericGet is a convenience function that tries to get a metric value from
// the store and returns the zero value if it didn't exist.
func (m *metric) genericGet(zero any, ctx context.Context, fieldVals []any) any {
	if ret := tsmon.Store(ctx).Get(ctx, m, fieldVals); ret != nil {
		return ret
	}
	return zero
}

type metric struct {
	types.MetricInfo
	types.MetricMetadata
}

func (m *metric) Info() types.MetricInfo         { return m.MetricInfo }
func (m *metric) Metadata() types.MetricMetadata { return m.MetricMetadata }

type intMetric struct{ metric }

func (m *intMetric) Get(ctx context.Context, fieldVals ...any) int64 {
	return m.genericGet(int64(0), ctx, fieldVals).(int64)
}

func (m *intMetric) Set(ctx context.Context, v int64, fieldVals ...any) {
	tsmon.Store(ctx).Set(ctx, m, fieldVals, v)
}

type counter struct{ intMetric }

func (m *counter) Add(ctx context.Context, n int64, fieldVals ...any) {
	tsmon.Store(ctx).Incr(ctx, m, fieldVals, n)
}

type floatMetric struct{ metric }

func (m *floatMetric) Get(ctx context.Context, fieldVals ...any) float64 {
	return m.genericGet(float64(0.0), ctx, fieldVals).(float64)
}

func (m *floatMetric) Set(ctx context.Context, v float64, fieldVals ...any) {
	tsmon.Store(ctx).Set(ctx, m, fieldVals, v)
}

type floatCounter struct{ floatMetric }

func (m *floatCounter) Add(ctx context.Context, n float64, fieldVals ...any) {
	tsmon.Store(ctx).Incr(ctx, m, fieldVals, n)
}

type stringMetric struct{ metric }

func (m *stringMetric) Get(ctx context.Context, fieldVals ...any) string {
	return m.genericGet("", ctx, fieldVals).(string)
}

func (m *stringMetric) Set(ctx context.Context, v string, fieldVals ...any) {
	tsmon.Store(ctx).Set(ctx, m, fieldVals, v)
}

type boolMetric struct{ metric }

func (m *boolMetric) Get(ctx context.Context, fieldVals ...any) bool {
	return m.genericGet(false, ctx, fieldVals).(bool)
}

func (m *boolMetric) Set(ctx context.Context, v bool, fieldVals ...any) {
	tsmon.Store(ctx).Set(ctx, m, fieldVals, v)
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
	tsmon.Store(ctx).Set(ctx, m, fieldVals, v)
}

type cumulativeDistributionMetric struct {
	nonCumulativeDistributionMetric
}

func (m *cumulativeDistributionMetric) Add(ctx context.Context, v float64, fieldVals ...any) {
	tsmon.Store(ctx).Incr(ctx, m, fieldVals, v)
}
