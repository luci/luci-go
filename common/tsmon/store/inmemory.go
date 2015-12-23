// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package store

import (
	"fmt"
	"reflect"
	"sync"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/tsmon/distribution"
	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/types"
	"golang.org/x/net/context"
)

type inMemoryStore struct {
	data map[string]*metricData
	lock sync.RWMutex
}

type metricData struct {
	types.MetricInfo

	cells map[uint64][]*types.CellData
	lock  sync.Mutex

	bucketer *distribution.Bucketer
}

func (m *metricData) get(fieldVals []interface{}, resetTime time.Time) (*types.CellData, error) {
	fieldVals, err := field.Canonicalize(m.Fields, fieldVals)
	if err != nil {
		return nil, err
	}

	key := field.Hash(fieldVals)
	cells, ok := m.cells[key]
	if ok {
		for _, cell := range cells {
			if reflect.DeepEqual(fieldVals, cell.FieldVals) {
				return cell, nil
			}
		}
	}

	cell := &types.CellData{fieldVals, resetTime, nil}
	m.cells[key] = append(cells, cell)
	return cell, nil
}

// NewInMemory creates a new metric store that holds metric data in this
// process' memory.
func NewInMemory() Store {
	return &inMemoryStore{
		data: map[string]*metricData{},
	}
}

// Register adds the metric to the store.  Must be called before Get, Set or
// Incr for this metric.  Returns an error if the metric is already registered.
func (s *inMemoryStore) Register(m types.Metric) (MetricHandle, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if _, ok := s.data[m.Name()]; ok {
		return nil, fmt.Errorf("A metric with the name '%s' was already registered", m.Name())
	}

	d := &metricData{
		MetricInfo: types.MetricInfo{
			MetricName: m.Name(),
			Fields:     m.Fields(),
			ValueType:  m.ValueType(),
		},
		cells: map[uint64][]*types.CellData{},
	}

	if dist, ok := m.(types.DistributionMetric); ok {
		d.bucketer = dist.Bucketer()
	}

	s.data[m.Name()] = d
	return d, nil
}

// Unregister removes the metric (along with all its values) from the store.
func (s *inMemoryStore) Unregister(h MetricHandle) {
	name := h.(*metricData).MetricName

	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.data, name)
}

// Get returns the value for a given metric cell.
func (s *inMemoryStore) Get(ctx context.Context, h MetricHandle, resetTime time.Time, fieldVals []interface{}) (value interface{}, err error) {
	if resetTime.IsZero() {
		resetTime = clock.Now(ctx)
	}

	m := h.(*metricData)
	m.lock.Lock()
	defer m.lock.Unlock()

	c, err := m.get(fieldVals, resetTime)
	if err != nil {
		return nil, err
	}

	return c.Value, nil
}

// Set writes the value into the given metric cell.
func (s *inMemoryStore) Set(ctx context.Context, h MetricHandle, resetTime time.Time, fieldVals []interface{}, value interface{}) error {
	if resetTime.IsZero() {
		resetTime = clock.Now(ctx)
	}

	m := h.(*metricData)
	m.lock.Lock()
	defer m.lock.Unlock()

	c, err := m.get(fieldVals, resetTime)
	if err != nil {
		return err
	}

	c.Value = value
	return nil
}

// Incr increments the value in a given metric cell by the given delta.
func (s *inMemoryStore) Incr(ctx context.Context, h MetricHandle, resetTime time.Time, fieldVals []interface{}, delta interface{}) error {
	if resetTime.IsZero() {
		resetTime = clock.Now(ctx)
	}

	m := h.(*metricData)
	m.lock.Lock()
	defer m.lock.Unlock()

	c, err := m.get(fieldVals, resetTime)
	if err != nil {
		return err
	}

	if m.ValueType == types.CumulativeDistributionType || m.ValueType == types.NonCumulativeDistributionType {
		d, ok := delta.(float64)
		if !ok {
			return fmt.Errorf("Incr got a delta of unsupported type (%v)", delta)
		}
		v, ok := c.Value.(*distribution.Distribution)
		if !ok {
			v = distribution.New(m.bucketer)
			c.Value = v
		}
		v.Add(float64(d))
	} else {
		switch d := delta.(type) {
		case int64:
			if v, ok := c.Value.(int64); ok {
				c.Value = v + d
			} else {
				c.Value = d
			}
		case float64:
			if v, ok := c.Value.(float64); ok {
				c.Value = v + d
			} else {
				c.Value = d
			}
		default:
			return fmt.Errorf("Incr got a delta of unsupported type (%v)", delta)
		}
	}

	return nil
}

// ResetForUnittest resets all metric values without unregistering any metrics.
// Useful for unit tests.
func (s *inMemoryStore) ResetForUnittest() {
	s.lock.Lock()
	defer s.lock.Unlock()

	for _, m := range s.data {
		m.lock.Lock()
		m.cells = make(map[uint64][]*types.CellData)
		m.lock.Unlock()
	}
}

// GetAll efficiently returns all cells in the store.
func (s *inMemoryStore) GetAll(ctx context.Context) []types.Cell {
	s.lock.Lock()
	defer s.lock.Unlock()

	ret := []types.Cell{}
	for _, m := range s.data {
		m.lock.Lock()
		for _, cells := range m.cells {
			for _, cell := range cells {
				ret = append(ret, types.Cell{m.MetricInfo, *cell})
			}
		}
		m.lock.Unlock()
	}
	return ret
}
