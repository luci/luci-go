// Copyright 2025 The LUCI Authors.
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

package gs

import (
	"context"
	"time"

	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
)

var downloadRetries = metric.NewCumulativeDistribution(
	"cipd/gs/chunks/retries",
	"Distribution of number of attempts gs.readerImpl.ReadAt had to retry a chunk. An entry in bucket[0] indicates no retries (i.e. 1 attempt).",
	&types.MetricMetadata{},
	// Each bucket is just 1 value wide, and we have the same number of buckets as
	// our maximum number of retries.
	distribution.FixedWidthBucketer(1, retryPolicy.Retries),
	// True iff this operation ultimately succeeded after the given number of
	// retries. This distinguishes between retries == max (failure) and retries ==
	// max (success), as well as retries < max (failure for some other reason).
	field.Bool("success"),
)

var downloadChunkSpeed = metric.NewCumulativeDistribution(
	"cipd/gs/chunks/speed",
	"Distribution of speeds of gs.readerImpl.ReadAt attempts.",
	&types.MetricMetadata{Units: "MBy/s"},
	// This bucketer gets us a lower end which looks like:
	// lowerBounds[0] - -Inf
	// lowerBounds[1] - 0.19999999999997797
	// lowerBounds[2] - 0.4001999999999395
	// lowerBounds[3] - 0.6006001999999455
	// lowerBounds[4] - 0.8012008001998971
	// lowerBounds[5] - 1.0020020010000685
	// lowerBounds[6] - 1.2030040030010625
	// lowerBounds[7] - 1.4042070070040324
	// lowerBounds[8] - 1.605611214011038
	//
	// And an upper end which tops out around 98MBps (over 400 buckets).
	//
	// As of 25Q1 the fastest observed speeds were in the 70-80MBps range.
	distribution.GeometricBucketerWithScale(1.001, 400, 200),
	// True iff this chunk finished without errors (including timeout)
	field.Bool("success"),
	// The size of the chunk in MB, divided by 8 (to reduce cardinality).
	field.Int("chunk_size_div_8_MB"),
)

func reportDownloadChunkSpeed(ctx context.Context, size int, dt time.Duration, success bool) {
	speedBps := float64(size) / dt.Seconds()
	speedMBps := speedBps / 1e6
	downloadChunkSpeed.Add(ctx, speedMBps, success, (size/1e6)/8)
}
