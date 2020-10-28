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

package metrics

import (
	"math"

	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
)

var (
	TagIndexInconsistentEntries = metric.NewNonCumulativeDistribution(
		"buildbucket/tag_index/inconsistent_entries",
		"Number of inconsistent entries encountered during build search",
		nil,
		// Bucketer for 0..1000 range, because we can't have more than 1000 entries
		// in a tag index.
		distribution.GeometricBucketer(math.Pow(10, 0.031), 100),
		field.String("tag"),
	)
	TagIndexSearchSkippedBuilds = metric.NewNonCumulativeDistribution(
		"buildbucket/tag_index/skipped_builds",
		"Number of builds we fetched, but skipped",
		nil,
		distribution.GeometricBucketer(math.Pow(10, 0.031), 100),
		field.String("tag"),
	)
)
