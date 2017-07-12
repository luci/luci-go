// Copyright 2015 The LUCI Authors.
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

// Package output contains interfaces and implementations for Butler Outputs,
// which are responsible for delivering Butler protobufs to LogDog collection
// endpoints.
//
// Output instance implementations must be goroutine-safe. The Butler may elect
// to output multiple messages at the same time.
//
// The package current provides the following implementations:
//   - pubsub: Write logs to Google Cloud Pub/Sub.
//   - log: (Debug/testing) data is dumped to the installed Logger instance.
package output
