// Copyright 2020 The LUCI Authors.
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

// Package tq provides a task queue implementation on top of Cloud Tasks.
//
// It exposes a high-level API that operates with proto messages and hides
// gory details such as serialization, routing, authentication, etc.
//
// Enabling it on Appengine
//
// All default options should just work, except you still need to setup
// the sweeper cron and the sweeper queue.
//
// In cron.yaml:
//   - url: /internal/tasks/c/sweep
//     schedule: every 1 minutes
//
// In queue.yaml:
//   - name: tq-sweep
//     rate: 500/s
//
// Using it with Spanner
//
// You need to Create a table in your database:
//   CREATE TABLE TQReminders (
//     ID STRING(MAX) NOT NULL,
//     FreshUntil TIMESTAMP NOT NULL,
//     Payload BYTES(102400) NOT NULL,
//     Extra BYTES(1024) NOT NULL,
//   ) PRIMARY KEY (ID ASC);
package tq
