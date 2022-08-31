// Copyright 2022 The LUCI Authors.
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

package bugs

import (
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
)

var (
	// BugsCreatedCounter is the metric that counts the number of bugs
	// created by LUCI Analysis, by project and bug-filing system.
	BugsCreatedCounter = metric.NewCounter("weetbix/bug_updater/bugs_created",
		"The number of bugs created by auto-bug filing, "+
			"by LUCI Project and bug-filing system.",
		&types.MetricMetadata{
			Units: "bugs",
		},
		// The LUCI project.
		field.String("project"),
		// The bug-filing system. Either "monorail" or "buganizer".
		field.String("bug_system"),
	)

	BugsUpdatedCounter = metric.NewCounter("weetbix/bug_updater/bugs_updated",
		"The number of bugs updated by auto-bug filing, "+
			"by LUCI Project and bug-filing system.",
		&types.MetricMetadata{
			Units: "bugs",
		},
		// The LUCI project.
		field.String("project"),
		// The bug-filing system. Either "monorail" or "buganizer".
		field.String("bug_system"))
)
