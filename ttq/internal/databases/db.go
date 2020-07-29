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

// package databases defines common database interface.
package databases

import (
	"context"

	"go.chromium.org/luci/ttq/internal/reminder"
)

// Database abstracts out specific storage implementation.
type Database interface {
	// Kind is used only for monitoring/logging purposes.
	Kind() string

	// SaveReminder persists reminder in a transaction context.
	SaveReminder(context.Context, *reminder.Reminder) error
	// DeleteReminder deletes reminder in a non-tranasction context.
	DeleteReminder(context.Context, *reminder.Reminder) error

	// FetchRemindersMeta fetches Reminders with Ids in [low..high) range.
	//
	// Payload of Reminders should not be fetched.
	// Both fresh & stale reminders should be fetched.
	// The reminders should be returned in order of ascending Id.
	//
	// In case of error, partial result of fetched Reminders so far should be
	// returned alongside the error. The caller will later call this method again
	// to fetch the remaining of Reminders in range of [<lastReturned.Id+1> .. high).
	FetchRemindersMeta(ctx context.Context, low, high string, limit int) ([]*reminder.Reminder, error)

	// FetchReminderPayloads fetches payloads of a batch of Reminders.
	//
	// The Reminder objects are re-used in the returned batch.
	// If any Reminder is no longer found, it is silently omitted in the returned
	// batch.
	// In case of any other error, partial result of fetched Reminders so far
	// should be returned alongside the error.
	FetchReminderPayloads(context.Context, []*reminder.Reminder) ([]*reminder.Reminder, error)
}
