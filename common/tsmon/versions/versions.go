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

// Package versions allows processes to report string-valued metrics with
// versions of various libraries they link with.
//
// The metric is named 'luci/components/version', and it has single field
// 'component' that defines the logical name of the component whose version is
// reported by the metric value.
//
// Having such metrics allows to easily detect processes that use stale code in
// production.
//
// Various go packages can register their versions during 'init' time, and all
// registered version will be flushed to monitoring whenever 'Report' is called.
package versions

import (
	"sync"

	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"

	"golang.org/x/net/context"
)

var registry struct {
	m sync.RWMutex
	r map[string]string
}

var (
	versionMetric = metric.NewString(
		"luci/components/version",
		"Versions of LUCI components linked into the process.",
		nil,
		field.String("component"),
	)
)

// Register tells the library to start reporting a version for given component.
//
// This should usually called during 'init' time.
//
// The component name will be used as a value of 'component' metric field, and
// the given version will become the actual reported metric value. It is a good
// idea to use a fully qualified go package name as 'component'.
func Register(component, version string) {
	registry.m.Lock()
	defer registry.m.Unlock()
	if registry.r == nil {
		registry.r = make(map[string]string, 1)
	}
	registry.r[component] = version
}

// Report populates 'luci/components/version' metric with versions of all
// registered components.
func Report(c context.Context) {
	registry.m.RLock()
	defer registry.m.RUnlock()
	for component := range registry.r {
		versionMetric.Set(c, registry.r[component], component)
	}
}
