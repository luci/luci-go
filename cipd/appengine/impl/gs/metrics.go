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

var downloadRetryCount = metric.NewCounter(
	"cipd/gs/chunks/dl/retry_count",
	"Counter for number of retries of readerImpl.ReadAt GSC calls. Increments 1/retry.",
	&types.MetricMetadata{},
	// The size of the chunk in MB, bucketed by 8MB (to reduce cardinality).
	field.Int("chunk_size"),
)

var downloadCallCount = metric.NewCounter(
	"cipd/gs/chunks/dl/call_count",
	"Counter for number of calls of readerImpl.ReadAt calls. One call may retry multiple times internally.",
	&types.MetricMetadata{},
	// True iff this chunk finished without errors (including timeout)
	field.Bool("success"),
	// The number of retries in this call.
	field.Int("retries"),
	// The size of the chunk in MB, bucketed by 8MB (to reduce cardinality).
	field.Int("chunk_size"),
)

var downloadChunkSpeed = metric.NewCumulativeDistribution(
	"cipd/gs/chunks/dl/speed",
	"Distribution of speeds of gs.readerImpl.ReadAt tries.",
	&types.MetricMetadata{Units: "MBy/s"},
	// This bucketer gets us a lower end of 0.05 MBps and scales to ~160MBps
	// over 400 buckets.
	//
	// As of 25Q1 the fastest observed speeds were in the 70-80MBps range.
	distribution.GeometricBucketerWithScale(1.0205, 400, 0.05),
	// True iff this chunk finished without errors (including timeout)
	field.Bool("success"),
	// The size of the chunk in MB, bucketed by 8MB (to reduce cardinality).
	field.Int("chunk_size"),
)

// Converts a size in bytes to a size in megabytes, rounded down to the nearest
// 8MB.
func toChunkSize(sizeBytes int) int {
	// e.g. 39284592 sizeBytes
	// / 1e6 => 39
	// / 8 => 4
	// * 8 => 32
	return ((sizeBytes / 1e6) / 8) * 8
}

func reportDownloadChunkSpeed(ctx context.Context, size int, dt time.Duration, success bool) {
	speedBps := float64(size) / dt.Seconds()
	speedMBps := speedBps / 1e6
	downloadChunkSpeed.Add(ctx, speedMBps, success, toChunkSize(size))
}
