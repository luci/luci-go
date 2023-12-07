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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq/internal/reminder"
)

// tableName is the name of the table that user must create in their Spanner
// database prior to using this package.
//
// CREATE TABLE TQReminders (
//
//	ID STRING(MAX) NOT NULL,
//	FreshUntil TIMESTAMP NOT NULL,
//	Payload BYTES(102400) NOT NULL,
//
// ) PRIMARY KEY (ID ASC);
//
// If you ever need to change this, change also user-visible server/tq doc.
const tableName = "TQReminders"

type spanDB struct{}

func (spanDB) Kind() string {
	return "spanner"
}

func (spanDB) Defer(ctx context.Context, cb func(context.Context)) {
	span.Defer(ctx, cb)
}

func (spanDB) SaveReminder(ctx context.Context, r *reminder.Reminder) error {
	span.BufferWrite(ctx, spanner.Insert(
		tableName,
		[]string{"ID", "FreshUntil", "Payload"},
		[]any{r.ID, r.FreshUntil, r.RawPayload}))
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
		[]string{"ID", "FreshUntil"},
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

func (spanDB) FetchReminderRawPayloads(ctx context.Context, batch []*reminder.Reminder) ([]*reminder.Reminder, error) {
	ks := make([]spanner.KeySet, len(batch))
	for i, r := range batch {
		ks[i] = spanner.Key{r.ID}
	}

	iter := span.Read(span.Single(ctx), tableName, spanner.KeySets(ks...),
		[]string{"ID", "FreshUntil", "Payload"})
	i := 0
	err := iter.Do(func(row *spanner.Row) error {
		if err := row.Columns(&batch[i].ID, &batch[i].FreshUntil, &batch[i].RawPayload); err != nil {
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
