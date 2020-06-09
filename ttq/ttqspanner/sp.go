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

package ttqspanner

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/google/uuid"

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
	txn, ok := getTxn(ctx)
	if !ok {
		return errors.Reason("must run in a transaction").Err()
	}
	err := txn.BufferWrite([]*spanner.Mutation{
		spanner.Insert("TTQReminders",
			[]string{"ReminderId", "FreshUntil", "Payload"},
			[]interface{}{r.ID, r.FreshUntil, r.Payload})})
	if err != nil {
		return errors.Annotate(err, "failed to buffer a new reminder").Tag(transient.Tag).Err()
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

func (s *Spanner) FetchReminderPayloads(ctx context.Context, batch []*internal.Reminder) ([]*internal.Reminder, error) {
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

func (s *Spanner) FetchReminderMeta(ctx context.Context, low, high string, limit int) (res []*internal.Reminder, err error) {
	t := s.client.ReadOnlyTransaction()
	defer t.Close()
	iter := t.ReadWithOptions(ctx, "TTQReminders",
		spanner.KeyRange{Start: spanner.Key{low}, End: spanner.Key{high}},
		[]string{"ReminderId", "FreshUntil"},
		&spanner.ReadOptions{Limit: limit})
	err = iter.Do(func(row *spanner.Row) error {
		r := &internal.Reminder{}
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

var txnContextKey = "context key for a *spanner.ReadWriteTransaction"

func withTxn(ctx context.Context, txn *spanner.ReadWriteTransaction) context.Context {
	return context.WithValue(ctx, &txnContextKey, txn)
}

func getTxn(ctx context.Context) (*spanner.ReadWriteTransaction, bool) {
	txn, ok := ctx.Value(&txnContextKey).(*spanner.ReadWriteTransaction)
	return txn, ok
}

func (s *Spanner) RunInTransaction(ctx context.Context, clbk func(context.Context) error) error {
	if _, ok := getTxn(ctx); ok {
		return errors.Reason("ReadWriteTransaction already in progress").Err()
	}
	_, err := s.client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		return clbk(withTxn(ctx, txn))
	})
	if err != nil {
		return errors.Annotate(err, "failed transaction").Tag(transient.Tag).Err()
	}
	return nil
}

type leaseMeta struct {
	ShardId int64
	LeaseId string
}

type readWithOptions interface {
	ReadWithOptions(context.Context, string, spanner.KeySet, []string, *spanner.ReadOptions) *spanner.RowIterator
}

func (s *Spanner) LoadLeases(ctx context.Context, shardId int) (leases []*internal.Lease, err error) {
	var reader readWithOptions
	if txn, ok := getTxn(ctx); ok {
		reader = txn
	} else {
		ro := s.client.ReadOnlyTransaction()
		defer ro.Close()
		reader = ro
	}
	iter := reader.ReadWithOptions(ctx, "TTQLeases",
		spanner.KeyRange{Start: spanner.Key{shardId, ""}, End: spanner.Key{shardId + 1, ""}},
		[]string{"ShardId", "LeaseId", "Parts", "ExpiresAt"},
		nil)
	err = iter.Do(func(row *spanner.Row) error {
		l := &internal.Lease{}
		meta := &leaseMeta{}
		l.Impl = meta
		if err := row.Columns(&meta.ShardId, &meta.LeaseId, &l.Parts, &l.ExpiresAt); err != nil {
			return err
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
	txn, ok := getTxn(ctx)
	if !ok {
		return errors.Reason("must run in a transaction").Err()
	}
	u, err := uuid.NewRandom()
	if err != nil {
		return errors.Annotate(err, "failed to generate Lease ID").Tag(transient.Tag).Err()
	}
	meta := &leaseMeta{ShardId: int64(shardId), LeaseId: u.String()}
	err = txn.BufferWrite([]*spanner.Mutation{
		spanner.Insert("TTQLeases",
			[]string{"ShardId", "LeaseId", "Parts", "ExpiresAt"},
			[]interface{}{shardId, meta.LeaseId, l.Parts, l.ExpiresAt})})
	if err != nil {
		return errors.Annotate(err, "failed to buffer new lease write").Tag(transient.Tag).Err()
	}
	l.Impl = meta
	return nil
}

func (s *Spanner) ReturnLease(ctx context.Context, l *internal.Lease) error {
	if _, ok := getTxn(ctx); ok {
		// The whole point of using UUID IDs is to avoid running in a transaction.
		return errors.Reason("must not run in a transaction").Err()
	}
	meta, ok := l.Impl.(*leaseMeta)
	if !ok {
		return errors.Reason("lease %s wasn't loaded or saved by the Spanner", l).Err()
	}
	_, err := s.client.Apply(ctx, []*spanner.Mutation{
		spanner.Delete("TTQLeases", spanner.Key{meta.ShardId, meta.LeaseId}),
	})
	if err != nil {
		return errors.Annotate(err, "failed to delete %s", l).Tag(transient.Tag).Err()
	}
	return nil
}
func (s *Spanner) DeleteExpiredLeases(ctx context.Context, leases []*internal.Lease) error {
	txn, ok := getTxn(ctx)
	if !ok {
		return errors.Reason("must run in a transaction").Err()
	}
	muts := make([]*spanner.Mutation, len(leases))
	for i, l := range leases {
		meta, ok := l.Impl.(*leaseMeta)
		if !ok {
			return errors.Reason("lease %s wasn't loaded or saved by the Spanner", l).Err()
		}
		muts[i] = spanner.Delete("TTQLeases", spanner.Key{meta.ShardId, meta.LeaseId})
	}
	if err := txn.BufferWrite(muts); err != nil {
		return errors.Annotate(err, "failed to buffer deletion of %d leases", len(leases)).Err()
	}
	return nil
}
