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
// fields on that metric. It is an error to define two metrics with the same
// name (this will cause a panic).
//
// When setting metric values, you must pass correct number of fields, with
// correct types, or the setter function will panic.
//
// Example:
//   var (
//     requests = metric.NewCounter("myapp/requests",
//       "Number of requests",
//       nil,
//       field.String("status"))
//   )
//   ...
//   func handleRequest() {
//     if success {
//       requests.Add(1, "success")
//     } else {
//       requests.Add(1, "failure")
//     }
//   }
package metric

import (
	"time"

	"golang.org/x/net/context"

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/registry"
	"go.chromium.org/luci/common/tsmon/types"
)

// Int is a non-cumulative integer gauge metric.
type Int interface {
	types.Metric

	Get(c context.Context, fieldVals ...interface{}) int64
	Set(c context.Context, v int64, fieldVals ...interface{})
}

// Counter is a cumulative integer metric.
type Counter interface {
	Int

	Add(c context.Context, n int64, fieldVals ...interface{})
}

// Float is a non-cumulative floating-point gauge metric.
type Float interface {
	types.Metric

	Get(c context.Context, fieldVals ...interface{}) float64
	Set(c context.Context, v float64, fieldVals ...interface{})
}

// FloatCounter is a cumulative floating-point metric.
type FloatCounter interface {
	Float

	Add(c context.Context, n float64, fieldVals ...interface{})
}

// String is a string-valued metric.
type String interface {
	types.Metric

	Get(c context.Context, fieldVals ...interface{}) string
	Set(c context.Context, v string, fieldVals ...interface{})
}

// Bool is a boolean-valued metric.
type Bool interface {
	types.Metric

	Get(c context.Context, fieldVals ...interface{}) bool
	Set(c context.Context, v bool, fieldVals ...interface{})
}

// NonCumulativeDistribution is a non-cumulative-distribution-valued metric.
type NonCumulativeDistribution interface {
	types.DistributionMetric

	Get(c context.Context, fieldVals ...interface{}) *distribution.Distribution
	Set(c context.Context, d *distribution.Distribution, fieldVals ...interface{})
}

// CumulativeDistribution is a cumulative-distribution-valued metric.
type CumulativeDistribution interface {
	NonCumulativeDistribution

	Add(c context.Context, v float64, fieldVals ...interface{})
}

// NewInt returns a new non-cumulative integer gauge metric.
func NewInt(name string, description string, metadata *types.MetricMetadata, fields ...field.Field) Int {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	m := &intMetric{metric{
		MetricInfo: types.MetricInfo{
			Name:        name,
			Description: description,
			Fields:      fields,
			ValueType:   types.NonCumulativeIntType,
		},
		MetricMetadata: *metadata,
	}}
	registry.Add(m)
	return m
}

// NewCounter returns a new cumulative integer metric.
func NewCounter(name string, description string, metadata *types.MetricMetadata, fields ...field.Field) Counter {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	m := &counter{intMetric{metric{
		MetricInfo: types.MetricInfo{
			Name:        name,
			Description: description,
			Fields:      fields,
			ValueType:   types.CumulativeIntType,
		},
		MetricMetadata: *metadata,
	}}}
	registry.Add(m)
	return m
}

// NewFloat returns a new non-cumulative floating-point gauge metric.
func NewFloat(name string, description string, metadata *types.MetricMetadata, fields ...field.Field) Float {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	m := &floatMetric{metric{
		MetricInfo: types.MetricInfo{
			Name:        name,
			Description: description,
			Fields:      fields,
			ValueType:   types.NonCumulativeFloatType,
		},
		MetricMetadata: *metadata,
	}}
	registry.Add(m)
	return m
}

// NewFloatCounter returns a new cumulative floating-point metric.
func NewFloatCounter(name string, description string, metadata *types.MetricMetadata, fields ...field.Field) FloatCounter {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	m := &floatCounter{floatMetric{metric{
		MetricInfo: types.MetricInfo{
			Name:        name,
			Description: description,
			Fields:      fields,
			ValueType:   types.CumulativeFloatType,
		},
		MetricMetadata: *metadata,
	}}}
	registry.Add(m)
	return m
}

// NewString returns a new string-valued metric.
func NewString(name string, description string, metadata *types.MetricMetadata, fields ...field.Field) String {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	m := &stringMetric{metric{
		MetricInfo: types.MetricInfo{
			Name:        name,
			Description: description,
			Fields:      fields,
			ValueType:   types.StringType,
		},
		MetricMetadata: *metadata,
	}}
	registry.Add(m)
	return m
}

