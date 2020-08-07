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

	"cloud.google.com/go/spanner"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/server/tq/internal/reminder"
)

const tableName = "TQReminders"

const initSQL = `
CREATE TABLE TQReminders (
	ReminderId STRING(MAX) NOT NULL,
	FreshUntil TIMESTAMP NOT NULL,
	Payload BYTES(102400) NOT NULL,
) PRIMARY KEY (ReminderId ASC);
`

type spanDB struct{}

func (spanDB) Kind() string {
	return "spanner"
}

func (spanDB) Defer(ctx context.Context, cb func(context.Context)) {
	if i := mathrand.Int(ctx); i&1 == 1 { // ~50% chance.
		span.Defer(ctx, cb)
	}
}

func (spanDB) SaveReminder(ctx context.Context, r *reminder.Reminder) error {
	span.BufferWrite(ctx, spanner.Insert(
		tableName,
		[]string{"ReminderID", "FreshUntil", "Payload"},
		[]interface{}{r.ID, r.FreshUntil, r.Payload}))
	return nil
}

func (spanDB) DeleteReminder(ctx context.Context, r *reminder.Reminder) error {
	_, err := span.Apply(ctx, []*spanner.Mutation{
		spanner.Delete(tableName, spanner.Key{r.ID}),
	}, spanner.ApplyAtLeastOnce())
	if err != nil {
		return errors.Annotate(err, "failed to delete the Reminder %s", r.ID).Tag(transient.Tag).Err()
	}
	return nil
}

func (spanDB) FetchRemindersMeta(ctx context.Context, low string, high string, limit int) (res []*reminder.Reminder, err error) {
	iter := span.ReadWithOptions(span.Single(ctx),
		tableName,
		spanner.KeyRange{Start: spanner.Key{low}, End: spanner.Key{high}},
		[]string{"ReminderId", "FreshUntil"},
		&spanner.ReadOptions{Limit: limit})
	err = iter.Do(func(row *spanner.Row) error {
		r := &reminder.Reminder{}
		if err := row.Columns(&r.ID, &r.FreshUntil); err != nil {
			return err
		}
		res = append(res, r)
		return nil
	})
	if err != nil && err != context.DeadlineExceeded {
		err = errors.Annotate(err, "failed to fetch Reminder keys").Tag(transient.Tag).Err()
	}
	return
}

func (spanDB) FetchReminderPayloads(ctx context.Context, batch []*reminder.Reminder) ([]*reminder.Reminder, error) {
	ks := spanner.KeySets()
	for _, r := range batch {
		ks = spanner.KeySets(ks, spanner.Key{r.ID})
	}

	iter := span.Read(span.Single(ctx), tableName, ks,
		[]string{"ReminderId", "FreshUntil", "Payload"})
	i := 0
	err := iter.Do(func(row *spanner.Row) error {
		if err := row.Columns(&batch[i].ID, &batch[i].FreshUntil, &batch[i].Payload); err != nil {
			return err
		}
		i++
		return nil
	})
	if err != nil {
		return batch[:i], errors.Annotate(err, "failed to fetch Reminders").Tag(transient.Tag).Err()
	}
	return batch[:i], nil
}
