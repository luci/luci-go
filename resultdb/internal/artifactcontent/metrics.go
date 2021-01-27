// Copyright 2021 The LUCI Authors.
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

package artifactcontent

import (
	"context"
	"time"

	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
)

const firstBucket = 1024

var (
	// artifactTransferDurations tracks the duration of artifacts uploads/downloads
	// to/from storage, e.g. RBE-CAS.
	artifactTransferDurations = metric.NewCumulativeDistribution(
		"resultdb/artifacts/durations",
		"Durations of artifact uploads/downloads to/from storage",
		nil,
		nil,
		field.Int("size_bucket"),
		field.String("operation"), // "upload" or "download".
	)
	// artifactTransferStatus tracks the response statuses of artifact
	// uploads/downloads to/from storage, e.g. RBE-CAS.
	artifactTransferStatus = metric.NewCounter(
		"resultdb/artifacts/response_status",
		"Response statuses of artifact uploads/downloads to/from storage",
		nil,
		field.Int("status"),
		field.Int("size_bucket"),
		field.String("operation"), // "upload" or "download".
	)
)

// TrackArtifactDownload collects metrics about a download from CAS.
func TrackArtifactDownload(ctx context.Context, startTime time.Time, status int, size int64) {
	collectArtifactTransferMetrics(ctx, startTime, status, size, "download")
}

// TrackArtifactUpload collects metrics about a upload from CAS.
func TrackArtifactUpload(ctx context.Context, startTime time.Time, status int, size int64) {
	collectArtifactTransferMetrics(ctx, startTime, status, size, "upload")
}

func collectArtifactTransferMetrics(ctx context.Context, startTime time.Time, status int, size int64, operation string) {
	duration := time.Now().Sub(startTime)
	sizeB := sizeBucket(size)
	artifactTransferDurations.Add(ctx, duration.Seconds()*1000, sizeB, operation)
	artifactTransferStatus.Add(ctx, 1, status, sizeB, operation)
}

// sizeBucket returns the high bound of the bucket that the given size falls
// into.
// I.e. the smallest power of 4 that is greater or equal to the given size,
// starting at firstBucket.
// E.g. 1024, 4096, 16Ki, 64Ki, 256Ki, 1Mi, ...
func sizeBucket(s int64) (b int64) {
	b = firstBucket
	// Shift b left 2 bits at a time, until it's greater than s, or until
	// shifting it further would cause an overflow.
	for s > b && b<<2 > 0 {
		b <<= 2
	}
	return b
}
