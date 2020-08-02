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

// Package spanner implements TTQ Database backend on top of Cloud Spanner.
//
// Uses "go.chromium.org/luci/server/span" library, requires all transactions to
// be initiated via its ReadWriteTransaction function.
package spanner

import (
	"context"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/ttq/internals/databases"
	"go.chromium.org/luci/ttq/internals/reminder"
)

type DB struct{}

var _ databases.Database = DB{}

func (DB) Kind() string {
	return "spanner"
}

func (DB) Defer(ctx context.Context, cb func(context.Context)) {
	span.Defer(ctx, cb)
}

func (DB) SaveReminder(_ context.Context, _ *reminder.Reminder) error {
	panic("not implemented") // TODO: Implement
}

func (DB) DeleteReminder(_ context.Context, _ *reminder.Reminder) error {
	panic("not implemented") // TODO: Implement
}

func (DB) FetchRemindersMeta(ctx context.Context, low string, high string, limit int) ([]*reminder.Reminder, error) {
	panic("not implemented") // TODO: Implement
}

func (DB) FetchReminderPayloads(_ context.Context, _ []*reminder.Reminder) ([]*reminder.Reminder, error) {
	panic("not implemented") // TODO: Implement
}
