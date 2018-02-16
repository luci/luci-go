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

// Package gkelogger is a simple logger that outputs all log entries as single-
// line JSON objects to stdout.
// The JSON format is defined by the Google Cloud Logging plugin for fluentd,
// which is mainly used by The Google Kubernetes Engine + Stackdriver stack.
// See https://github.com/GoogleCloudPlatform/fluent-plugin-google-cloud
//
// Default usage (logging to stderr):
//
//   import (
//     "go.chromium.org/luci/common/logging"
//     "go.chromium.org/luci/common/logging/gkelogger"
//   )
//
//   ...
//
//   ctx := context.Background()
//   ctx = gkelogger.Use(ctx)
//   logging.Infof(ctx, "Hello %s", "world")
package gkelogger
