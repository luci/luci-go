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
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/google/uuid"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/ttq/internal"
)

type Spanner struct {
	client *spanner.Client
}

func New(client *spanner.Client) *Spanner {
	return &Spanner{client: client}
}

const happyPathDuration = time.Minute

const keySpaceBytes = 16 // Half of SHA2.

func (s *Spanner) Kind() string {
	return "spanner"
}

func (s *Spanner) SaveReminder(ctx context.Context, r *internal.Reminder) error {
	_, err := s.client.Apply(ctx, []*spanner.Mutation{
		spanner.Insert("TTQReminders",
			[]string{"ReminderId", "FreshUntil", "Payload"},
			[]interface{}{r.ID, r.FreshUntil, r.Payload})})
	if err != nil {
		return errors.Annotate(err, "failed to save a new reminder").Tag(transient.Tag).Err()
	}
	return nil
}

func (s *Spanner) DeleteReminder(ctx context.Context, r *internal.Reminder) error {
	_, err := s.client.Apply(ctx, []*spanner.Mutation{
		spanner.Delete("TTQReminders", spanner.Key{r.ID}),
	})
	if err != nil {
		return errors.Annotate(err, "failed to delete Reminder %s", r.ID).Tag(transient.Tag).Err()
	}
	return nil
}

func (s *Spanner) FetchRemainingReminders(ctx context.Context, batch []*internal.Reminder) ([]*internal.Reminder, error) {
	ks := spanner.KeySets()
	for _, r := range batch {
		ks = spanner.KeySets(ks, spanner.Key{r.ID})
	}

	t := s.client.ReadOnlyTransaction()
	defer t.Close()
	iter := t.ReadWithOptions(ctx, "TTQReminders", ks,
		[]string{"ReminderId", "FreshUntil", "Payload"},
		nil)
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

func (s *Spanner) FetchReminderKeys(ctx context.Context, low, high string) (res []*internal.Reminder, err error) {
	t := s.client.ReadOnlyTransaction()
	defer t.Close()
	iter := t.ReadWithOptions(ctx, "TTQReminders",
		spanner.KeyRange{Start: spanner.Key{low}, End: spanner.Key{high}},
		[]string{"ReminderId"},
		&spanner.ReadOptions{Limit: shardSweepItemsLimit})
	err = iter.Do(func(row *spanner.Row) error {
		r := &internal.Reminder{}
		if err := row.Columns(&r.ID, &r.FreshUntil, &r.Payload); err != nil {
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

func (s *Spanner) RunInTransaction(ctx context.Context, clbk func(context.Context) error) error {
	return ds.RunInTransaction(ctx, clbk, &ds.TransactionOptions{Attempts: 5})
}

// shardSweepItemsLimit is the number of keys that can be fetched & processed
// within the 1 minute deadline by a single shard worker.
//
// This is determined practically via a load test.
const shardSweepItemsLimit = 2048

func (s *Spanner) LoadLeases(ctx context.Context, shardIndex int) (leases []*internal.Lease, err error) {
	t := s.client.ReadOnlyTransaction()
	defer t.Close()
	iter := t.ReadWithOptions(ctx, "TTQLeases",
		spanner.KeyRange{Start: spanner.Key{shardIndex, ""}, End: spanner.Key{shardIndex + 1, ""}},
		[]string{"ShardId", "LeaseId", "Parts", "ExpiresAt"},
		nil)
	err = iter.Do(func(row *spanner.Row) error {
		l := &internal.Lease{}
		var shardVerify int
		if err := row.Columns(&shardVerify, &l.ID, &l.Parts, &l.ExpiresAt); err != nil {
			return err
		}
		if shardVerify != 1 {
			panic(fmt.Errorf("bad query or schema returned %d shard instead of %d", shardVerify, shardIndex))
		}
		leases = append(leases, l)
		return nil
	})
	if err != nil {
		err = errors.Annotate(err, "failed to fetch leases").Tag(transient.Tag).Err()
	}
	return
}

func (s *Spanner) SaveLease(ctx context.Context, l *internal.Lease, shardId int) error {
	u, err := uuid.NewRandom()
	if err != nil {
		return errors.Annotate(err, "failed to generate Lease ID").Tag(transient.Tag).Err()
	}
	l.ID = u.String()

	_, err = s.client.Apply(ctx, []*spanner.Mutation{
		spanner.Insert("TTQLeases",
			[]string{"ShardId", "LeaseId", "Parts", "ExpiresAt"},
			[]interface{}{shardId, l.ID, l.Parts, l.ExpiresAt})})
	if err != nil {
		return errors.Annotate(err, "failed to save a new lease").Tag(transient.Tag).Err()
	}
	return nil
}

func (s *Spanner) ReturnLease(ctx context.Context, l *internal.Lease, shardId int) error {
	_, err := s.client.Apply(ctx, []*spanner.Mutation{
		spanner.Delete("TTQLeases", spanner.Key{shardId, l.ID}),
	})
	if err != nil {
		return errors.Annotate(err, "failed to delete Lease %d %s", shardId, l.ID).Tag(transient.Tag).Err()
	}
	return nil
}
