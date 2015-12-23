// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package metric is the API for defining metrics and updating their values.
//
// When you define a metric you must also define the names and types of any
// fields on that metric.  It is an error to define two metrics with the same
// name (this will cause a panic).
//
// Example:
//   var (
//     Requests = metric.NewCounter("myapp/requests", field.String("status"))
//   )
//   ...
//   func handleRequest() {
//     if success {
//       Requests.Add(1, "success")
//     } else {
//       Requests.Add(1, "failure")
//     }
//   }
package metric

import (
	"time"

	"github.com/luci/luci-go/common/tsmon"
	"github.com/luci/luci-go/common/tsmon/distribution"
	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/store"
	"github.com/luci/luci-go/common/tsmon/types"
	"golang.org/x/net/context"
)

// Int is a non-cumulative integer gauge metric.
type Int interface {
	types.Metric

	Get(ctx context.Context, fieldVals ...interface{}) (int64, error)
	Set(ctx context.Context, v int64, fieldVals ...interface{}) error
}

// Counter is a cumulative integer metric.
type Counter interface {
	Int

	Add(ctx context.Context, n int64, fieldVals ...interface{}) error
}

// Float is a non-cumulative floating-point gauge metric.
type Float interface {
	types.Metric

	Get(ctx context.Context, fieldVals ...interface{}) (float64, error)
	Set(ctx context.Context, v float64, fieldVals ...interface{}) error
}

// FloatCounter is a cumulative floating-point metric.
type FloatCounter interface {
	Float

	Add(ctx context.Context, n float64, fieldVals ...interface{}) error
}

// String is a string-valued metric.
type String interface {
	types.Metric

	Get(ctx context.Context, fieldVals ...interface{}) (string, error)
	Set(ctx context.Context, v string, fieldVals ...interface{}) error
}

// Bool is a boolean-valued metric.
type Bool interface {
	types.Metric

	Get(ctx context.Context, fieldVals ...interface{}) (bool, error)
	Set(ctx context.Context, v bool, fieldVals ...interface{}) error
}

// CumulativeDistribution is a cumulative-distribution-valued metric.
type CumulativeDistribution interface {
	types.DistributionMetric

	Get(ctx context.Context, fieldVals ...interface{}) (*distribution.Distribution, error)
	Add(ctx context.Context, v float64, fieldVals ...interface{}) error
}

// NonCumulativeDistribution is a non-cumulative-distribution-valued metric.
type NonCumulativeDistribution interface {
	CumulativeDistribution

	Set(ctx context.Context, d *distribution.Distribution, fieldVals ...interface{}) error
}

// NewInt returns a new non-cumulative integer gauge metric.  This will panic if
// another metric already exists with this name.
func NewInt(name string, fields ...field.Field) Int {
	m := &intMetric{metric{MetricInfo: types.MetricInfo{
		MetricName: name,
		Fields:     fields,
		ValueType:  types.NonCumulativeIntType,
	}}}
	h, err := tsmon.Store.Register(m)
	if err != nil {
		panic(err)
	}
	m.handle = h
	return m
}

// NewCounter returns a new cumulative integer metric.  This will panic if
// another metric already exists with this name.
func NewCounter(name string, fields ...field.Field) Counter {
	m := &counter{intMetric{metric{MetricInfo: types.MetricInfo{
		MetricName: name,
		Fields:     fields,
		ValueType:  types.CumulativeIntType,
	}}}}
	h, err := tsmon.Store.Register(m)
	if err != nil {
		panic(err)
	}
	m.handle = h
	return m
}

// NewFloat returns a new non-cumulative floating-point gauge metric.  This will
// panic if another metric already exists with this name.
func NewFloat(name string, fields ...field.Field) Float {
	m := &floatMetric{metric{MetricInfo: types.MetricInfo{
		MetricName: name,
		Fields:     fields,
		ValueType:  types.NonCumulativeFloatType,
	}}}
	h, err := tsmon.Store.Register(m)
	if err != nil {
		panic(err)
	}
	m.handle = h
	return m
}

// NewFloatCounter returns a new cumulative floating-point metric.  This will
// panic if another metric already exists with this name.
func NewFloatCounter(name string, fields ...field.Field) FloatCounter {
	m := &floatCounter{floatMetric{metric{MetricInfo: types.MetricInfo{
		MetricName: name,
		Fields:     fields,
		ValueType:  types.CumulativeFloatType,
	}}}}
	h, err := tsmon.Store.Register(m)
	if err != nil {
		panic(err)
	}
	m.handle = h
	return m
}

// NewString returns a new string-valued metric.  This will panic if another
// metric already exists with this name.
func NewString(name string, fields ...field.Field) String {
	m := &stringMetric{metric{MetricInfo: types.MetricInfo{
		MetricName: name,
		Fields:     fields,
		ValueType:  types.StringType,
	}}}
	h, err := tsmon.Store.Register(m)
	if err != nil {
		panic(err)
	}
	m.handle = h
	return m
}

// NewBool returns a new bool-valued metric.  This will panic if another
// metric already exists with this name.
func NewBool(name string, fields ...field.Field) Bool {
	m := &boolMetric{metric{MetricInfo: types.MetricInfo{
		MetricName: name,
		Fields:     fields,
		ValueType:  types.BoolType,
	}}}
	h, err := tsmon.Store.Register(m)
	if err != nil {
		panic(err)
	}
	m.handle = h
	return m
}

