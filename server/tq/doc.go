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
// # Transactional tasks
//
// Tasks can be submitted as part of a database transaction. This is controlled
// by Kind field of TaskClass. A transactional task is guaranteed to be
// delivered (at least once) if and only if the database transaction was
// committed successfully. The are some prerequisites for using this mechanism.
//
// First, the sweeper must be running somewhere. The sweeper is responsible for
// discovering tasks that were successfully committed into the database, but
// were failed to be dispatched to Cloud Tasks (for example if the client that
// was submitting the task crashed right after committing the transaction). The
// sweeper can run either as a standalone service (the most convenient option
// for Kubernetes deployments) or as a cron job (the most convenient option for
// Appengine deployments).
//
// Second, the core server/tq library needs to "know" how to talk to the
// database that implements transactions. This is achieved by blank-importing
// a corresponding package.
//
// For Datastore:
//
//	import _ "go.chromium.org/luci/server/tq/txn/datastore"
//
// For Spanner:
//
//	import _ "go.chromium.org/luci/server/tq/txn/spanner"
//
// The exact location of the import doesn't matter as long as the package is
// present in the import tree of the binary. If your tests use transactional
// tasks, they'll need to import the corresponding packages as well.
//
// # Enabling the sweeper on Appengine
//
// In cron.yaml (required):
//   - url: /internal/tasks/c/sweep
//     schedule: every 1 minutes
//
// In queue.yaml (required when using the default distributed sweep mode):
//   - name: tq-sweep
//     rate: 500/s
//
// # Using the sweeper with Spanner
//
// You need to Create below tables in your database:
//
//	CREATE TABLE TQReminders (
//	  ID STRING(MAX) NOT NULL,
//	  FreshUntil TIMESTAMP NOT NULL,
//	  Payload BYTES(102400) NOT NULL,
//	) PRIMARY KEY (ID ASC);
//
//	CREATE TABLE TQLeases (
//	  SectionID STRING(MAX) NOT NULL,
//	  LeaseID INT64 NOT NULL,
//	  SerializedParts ARRAY<STRING(MAX)>,
//	  ExpiresAt TIMESTAMP NOT NULL,
//	) PRIMARY KEY (SectionID ASC, LeaseID ASC);
package tq
