// Copyright 2018 The LUCI Authors.
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

// Package bq is a library for working with BigQuery.
//
// Limits
//
// Please see BigQuery docs:
// https://cloud.google.com/bigquery/quotas#streaminginserts for the most
// updated limits for streaming inserts. It is expected that the client is
// responsible for ensuring their usage will not exceed these limits through
// bq usage. A note on maximum rows per request: Put() batches rows per request,
// ensuring that no more than 10,000 rows are sent per request, and allowing for
// custom batch size. BigQuery recommends using 500 as a practical limit (so we
// use this as a default), and experimenting with your specific schema and data
// sizes to determine the batch size with the ideal balance of throughput and
// latency for your use case.
//
// Authentication
//
// Authentication for the Cloud projects happens
// during client creation: https://godoc.org/cloud.google.com/go#pkg-examples.
// What form this takes depends on the application.
//
// Monitoring
//
// You can use tsmon (https://godoc.org/go.chromium.org/luci/common/tsmon) to
// track upload latency and errors.
//
// If Uploader.UploadsMetricName field is not zero, Uploader will
// create a counter metric to track successes and failures.
package bq
