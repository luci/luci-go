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

package backend

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
	connected = metric.NewBool(
		"gce/instance/connected",
		"Whether a GCE instance is connected to its Swarming server.",
		nil,
		field.String("hostname"),
		field.String("prefix"),
		field.String("project"),
		field.String("server"),
		field.String("zone"),
	)

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

type inst struct {
	connected bool
	prefix    string
	project   string
	server    string
	zone      string
}

var (
	// instances is a map of hostname to *insts to report connected values for.
	instances = make(map[string]*inst)

	// instLock is the lock under which instances should be accessed.
	instLock = &sync.Mutex{}
)

// setConnected sets the connected metric for the given VM.
func setConnected(c context.Context, is bool, vm *model.VM) {
	instLock.Lock()
	defer instLock.Unlock()
	instances[vm.Hostname] = &inst{
		connected: is,
		prefix:    vm.Prefix,
		project:   vm.Attributes.Project,
		server:    vm.Swarming,
		zone:      vm.Attributes.Zone,
	}
}

// autogen returns the field value to use when the given field value is
// automatically generated.
func autogen(s string) string {
	// Generally metric fields should have a known set of possible values.
	// autogen: prefix tells tsmon that these values are generated at random.
	return fmt.Sprintf("autogen:%s", s)
}

// reportConnected reports cached connected values to tsmon.
func reportConnected(c context.Context) {
	instLock.Lock()
	defer instLock.Unlock()
	for hostname, inst := range instances {
		connected.Set(c, inst.connected, autogen(hostname), inst.prefix, inst.project, inst.server, inst.zone)
		delete(instances, hostname)
	}
}

func init() {
	tsmon.RegisterGlobalCallback(reportConnected, connected)
}
