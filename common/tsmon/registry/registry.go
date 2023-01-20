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
	"sync"

	"go.chromium.org/luci/common/tsmon/monitor"
	"go.chromium.org/luci/common/tsmon/types"
)

var (
	registry          = map[metricRegistryKey]types.Metric{}
	lock              = sync.RWMutex{}
	metricNameRe      = regexp.MustCompile("^(/[a-zA-Z0-9_-]+)+$")
	metricFieldNameRe = regexp.MustCompile("^[A-Za-z_][A-Za-z0-9_]*$")
)

type metricRegistryKey struct {
	MetricName string
	TargetType types.TargetType
}

// Add adds a metric to the metric registry.
//
// Panics if
// - the metric name is invalid.
// - a metric with the same name and target type is defined already.
// - a field name is invalid.
func Add(m types.Metric) {
	key := metricRegistryKey{
		MetricName: m.Info().Name,
		TargetType: m.Info().TargetType,
	}
	fields := m.Info().Fields

	lock.Lock()
	defer lock.Unlock()

	switch _, exist := registry[key]; {
	case key.MetricName == "":
		panic(fmt.Errorf("empty metric name"))
	case !metricNameRe.MatchString(monitor.MetricNamePrefix + key.MetricName):
		panic(fmt.Errorf("invalid metric name %q: doesn't match %s", key.MetricName, metricNameRe))
	case exist:
		panic(fmt.Errorf("duplicate metric name: metric %q with target %q is registered already",
			key.MetricName, key.TargetType.Name))
	default:
		for _, f := range fields {
			if !metricFieldNameRe.MatchString(f.Name) {
				panic(fmt.Errorf("invalid field name %q: doesn't match %s",
					f.Name, metricFieldNameRe))
			}
		}
	}
	registry[key] = m
}

// Iter calls a callback for each registered metric.
//
// Metrics are visited in no particular order. The callback must not modify
// the registry.
func Iter(cb func(m types.Metric)) {
	lock.RLock()
	defer lock.RUnlock()
	for _, v := range registry {
		cb(v)
	}
}
