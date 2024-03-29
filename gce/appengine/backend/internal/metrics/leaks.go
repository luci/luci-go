// Copyright 2023 The LUCI Authors.
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
	leakCount = metric.NewInt(
		"gce/leaks/count",
		"The number of instances leaked",
		nil,
		field.String("project"),
		field.String("zone"),
	)
)

// UpdateLeaks sets GCE instance leak metrics.
func UpdateLeaks(c context.Context, leak int64, project, zone string) {
	leakCount.Set(c, leak, project, zone)
}
