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
	"sync"

	"go.chromium.org/luci/common/tsmon/types"
)

var (
	registry = map[string]types.Metric{}
	lock     = sync.RWMutex{}
)

// Add adds a metric to the metric registry.
//
// Panics if a metric with such name is already defined.
func Add(m types.Metric) {
	name := m.Info().Name

	lock.Lock()
	defer lock.Unlock()

	if _, ok := registry[name]; ok {
		panic(fmt.Sprintf("A metric with the name %q was already registered", name))
	}
	registry[name] = m
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
