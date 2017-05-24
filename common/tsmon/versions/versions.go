// Copyright 2017 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

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

	"github.com/luci/luci-go/common/tsmon/field"
	"github.com/luci/luci-go/common/tsmon/metric"

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
