// Copyright 2017 The LUCI Authors.
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

// Package registry holds a map of all metrics registered by the process.
//
// This map is global and it is populated during init() time, when individual
// metrics are defined.
package registry

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/types"
)

var (
	Global            = NewRegistry()
	metricNameRe      = regexp.MustCompile("^(/[a-zA-Z0-9_-]+)+$")
	metricFieldNameRe = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_]*$")
)

type metricRegistryKey struct {
	MetricName string
	TargetType types.TargetType
}

type Registry struct {
	metrics map[metricRegistryKey]types.Metric
	lock    sync.RWMutex
}

// NewRegistry creates a new registry.
func NewRegistry() *Registry {
	return &Registry{
		metrics: map[metricRegistryKey]types.Metric{},
	}
}

// Add adds a metric to the metric registry.
//
// Panics if
// - the metric name is invalid.
// - a metric with the same name and target type is defined already.
// - a field name is invalid.
func (registry *Registry) Add(m types.Metric) {
	key := metricRegistryKey{
		MetricName: m.Info().Name,
		TargetType: m.Info().TargetType,
	}
	fields := m.Info().Fields

	registry.lock.Lock()
	defer registry.lock.Unlock()

	if err := ValidateMetricName(key.MetricName); err != nil {
		panic(err)
	}

	switch _, exist := registry.metrics[key]; {
	case exist:
		panic(fmt.Errorf("duplicate metric name: metric %q with target %q is registered already",
			key.MetricName, key.TargetType.Name))
	default:
		for _, f := range fields {
			if err := ValidateMetricFieldName(f.Name); err != nil {
				panic(err)
			}
		}
	}

	registry.metrics[key] = m
}

// Iter calls a callback for each registered metric.
//
// Metrics are visited in no particular order. The callback must not modify
// the registry.
func (registry *Registry) Iter(cb func(m types.Metric)) {
	registry.lock.RLock()
	defer registry.lock.RUnlock()
	for _, v := range registry.metrics {
		cb(v)
	}
}

func validateMetricName(name string) error {
	if metricNameRe.MatchString(name) {
		return nil
	}
	return fmt.Errorf("invalid metric name %q: doesn't match %s", name, metricNameRe)
}

// ValidateMetricName validates the provided metric name.
// * if the provided name starts with "/", validate it as is;
// * otherwise prepend monitor.MetricNamePrefix to it then validate.
func ValidateMetricName(name string) error {
	if name == "" {
		return fmt.Errorf("empty metric name")
	}

	if strings.HasPrefix(name, "/") {
		return validateMetricName(name)
	}
	return validateMetricName(monitor.MetricNamePrefix + name)
}

// ValidateMetricFieldName validates a metric field name.
func ValidateMetricFieldName(fName string) error {
	if metricFieldNameRe.MatchString(fName) {
		return nil
	}
	return fmt.Errorf("invalid field name %q: doesn't match %s",
		fName, metricFieldNameRe)
}
