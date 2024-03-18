// Copyright 2024 The LUCI Authors.
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

// Package metrics defines metrics used in Swarming.
package metrics

import (
	"math"

	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
)

var (
	TaskStatusChangePubsubLatency = metric.NewCumulativeDistribution(
		"swarming/tasks/state_change_pubsub_notify_latencies",
		"Latency (in ms) of PubSub notification when backend receives task_update",
		nil,
		// Custom bucketer with 2% resolution in the range of 100ms...100s. Used for
		// pubsub latency measurements.
		// Roughly speaking measurements range between 150ms and 300ms. However timeout
		// for pubsub notification is 10s.
		distribution.GeometricBucketer(math.Pow(10, 0.01), 300),
		field.String("pool"),          // e.g. 'skia'.
		field.String("status"),        // e.g. 'User canceled'
		field.Int("http_status_code")) // e.g. 404
)
