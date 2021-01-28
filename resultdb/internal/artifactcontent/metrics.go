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
	"net/http"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/server/router"
)

const firstBucket = 1024

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
		&types.MetricMetadata{Units: "artifacts"},
		field.Int("http_status"),
		field.Int("size_bucket"),
		field.String("op"), // "upload" or "download".
	)
)

// WriteUploadMetrics can be called after completing an upload to measure the
// time it took to complete, and record it in the appropriate metrics along
// with the size of the artifact and the status code sent to the client.
func WriteUploadMetrics(c *router.Context, size int64) {
	writeRequestMetrics(c, size, "upload")
}

// WriteDownloadMetrics can be called after serving an artifact to measure the
// time it took to send, and record it in the appropriate metrics along with
// the size of the artifact and the status code sent to the client.
func WriteDownloadMetrics(c *router.Context, r *contentRequest) {
	writeRequestMetrics(c, r.size.Int64, "download")
}

// WrapWriter wraps c.Writer to keep track of the status code it writes.
// It also tracks the start time for computing the duration of the request.
func WrapWriter(c *router.Context) {
	c.Writer = &statusTrackingWriter{
		ResponseWriter: c.Writer,
		startTime:      clock.Now(c.Context),
	}
}

type statusTrackingWriter struct {
	http.ResponseWriter
	startTime time.Time
	status    int
}

func (w *statusTrackingWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func writeRequestMetrics(c *router.Context, size int64, op string) {
	w := c.Writer.(*statusTrackingWriter)
	duration := clock.Now(c.Context).Sub(w.startTime)
	sizeB := sizeBucket(size)
	artifactTransferDurations.Add(c.Context, duration.Seconds()*1000, sizeB, op)
	artifactTransferStatus.Add(c.Context, 1, w.status, sizeB, op)
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
