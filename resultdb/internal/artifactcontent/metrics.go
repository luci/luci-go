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
	"net/http"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/server/router"
)

const firstBucket = 1024

const BreakEvenSize = 15 * 1000 // 15kb.

var (
	// artifactTransferDurations tracks the duration of artifacts transfers
	// between client and storage, e.g. RBE-CAS.
	artifactTransferDurations = metric.NewCumulativeDistribution(
		"resultdb/artifacts/durations",
		"Durations of artifact transfers between client and storage",
		&types.MetricMetadata{Units: types.Milliseconds},
		nil,
		field.Int("size_bucket"),
		field.String("op"), // "upload" or "download".
	)
	// artifactTransferStatus tracks the response statuses of artifact
	// transfers between client and storage, e.g. RBE-CAS.
	artifactTransferStatus = metric.NewCounter(
		"resultdb/artifacts/response_status",
		"Response statuses sent to clients requesting artifact transfer",
		&types.MetricMetadata{Units: "operations"},
		field.Int("http_status"),
		field.Int("size_bucket"),
		field.String("op"), // "upload" or "download".
	)

	ArtifactCounter = metric.NewCounter(
		"resultdb/artifact/count",
		"Count of artifact uploads to ResultDB",
		&types.MetricMetadata{Units: types.Bytes},
		field.Bool("less_than_15kb"),
		field.String("content_type"),
	)

	LogicalSizeCounter = metric.NewCounter(
		"resultdb/artifact/logical_size",
		"Cumulative logical size of artifact uploads to ResultDB",
		&types.MetricMetadata{Units: types.Bytes},
		field.Bool("less_than_15kb"), // Whether the compressed size is less than or equal to 15kb.
		field.String("content_type"),
	)
	CompressedSizeCounter = metric.NewCounter(
		"resultdb/artifact/compressed_size",
		"Cumulative compressed size of artifact uploads to ResultDB",
		&types.MetricMetadata{Units: types.Bytes},
		field.Bool("less_than_15kb"), // Whether the compressed size is less than or equal to 15kb.
		field.String("content_type"),
	)
)

// NewMetricsWriter creates a MetricsWriter to be used after the transfer is
// complete to time the operation and write the metrics.
// It also replaces the given context's writer with a wrapped writer that keeps
// track of the response status.
func NewMetricsWriter(c *router.Context) *MetricsWriter {
	if _, ok := c.Writer.(*statusTrackingWriter); !ok {
		c.Writer = &statusTrackingWriter{ResponseWriter: c.Writer}
	}
	return &MetricsWriter{
		statusTracker: c.Writer.(*statusTrackingWriter),
		startTime:     clock.Now(c.Request.Context()),
	}
}

// MetricsWriter can be used to record artifact transfer metrics, by creating
// one out of the router context before the transfer, and calling its .Download
// or .Upload methods afterwards.
type MetricsWriter struct {
	statusTracker *statusTrackingWriter
	startTime     time.Time
}

// Upload writes upload metrics.
func (mw *MetricsWriter) Upload(ctx context.Context, size int64) {
	mw.writeRequestMetrics(ctx, size, "upload")
}

// Download writes download metrics.
func (mw *MetricsWriter) Download(ctx context.Context, size int64) {
	mw.writeRequestMetrics(ctx, size, "download")
}

func (mw *MetricsWriter) writeRequestMetrics(ctx context.Context, size int64, op string) {
	duration := clock.Since(ctx, mw.startTime)
	sizeB := sizeBucket(size)
	artifactTransferDurations.Add(ctx, duration.Seconds()*1000, sizeB, op)
	artifactTransferStatus.Add(ctx, 1, mw.statusTracker.status, sizeB, op)
}

type statusTrackingWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusTrackingWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// sizeBucket returns the high bound of the bucket that the given size falls
// into.
// I.e. the smallest power of 4 that is greater or equal to the given size,
// starting at firstBucket.
// E.g. 1024, 4096, 16Ki, 64Ki, 256Ki, 1Mi, ...
func sizeBucket(s int64) (b int64) {
	b = firstBucket
	// Shift b left 2 bits at a time, until it's greater than or equal to s, or
	// until shifting it further would cause an overflow.
	for s > b && b<<2 > 0 {
		b <<= 2
	}
	return b
}
