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

package metric

import (
	"math"

	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
)

var (
	SeqNumGenDuration = metric.NewCumulativeDistribution(
		"buildbucket/sequence_number/gen_duration",
		"Duration of a sequence number generation in ms",
		&types.MetricMetadata{Units: types.Milliseconds},
		// Bucketer for 1ms..5s range
		distribution.GeometricBucketer(math.Pow(10, 0.0374), 100),
		field.String("sequence"),
	)
)
