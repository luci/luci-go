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
	// casTransferDurations tracks the duration of artifacts uploads/downloads
	// to/from CAS.
	casTransferDurations = metric.NewCumulativeDistribution(
		"resultdb/cas_artifacts/durations",
		"Durations of artifact uploads/downloads to/from CAS",
		nil,
		nil,
		field.Int("bucket"),
		field.String("operation"), // "upload" or "download".
	)
	// casTransferStatus tracks the response statuses of artifact
	// uploads/downloads to/from CAS.
	casTransferStatus = metric.NewCounter(
		"resultdb/cas_artifacts/response_status",
		"Response statuses of artifact uploads/downloads to/from CAS",
		nil,
		field.Int("status"),
		field.Int("bucket"),
		field.String("operation"), // "upload" or "download".
	)
)

// TrackCASDownload collects metrics about a download from CAS.
func TrackCASDownload(ctx context.Context, startTime time.Time, status int, size int64) {
	collectCASTransferMetrics(ctx, startTime, status, size, "download")
}

// TrackCASUpload collects metrics about a upload from CAS.
func TrackCASUpload(ctx context.Context, startTime time.Time, status int, size int64) {
	collectCASTransferMetrics(ctx, startTime, status, size, "upload")
}

func collectCASTransferMetrics(ctx context.Context, startTime time.Time, status int, size int64, operation string) {
	endTime := time.Now().Sub(startTime)
	bucket := sizeBucket(size)
	casTransferDurations.Add(ctx, endTime.Seconds()*1000, bucket, operation)
	casTransferStatus.Add(ctx, 1, status, bucket, operation)
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
