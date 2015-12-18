// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package store

import (
	"fmt"
	"reflect"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/types"
	"golang.org/x/net/context"
)

// InMemoryStore holds metric data in this process' memory.
type InMemoryStore map[string]metricData

type metricData struct {
	fields []field.Field
	typ    types.ValueType
	cells  map[uint64][]*cell
}

type cell struct {
	fieldVals []interface{}
	resetTime time.Time
	value     interface{}
}

func (m *metricData) get(fieldVals []interface{}, resetTime time.Time) (*cell, error) {
	fieldVals, err := field.Canonicalize(m.fields, fieldVals)
	if err != nil {
		return nil, err
	}

	key := field.Hash(fieldVals)
	cells, ok := m.cells[key]
	if ok {
		for _, cell := range cells {
			if reflect.DeepEqual(fieldVals, cell.fieldVals) {
				return cell, nil
			}
		}
	}

	cell := &cell{fieldVals, resetTime, nil}
	m.cells[key] = append(cells, cell)
	return cell, nil
}

// Register adds the metric to the store.  Must be called before Get, Set or
// Incr for this metric.  Returns an error if the metric is already registered.
func (s InMemoryStore) Register(m types.Metric) error {
	if _, ok := s[m.Name()]; ok {
		return fmt.Errorf("A metric with the name '%s' was already registered", m.Name())
	}

	s[m.Name()] = metricData{
		fields: m.Fields(),
		cells:  map[uint64][]*cell{},
		typ:    m.ValueType(),
	}
	return nil
}

// Unregister removes the metric (along with all its values) from the store.
func (s InMemoryStore) Unregister(name string) {
	delete(s, name)
}

// Get returns the value for a given metric cell.
func (s InMemoryStore) Get(ctx context.Context, name string, fieldVals []interface{}) (value interface{}, err error) {
	m, ok := s[name]
	if !ok {
		return nil, fmt.Errorf("metric %s is not registered", name)
	}

	c, err := m.get(fieldVals, clock.Now(ctx))
	if err != nil {
		return nil, err
	}

	return c.value, nil
}

// Set writes the value into the given metric cell.
func (s InMemoryStore) Set(ctx context.Context, name string, fieldVals []interface{}, value interface{}) error {
	m, ok := s[name]
	if !ok {
		return fmt.Errorf("metric %s is not registered", name)
	}

	c, err := m.get(fieldVals, clock.Now(ctx))
	if err != nil {
		return err
	}

	c.value = value
	return nil
}

// Incr increments the value in a given metric cell by the given delta.
func (s InMemoryStore) Incr(ctx context.Context, name string, fieldVals []interface{}, delta interface{}) error {
	m, ok := s[name]
	if !ok {
		return fmt.Errorf("metric %s is not registered", name)
	}

	c, err := m.get(fieldVals, clock.Now(ctx))
	if err != nil {
		return err
	}

	switch d := delta.(type) {
	case int64:
		if v, ok := c.value.(int64); ok {
			c.value = v + d
		} else {
			c.value = d
		}
	case float64:
		if v, ok := c.value.(float64); ok {
			c.value = v + d
		} else {
			c.value = d
		}
	default:
		return fmt.Errorf("Incr got a delta of unsupported type (%v)", delta)
	}

	return nil
}

// ResetForUnittest resets all metric values without unregistering any metrics.
// Useful for unit tests.
func (s InMemoryStore) ResetForUnittest() {
	for _, m := range s {
		m.cells = make(map[uint64][]*cell)
	}
}

// GetAll efficiently returns all cells in the store.
func (s InMemoryStore) GetAll(ctx context.Context) []types.Cell {
	ret := []types.Cell{}
	for name, m := range s {
		for _, cells := range m.cells {
			for _, cell := range cells {
				ret = append(ret, types.Cell{
					MetricName: name,
					Fields:     m.fields,
					ValueType:  m.typ,
					FieldVals:  cell.fieldVals,
					ResetTime:  cell.resetTime,
					Value:      cell.value,
				})
			}
		}
	}
	return ret
}