// NewCumulativeDistribution returns a new cumulative-distribution-valued
// metric.  This will panic if another metric already exists with this name.
func NewCumulativeDistribution(name string, bucketer *distribution.Bucketer, fields ...field.Field) CumulativeDistribution {
	m := &cumulativeDistributionMetric{
		metric: metric{MetricInfo: types.MetricInfo{
			MetricName: name,
			Fields:     fields,
			ValueType:  types.CumulativeDistributionType,
		}},
		bucketer: bucketer,
	}
	h, err := tsmon.Store.Register(m)
	if err != nil {
		panic(err)
	}
	m.handle = h
	return m
}

// NewNonCumulativeDistribution returns a new non-cumulative-distribution-valued
// metric.  This will panic if another metric already exists with this name.
func NewNonCumulativeDistribution(name string, bucketer *distribution.Bucketer, fields ...field.Field) NonCumulativeDistribution {
	m := &nonCumulativeDistributionMetric{
		cumulativeDistributionMetric{
			metric: metric{MetricInfo: types.MetricInfo{
				MetricName: name,
				Fields:     fields,
				ValueType:  types.NonCumulativeDistributionType,
			}},
			bucketer: bucketer,
		},
	}
	h, err := tsmon.Store.Register(m)
	if err != nil {
		panic(err)
	}
	m.handle = h
	return m
}

type metric struct {
	types.MetricInfo

	handle         store.MetricHandle
	fixedResetTime time.Time
}

func (m *metric) Name() string                  { return m.MetricInfo.MetricName }
func (m *metric) Fields() []field.Field         { return m.MetricInfo.Fields }
func (m *metric) ValueType() types.ValueType    { return m.MetricInfo.ValueType }
func (m *metric) SetFixedResetTime(t time.Time) { m.fixedResetTime = t }

type intMetric struct{ metric }

func (m *intMetric) Get(ctx context.Context, fieldVals ...interface{}) (int64, error) {
	ret, err := tsmon.Store.Get(ctx, m.handle, m.fixedResetTime, fieldVals)
	if err != nil {
		return 0, err
	}
	if ret == nil {
		return 0, nil
	}
	return ret.(int64), nil
}

func (m *intMetric) Set(ctx context.Context, v int64, fieldVals ...interface{}) error {
	return tsmon.Store.Set(ctx, m.handle, m.fixedResetTime, fieldVals, v)
}

type counter struct{ intMetric }

func (m *counter) Add(ctx context.Context, n int64, fieldVals ...interface{}) error {
	return tsmon.Store.Incr(ctx, m.handle, m.fixedResetTime, fieldVals, n)
}

type floatMetric struct{ metric }

func (m *floatMetric) Get(ctx context.Context, fieldVals ...interface{}) (float64, error) {
	ret, err := tsmon.Store.Get(ctx, m.handle, m.fixedResetTime, fieldVals)
	if err != nil {
		return 0, err
	}
	if ret == nil {
		return 0, nil
	}
	return ret.(float64), nil
}

func (m *floatMetric) Set(ctx context.Context, v float64, fieldVals ...interface{}) error {
	return tsmon.Store.Set(ctx, m.handle, m.fixedResetTime, fieldVals, v)
}

type floatCounter struct{ floatMetric }

func (m *floatCounter) Add(ctx context.Context, n float64, fieldVals ...interface{}) error {
	return tsmon.Store.Incr(ctx, m.handle, m.fixedResetTime, fieldVals, n)
}

type stringMetric struct{ metric }

func (m *stringMetric) Get(ctx context.Context, fieldVals ...interface{}) (string, error) {
	ret, err := tsmon.Store.Get(ctx, m.handle, m.fixedResetTime, fieldVals)
	if err != nil {
		return "", err
	}
	if ret == nil {
		return "", nil
	}
	return ret.(string), nil
}

func (m *stringMetric) Set(ctx context.Context, v string, fieldVals ...interface{}) error {
	return tsmon.Store.Set(ctx, m.handle, m.fixedResetTime, fieldVals, v)
}

type boolMetric struct{ metric }

func (m *boolMetric) Get(ctx context.Context, fieldVals ...interface{}) (bool, error) {
	ret, err := tsmon.Store.Get(ctx, m.handle, m.fixedResetTime, fieldVals)
	if err != nil {
		return false, err
	}
	if ret == nil {
		return false, nil
	}
	return ret.(bool), nil
}

func (m *boolMetric) Set(ctx context.Context, v bool, fieldVals ...interface{}) error {
	return tsmon.Store.Set(ctx, m.handle, m.fixedResetTime, fieldVals, v)
}

type cumulativeDistributionMetric struct {
	metric
	bucketer *distribution.Bucketer
}

func (m *cumulativeDistributionMetric) Bucketer() *distribution.Bucketer { return m.bucketer }

func (m *cumulativeDistributionMetric) Get(ctx context.Context, fieldVals ...interface{}) (*distribution.Distribution, error) {
	ret, err := tsmon.Store.Get(ctx, m.handle, m.fixedResetTime, fieldVals)
	if err != nil {
		return nil, err
	}
	if ret == nil {
		return nil, nil
	}
	return ret.(*distribution.Distribution), nil
}

func (m *cumulativeDistributionMetric) Add(ctx context.Context, v float64, fieldVals ...interface{}) error {
	return tsmon.Store.Incr(ctx, m.handle, m.fixedResetTime, fieldVals, v)
}

type nonCumulativeDistributionMetric struct{ cumulativeDistributionMetric }

func (m *nonCumulativeDistributionMetric) Set(ctx context.Context, v *distribution.Distribution, fieldVals ...interface{}) error {
	return tsmon.Store.Set(ctx, m.handle, m.fixedResetTime, fieldVals, v)
}
