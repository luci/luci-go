// Copyright 2016 The LUCI Authors.
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
	"context"
	"strings"
	"time"

	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/common/tsmon/distribution"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/types"
)

// Metrics common to all tasks and devices.
var (
	presenceMetric = NewBool(
		"presence/up",
		"Set to True when the program is running, missing otherwise.",
		nil)
)

// Standard metrics for all requests to remote endpoints.  These
// metrics should be consistent across all tsmon implementations,
// Python or Go.
var (
	requestBytesMetric = NewCumulativeDistribution(
		"http/request_bytes",
		"Bytes sent per http request (body only).",
		&types.MetricMetadata{Units: types.Bytes},
		distribution.DefaultBucketer,
		field.String("name"),   // Usually the requested service name
		field.String("client")) // http client used, e.g. urlfetch

	responseBytesMetric = NewCumulativeDistribution(
		"http/response_bytes",
		"Bytes received per http request (content only).",
		&types.MetricMetadata{Units: types.Bytes},
		distribution.DefaultBucketer,
		field.String("name"),   // Usually the requested service name
		field.String("client")) // http client used, e.g. urlfetch

	requestDurationsMetric = NewCumulativeDistribution(
		"http/durations",
		"Time elapsed between sending a request and getting a response (including parsing) in milliseconds.",
		&types.MetricMetadata{Units: types.Milliseconds},
		distribution.DefaultBucketer,
		field.String("name"),   // Usually the requested service name
		field.String("client")) // http client used, e.g. urlfetch

	responseStatusMetric = NewCounter(
		"http/response_status",
		"Number of responses received by HTTP status code.",
		nil,
		field.Int("status"),    // HTTP status code
		field.String("name"),   // Usually the requested service name
		field.String("client")) // http client used, e.g. urlfetch
)

// Standard metrics for server-side request handlers.  These metrics
// should be consistent across all tsmon implementations, Python or
// Go.
var (
	serverRequestBytesMetric = NewCumulativeDistribution(
		"http/server_request_bytes",
		"Bytes received per http request (body only).",
		&types.MetricMetadata{Units: types.Bytes},
		distribution.DefaultBucketer,
		field.Int("status"),    // HTTP status code
		field.String("name"),   // URL template
		field.Bool("is_robot")) // If request is made by a bot

	serverResponseBytesMetric = NewCumulativeDistribution(
		"http/server_response_bytes",
		"Bytes sent per http request (body only).",
		&types.MetricMetadata{Units: types.Bytes},
		distribution.DefaultBucketer,
		field.Int("status"),    // HTTP status code
		field.String("name"),   // URL template
		field.Bool("is_robot")) // If request is made by a bot

	serverDurationsMetric = NewCumulativeDistribution(
		"http/server_durations",
		"Time elapsed between receiving a request and sending a response (including parsing) in milliseconds.",
		&types.MetricMetadata{Units: types.Milliseconds},
		distribution.DefaultBucketer,
		field.Int("status"),    // HTTP status code
		field.String("name"),   // URL template
		field.Bool("is_robot")) // If request is made by a bot

	serverResponseStatusMetric = NewCounter(
		"http/server_response_status",
		"Number of responses sent by HTTP status code.",
		nil,
		field.Int("status"),    // HTTP status code
		field.String("name"),   // URL template
		field.Bool("is_robot")) // If request is made by a bot
)

func init() {
	registerCallbacks(context.Background())
}

func registerCallbacks(ctx context.Context) {
	tsmon.RegisterCallbackIn(ctx, func(ctx context.Context) {
		presenceMetric.Set(ctx, true)
	})
}

// UpdateHTTPMetrics updates the metrics for a request to a remote server.
func UpdateHTTPMetrics(ctx context.Context, name string, client string,
	code int, duration time.Duration, requestBytes int64, responseBytes int64) {
	requestBytesMetric.Add(ctx, float64(requestBytes), name, client)
	responseBytesMetric.Add(ctx, float64(responseBytes), name, client)
	requestDurationsMetric.Add(ctx, float64(int64(duration)/int64(time.Millisecond)), name, client)
	responseStatusMetric.Add(ctx, 1, code, name, client)
}

// UpdateServerMetrics updates the metrics for a handled request.
func UpdateServerMetrics(ctx context.Context, name string, code int,
	duration time.Duration, requestBytes int64, responseBytes int64,
	userAgent string) {
	isRobot := (strings.Contains(userAgent, "GoogleBot") ||
		strings.Contains(userAgent, "GoogleSecurityScanner") ||
		userAgent == "B3M/prober")
	serverDurationsMetric.Add(ctx, float64(int64(duration)/int64(time.Millisecond)), code, name, isRobot)
	serverResponseStatusMetric.Add(ctx, 1, code, name, isRobot)
	serverRequestBytesMetric.Add(ctx, float64(requestBytes), code, name, isRobot)
	serverResponseBytesMetric.Add(ctx, float64(responseBytes), code, name, isRobot)
}
