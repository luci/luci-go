// Copyright 2019 The LUCI Authors.
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

// Package metrics includes tsmon metric support for internal use by backend.
package metrics

import (
	"context"
	"fmt"
	"sync"

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"

	"go.chromium.org/luci/gce/appengine/model"
)

var (
	quotaLimit = metric.NewFloat(
		"gce/quota/limit",
		"The GCE quota limit for a particular metric.",
		nil,
		field.String("metric"),
		field.String("project"),
		field.String("region"),
	)

	quotaRemaining = metric.NewFloat(
		"gce/quota/remaining",
		"The remaining GCE quota for a particular metric.",
		nil,
		field.String("metric"),
		field.String("project"),
		field.String("region"),
	)

	quotaUsage = metric.NewFloat(
		"gce/quota/usage",
		"The GCE quota being used for a particular metric.",
		nil,
		field.String("metric"),
		field.String("project"),
		field.String("region"),
	)
)

// UpdateQuota sets GCE quota metrics.
func UpdateQuota(c context.Context, limit, usage float64, metric, project, region string) {
	quotaLimit.Set(c, limit, metric, project, region)
	quotaRemaining.Set(c, limit-usage, metric, project, region)
	quotaUsage.Set(c, usage, metric, project, region)
}

var (
	instanceConnected = metric.NewBool(
		"gce/instance/connected",
		"Whether a GCE instance is connected to its Swarming server.",
		nil,
		field.String("hostname"),
		field.String("prefix"),
		field.String("project"),
		field.String("server"),
		field.String("zone"),
	)

	// cache is a map of hostname to *connInst to report instanceConnected for.
	cache = make(map[string]*connInst)

	// cacheLock is the lock under which the cache should be accessed.
	cacheLock = &sync.Mutex{}
)

type connInst struct {
	connected bool
	prefix    string
	project   string
	server    string
	zone      string
}

// UpdateInstance sets GCE instance metrics.
func UpdateInstance(c context.Context, connected bool, vm *model.VM) {
	inst := &connInst{
		connected: connected,
		prefix:    vm.Prefix,
		project:   vm.Attributes.GetProject(),
		server:    vm.Swarming,
		zone:      vm.Attributes.GetZone(),
	}
	cacheLock.Lock()
	defer cacheLock.Unlock()
	cache[vm.Hostname] = inst
}

// autogen returns the field value to use when the given field value is
// automatically generated.
func autogen(s string) string {
	// Generally metric fields should have a known set of possible values.
	// autogen: prefix tells tsmon that these values are generated at random.
	return fmt.Sprintf("autogen:%s", s)
}

// setCached reports cached values to tsmon.
func setCached(c context.Context) {
	cacheLock.Lock()
	defer cacheLock.Unlock()
	for hostname, inst := range cache {
		instanceConnected.Set(c, inst.connected, autogen(hostname), inst.prefix, inst.project, inst.server, inst.zone)
		delete(cache, hostname)
	}
}

func init() {
	tsmon.RegisterGlobalCallback(setCached, instanceConnected)
}
