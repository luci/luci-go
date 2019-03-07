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

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
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

// autogen returns the field value to use when the given field value is
// automatically generated.
func autogen(s string) string {
	// Generally metric fields should have a known set of possible values.
	// autogen: prefix tells tsmon that these values are generated at random.
	return fmt.Sprintf("autogen:%s", s)
}

func init() {
	// Computing each VM's connected metric globally is costly. However,
	// only global metrics will be purged from memory after transmission.
	// To prevent metric values from piling up, register a no-op global
	// callback which declares connected as global.
	tsmon.RegisterGlobalCallback(func(context.Context) {}, connected)
}
