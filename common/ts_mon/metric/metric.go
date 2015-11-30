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
	"github.com/luci/luci-go/common/ts_mon/field"
	"github.com/luci/luci-go/common/ts_mon/iface"
	"github.com/luci/luci-go/common/ts_mon/types"
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

// NewInt returns a new non-cumulative integer gauge metric.  This will panic if
// another metric already exists with this name.
func NewInt(name string, fields ...field.Field) Int {
	m := &intMetric{metric{name: name, fields: fields, typ: types.NonCumulativeIntType}}
	if err := iface.Store.Register(m); err != nil {
		panic(err)
	}
	return m
}

// NewCounter returns a new cumulative integer metric.  This will panic if
// another metric already exists with this name.
func NewCounter(name string, fields ...field.Field) Counter {
	m := &counter{intMetric{metric{name: name, fields: fields, typ: types.CumulativeIntType}}}
	if err := iface.Store.Register(m); err != nil {
		panic(err)
	}
	return m
}

// NewFloat returns a new non-cumulative floating-point gauge metric.  This will
// panic if another metric already exists with this name.
func NewFloat(name string, fields ...field.Field) Float {
	m := &floatMetric{metric{name: name, fields: fields, typ: types.NonCumulativeFloatType}}
	if err := iface.Store.Register(m); err != nil {
		panic(err)
	}
	return m
}

// NewFloatCounter returns a new cumulative floating-point metric.  This will
// panic if another metric already exists with this name.
func NewFloatCounter(name string, fields ...field.Field) FloatCounter {
	m := &floatCounter{floatMetric{metric{name: name, fields: fields, typ: types.CumulativeFloatType}}}
	if err := iface.Store.Register(m); err != nil {
		panic(err)
	}
	return m
}

// NewString returns a new string-valued metric.  This will panic if another
// metric already exists with this name.
func NewString(name string, fields ...field.Field) String {
	m := &stringMetric{metric{name: name, fields: fields, typ: types.StringType}}
	if err := iface.Store.Register(m); err != nil {
		panic(err)
	}
	return m
}

// NewBool returns a new bool-valued metric.  This will panic if another
// metric already exists with this name.
func NewBool(name string, fields ...field.Field) Bool {
	m := &boolMetric{metric{name: name, fields: fields, typ: types.BoolType}}
	if err := iface.Store.Register(m); err != nil {
		panic(err)
	}
	return m
}

type metric struct {
	name   string
	fields []field.Field
	typ    types.ValueType
}

func (m *metric) Name() string               { return m.name }
func (m *metric) Fields() []field.Field      { return m.fields }
func (m *metric) ValueType() types.ValueType { return m.typ }

type intMetric struct{ metric }

func (m *intMetric) Get(ctx context.Context, fieldVals ...interface{}) (int64, error) {
	ret, err := iface.Store.Get(ctx, m.name, fieldVals)
	if err != nil {
		return 0, err
	}
	if ret == nil {
		return 0, nil
	}
	return ret.(int64), nil
}

func (m *intMetric) Set(ctx context.Context, v int64, fieldVals ...interface{}) error {
	return iface.Store.Set(ctx, m.name, fieldVals, v)
}

type counter struct{ intMetric }

func (m *counter) Add(ctx context.Context, n int64, fieldVals ...interface{}) error {
	return iface.Store.Incr(ctx, m.name, fieldVals, n)
}

type floatMetric struct{ metric }

func (m *floatMetric) Get(ctx context.Context, fieldVals ...interface{}) (float64, error) {
	ret, err := iface.Store.Get(ctx, m.name, fieldVals)
	if err != nil {
		return 0, err
	}
	if ret == nil {
		return 0, nil
	}
	return ret.(float64), nil
}

func (m *floatMetric) Set(ctx context.Context, v float64, fieldVals ...interface{}) error {
	return iface.Store.Set(ctx, m.name, fieldVals, v)
}

type floatCounter struct{ floatMetric }

func (m *floatCounter) Add(ctx context.Context, n float64, fieldVals ...interface{}) error {
	return iface.Store.Incr(ctx, m.name, fieldVals, n)
}

type stringMetric struct{ metric }

func (m *stringMetric) Get(ctx context.Context, fieldVals ...interface{}) (string, error) {
	ret, err := iface.Store.Get(ctx, m.name, fieldVals)
	if err != nil {
		return "", err
	}
	if ret == nil {
		return "", nil
	}
	return ret.(string), nil
}

func (m *stringMetric) Set(ctx context.Context, v string, fieldVals ...interface{}) error {
	return iface.Store.Set(ctx, m.name, fieldVals, v)
}

type boolMetric struct{ metric }

func (m *boolMetric) Get(ctx context.Context, fieldVals ...interface{}) (bool, error) {
	ret, err := iface.Store.Get(ctx, m.name, fieldVals)
	if err != nil {
		return false, err
	}
	if ret == nil {
		return false, nil
	}
	return ret.(bool), nil
}

func (m *boolMetric) Set(ctx context.Context, v bool, fieldVals ...interface{}) error {
	return iface.Store.Set(ctx, m.name, fieldVals, v)
}
