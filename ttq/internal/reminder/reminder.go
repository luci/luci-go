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

// Package reminder holds Reminder to avoid circular dependency of internal and
// testing.
package reminder

import "time"

// Reminder reminds to enqueue a task.
//
// It is persisted transactionally with some other user logic to the database.
// Later, a task is actually scheduled and a reminder can be deleted
// non-transactionally.
type Reminder struct {
	// Id identifies a reminder.
	//
	// Id values are always in hex-encoded and are well distributed in keyspace.
	Id string
	// FreshUntil is the expected time by which the happy path should complete.
	//
	// If the sweeper encounters a Reminder before this time, the sweeper ignores
	// it to allow the happy path to complete.
	//
	// Truncated to FreshUntilPrecision.
	FreshUntil time.Time
	// Payload is a proto-serialized taskspb.Task.
	Payload []byte
}

// FreshUntilPrecision is precision of Reminder.FreshUntil, to which it is
// always truncated.
const FreshUntilPrecision = time.Millisecond
