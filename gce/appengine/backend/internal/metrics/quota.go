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

package metrics

import (
	"context"

	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
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
