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

package spanner

import (
	"context"

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/server/tq/internal/reminder"
)

type spanDB struct{}

func (spanDB) Kind() string {
	return "spanner"
}

func (spanDB) Defer(ctx context.Context, cb func(context.Context)) {
	span.Defer(ctx, cb)
}

func (spanDB) SaveReminder(_ context.Context, _ *reminder.Reminder) error {
	panic("not implemented") // TODO: Implement
}

func (spanDB) DeleteReminder(_ context.Context, _ *reminder.Reminder) error {
	panic("not implemented") // TODO: Implement
}

func (spanDB) FetchRemindersMeta(ctx context.Context, low string, high string, limit int) ([]*reminder.Reminder, error) {
	panic("not implemented") // TODO: Implement
}

func (spanDB) FetchReminderPayloads(_ context.Context, _ []*reminder.Reminder) ([]*reminder.Reminder, error) {
	panic("not implemented") // TODO: Implement
}