// NewBool returns a new bool-valued metric.
func NewBool(name string, description string, metadata *types.MetricMetadata, fields ...field.Field) Bool {
	if metadata == nil {
		metadata = &types.MetricMetadata{}
	}
	m := &boolMetric{metric{
		MetricInfo: types.MetricInfo{
			Name:        name,
			Description: description,
			Fields:      fields,
			ValueType:   types.BoolType,
		},
		MetricMetadata: *metadata,
	}}
	registry.Add(m)
	return m
}

// NewCumulativeDistribution returns a new cumulative-distribution-valued
// metric.
func NewCumulativeDistribution(name string, description string, metadata *types.MetricMetadata, bucketer *distribution.Bucketer, fields ...field.Field) CumulativeDistribution {
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
				},
				MetricMetadata: *metadata,
			},
			bucketer: bucketer,
		},
	}
	registry.Add(m)
	return m
}

// NewNonCumulativeDistribution returns a new non-cumulative-distribution-valued
// metric.
func NewNonCumulativeDistribution(name string, description string, metadata *types.MetricMetadata, bucketer *distribution.Bucketer, fields ...field.Field) NonCumulativeDistribution {
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
			},
			MetricMetadata: *metadata,
		},
		bucketer: bucketer,
	}
	registry.Add(m)
	return m
}

// genericGet is a convenience function that tries to get a metric value from
// the store and returns the zero value if it didn't exist.
func (m *metric) genericGet(zero interface{}, c context.Context, fieldVals []interface{}) interface{} {
	if ret := tsmon.Store(c).Get(c, m, m.fixedResetTime, fieldVals); ret != nil {
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

func (m *intMetric) Get(c context.Context, fieldVals ...interface{}) int64 {
	return m.genericGet(int64(0), c, fieldVals).(int64)
}

func (m *intMetric) Set(c context.Context, v int64, fieldVals ...interface{}) {
	tsmon.Store(c).Set(c, m, m.fixedResetTime, fieldVals, v)
}

type counter struct{ intMetric }

func (m *counter) Add(c context.Context, n int64, fieldVals ...interface{}) {
	tsmon.Store(c).Incr(c, m, m.fixedResetTime, fieldVals, n)
}

type floatMetric struct{ metric }

func (m *floatMetric) Get(c context.Context, fieldVals ...interface{}) float64 {
	return m.genericGet(float64(0.0), c, fieldVals).(float64)
}

func (m *floatMetric) Set(c context.Context, v float64, fieldVals ...interface{}) {
	tsmon.Store(c).Set(c, m, m.fixedResetTime, fieldVals, v)
}

type floatCounter struct{ floatMetric }

func (m *floatCounter) Add(c context.Context, n float64, fieldVals ...interface{}) {
	tsmon.Store(c).Incr(c, m, m.fixedResetTime, fieldVals, n)
}

type stringMetric struct{ metric }

func (m *stringMetric) Get(c context.Context, fieldVals ...interface{}) string {
	return m.genericGet("", c, fieldVals).(string)
}

func (m *stringMetric) Set(c context.Context, v string, fieldVals ...interface{}) {
	tsmon.Store(c).Set(c, m, m.fixedResetTime, fieldVals, v)
}

type boolMetric struct{ metric }

func (m *boolMetric) Get(c context.Context, fieldVals ...interface{}) bool {
	return m.genericGet(false, c, fieldVals).(bool)
}

func (m *boolMetric) Set(c context.Context, v bool, fieldVals ...interface{}) {
	tsmon.Store(c).Set(c, m, m.fixedResetTime, fieldVals, v)
}

type nonCumulativeDistributionMetric struct {
	metric
	bucketer *distribution.Bucketer
}

func (m *nonCumulativeDistributionMetric) Bucketer() *distribution.Bucketer { return m.bucketer }

func (m *nonCumulativeDistributionMetric) Get(c context.Context, fieldVals ...interface{}) *distribution.Distribution {
	ret := m.genericGet((*distribution.Distribution)(nil), c, fieldVals)
	return ret.(*distribution.Distribution)
}

func (m *nonCumulativeDistributionMetric) Set(c context.Context, v *distribution.Distribution, fieldVals ...interface{}) {
	tsmon.Store(c).Set(c, m, m.fixedResetTime, fieldVals, v)
}

type cumulativeDistributionMetric struct {
	nonCumulativeDistributionMetric
}

func (m *cumulativeDistributionMetric) Add(c context.Context, v float64, fieldVals ...interface{}) {
	tsmon.Store(c).Incr(c, m, m.fixedResetTime, fieldVals, v)
}
