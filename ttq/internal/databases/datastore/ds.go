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
	"fmt"
	"strconv"
	"strings"
	"time"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/ttq/internal/reminder"
)

type DB struct {
}

// Kind is used only for monitoring/logging purposes.
func (d *DB) Kind() string {
	return "datastore"
}

const reminderKind = "ttq.Reminder"

type DSReminder struct {
	_kind string `gae:"$kind,ttq.Reminder"`

	Id      string `gae:"$id"` // "{Reminder.ID}_{Reminder.FreshUntil}".
	Payload []byte `gae:",noindex"`
}

func (d *DSReminder) fromReminder(r *reminder.Reminder) *DSReminder {
	d.Id = fmt.Sprintf("%s_%d", r.Id, r.FreshUntil.UnixNano())
	d.Payload = r.Payload
	return d
}

func (d DSReminder) toReminder(r *reminder.Reminder) *reminder.Reminder {
	parts := strings.Split(d.Id, "_")
	if len(parts) != 2 {
		panic(errors.Reason("malformed DSReminder ID %q", d.Id).Err())
	}
	ns, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		panic(errors.Reason("malformed DSReminder ID %q: %s", d.Id, err).Err())
	}
	if r == nil {
		r = &reminder.Reminder{}
	}
	r.Id = parts[0]
	r.FreshUntil = time.Unix(0, ns).UTC()
	r.Payload = d.Payload
	return r
}

// SaveReminder persists reminder in a transaction context.
func (d *DB) SaveReminder(ctx context.Context, r *reminder.Reminder) error {
	v := DSReminder{}
	if err := ds.Put(ctx, v.fromReminder(r)); err != nil {
		return errors.Annotate(err, "failed to persist to datastore").Tag(transient.Tag).Err()
	}
	return nil
}

// DeleteReminder deletes reminder in a non-tranasction context.
func (d *DB) DeleteReminder(ctx context.Context, r *reminder.Reminder) error {
	v := DSReminder{}
	if err := ds.Delete(ctx, v.fromReminder(r)); err != nil {
		return errors.Annotate(err, "failed to delete the Reminder %s", r.Id).Tag(transient.Tag).Err()
	}
	return nil
}

// FetchRemindersMeta fetches Reminders with Ids in [low..high) range.
//
// Payload of Reminders should not be fetched.
// Both fresh & stale reminders should be fetched.
// The reminders should be returned in order of ascending Id.
//
// In case of error, partial result of fetched Reminders so far should be
// returned alongside the error. The caller will later call this method again
// to fetch the remaining of Reminders in range of [<lastReturned.Id+1> .. high).
func (d *DB) FetchRemindersMeta(ctx context.Context, low string, high string, limit int) (items []*reminder.Reminder, err error) {
	q := ds.NewQuery(reminderKind).Order("__key__")
	q = q.Gte("__key__", ds.NewKey(ctx, reminderKind, low, 0, nil))
	q = q.Lt("__key__", ds.NewKey(ctx, reminderKind, high, 0, nil))
	q = q.Limit(int32(limit)).KeysOnly(true)
	err = ds.Run(ctx, q, func(k *ds.Key) {
		items = append(items, DSReminder{Id: k.StringID()}.toReminder(nil))
	})
	if err != nil && err != context.DeadlineExceeded {
		err = errors.Annotate(err, "failed to fetch Reminder keys").Tag(transient.Tag).Err()
	}
	return
}

// FetchReminderPayloads fetches payloads of a batch of Reminders.
//
// The Reminder objects are re-used in the returned batch.
// If any Reminder is no longer found, it is silently omitted in the returned
// batch.
// In case of any other error, partial result of fetched Reminders so far
// should be returned alongside the error.
func (d *DB) FetchReminderPayloads(ctx context.Context, batch []*reminder.Reminder) ([]*reminder.Reminder, error) {
	vs := make([]*DSReminder, len(batch))
	for i, r := range batch {
		vs[i] = (&DSReminder{}).fromReminder(r)
	}
	err := ds.Get(ctx, vs)
	merr, ok := err.(errors.MultiError)
	if err != nil && !ok {
		return nil, errors.Annotate(err, "failed to fetch Reminders").Tag(transient.Tag).Err()
	}

	res := make([]*reminder.Reminder, 0, len(batch))
	// Copy reminders with loaded payloads to result and move to the left errors
	// in MultiError which are not expected.
	ei := 0
	for i, v := range vs {
		switch {
		case merr == nil || merr[i] == nil:
			res = append(res, v.toReminder(batch[i]))
		case merr[i] != ds.ErrNoSuchEntity:
			merr[ei] = merr[i]
			ei++
		}
	}

	if ei == 0 {
		return res, nil
	}
	return res, merr[:ei]
}
