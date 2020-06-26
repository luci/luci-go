// Copyright 2020 The LUCI Authors.
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

package internal

import (
	"math"

	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
)

var (
	// bucketer1msTo5min covers range of 1..300k.
	bucketer1msTo5min = distribution.GeometricBucketer(math.Pow(10, 0.055), 100)

	metricPostProcessedDurationsMS = metric.NewCumulativeDistribution(
		"ttq/postprocessed/durations",
		"How long it took to PostProcess a Reminder (ms)",
		&types.MetricMetadata{Units: types.Milliseconds},
		bucketer1msTo5min,
		field.String("status"), // OK | various failures
		field.String("when"),   // happy | sweep
		field.String("db"),
	)
	metricPostProcessedAttempts = metric.NewCounter(
		"ttq/postprocessed/attempts",
		"Number of PostProcess attempts",
		nil,
		field.String("status"), // OK | various failures
		field.String("when"),   // happy | sweep
		field.String("db"),
	)
	metricTasksCreated = metric.NewCounter(
		"ttq/tasks/created",
		"Number of user tasks actually created",
		nil,
		field.String("code"), // gRPC code
		field.String("when"), // happy | sweep
		field.String("db"),
	)
)
