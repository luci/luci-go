// Copyright 2015 The LUCI Authors.
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

package types

import (
	"go.chromium.org/luci/common/tsmon/distribution"
)

// Metric is the low-level interface provided by all metrics.
// Concrete types are defined in the "metrics" package.
type Metric interface {
	Info() MetricInfo
	Metadata() MetricMetadata
}

// DistributionMetric is the low-level interface provided by all distribution
// metrics.  It has a Bucketer which is responsible for assigning buckets to
// samples.  Concrete types are defined in the "metrics" package.
type DistributionMetric interface {
	Metric

	Bucketer() *distribution.Bucketer
}
