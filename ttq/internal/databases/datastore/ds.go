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

// package datastore implements TTQ Database backend on top of Cloud Datastore.
package datastore

import (
	"context"

	"go.chromium.org/luci/ttq/internal/databases"
	"go.chromium.org/luci/ttq/internal/reminder"
)

type DB struct {
}

var _ databases.Database = (*DB)(nil)

// Kind is used only for monitoring/logging purposes.
func (d *DB) Kind() string {
	return "datastore"
}

func (d *DB) SaveReminder(_ context.Context, _ *reminder.Reminder) error {
	panic("not implemented") // TODO: Implement
}

func (d *DB) DeleteReminder(_ context.Context, _ *reminder.Reminder) error {
	panic("not implemented") // TODO: Implement

}

func (d *DB) FetchRemindersMeta(ctx context.Context, low string, high string, limit int) ([]*reminder.Reminder, error) {
	panic("not implemented") // TODO: Implement
}

func (d *DB) FetchReminderPayloads(_ context.Context, _ []*reminder.Reminder) ([]*reminder.Reminder, error) {
	panic("not implemented") // TODO: Implement
}
