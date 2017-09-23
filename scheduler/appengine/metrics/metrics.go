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

// package metrics contains all task-agnostic monitoring metrics for scheduler
// service.
//
// Task-specific metrics are defined next to their respective task
// managers.
//
// All metrics of scheduler must start with "luci/scheduler/".
package metrics

import (
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
)

var (
	// All metrics must start with "luci/scheduler/".

	////////////////////////////////////////////////////////////////////////
	// Projects, their configs and validation.

	// TODO(tandrii): deprecate these once scheduler implements validation
	// endpoint which luci-config will use to pre-validate configs before
	// giving them to scheduler. See https://crbug.com/761488.

	Configs = metric.NewBool(
		"luci/scheduler/configs",
		"Whether project config is valid or invalid.",
		nil,
		field.String("project"),
	)

	ConfiguredJobs = metric.NewInt(
		"luci/scheduler/config/jobs",
		"Number of job or trigger definitions in a project.",
		nil,
		field.String("project"),
		field.String("status"), // one of "disabled", "invalid", "valid".
	)

