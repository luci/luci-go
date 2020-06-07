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

package datastore

import (
	"context"
	"strconv"
	"time"

	"github.com/google/uuid"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/ttq/internal"
)

type Datastore struct{}

func New() *Datastore {
	return &Datastore{}
}

const happyPathDuration = time.Minute

const keySpaceBytes = 16 // Half of SHA2.

func (d *Datastore) Kind() string {
	return "datastore"
}

func (d *Datastore) SaveReminder(ctx context.Context, r *internal.Reminder) error {
	if err := ds.Put(ctx, r); err != nil {
		return errors.Annotate(err, "failed to persist to datastore").Tag(transient.Tag).Err()
	}
	return nil
}

func (d *Datastore) DeleteReminder(ctx context.Context, r *internal.Reminder) error {
	if err := ds.Delete(ctx, r); err != nil {
		return errors.Annotate(err, "failed to delete the Reminder %s", r.ID).Err()
	}
	return nil
}

func (s *Datastore) FetchRemainingReminders(ctx context.Context, batch []*internal.Reminder) ([]*internal.Reminder, error) {
	err := ds.Get(ctx, batch)
	if err == nil {
		return batch, nil
	}
	merr, ok := err.(errors.MultiError)
	if !ok {
		return nil, errors.Annotate(err, "failed to fetch reminders").Err()
	}

	// Move to the left reminders in a batch that were read,
	// and errors in MultiError which aren't expected.
	bi := 0
	ei := 0
	for i, _ := range batch {
		switch {
		case merr[i] == nil:
			batch[bi] = batch[i]
			bi++
		case !ds.IsErrNoSuchEntity(merr[i]):
			merr[ei] = merr[i]
			ei++
		}
	}

	if ei == 0 {
		return batch[:bi], nil
	}
	return batch[:bi], merr[:ei]
}

func (d *Datastore) FetchReminderKeys(ctx context.Context, low, high string) ([]*internal.Reminder, error) {
	q := ds.NewQuery("ttq.Task").Order("__key__")
	q = q.Gte("__key__", ds.KeyForObj(ctx, &internal.Reminder{ID: low}))
	q = q.Lt("__key__", ds.KeyForObj(ctx, &internal.Reminder{ID: high}))
	q = q.Limit(shardSweepItemsLimit).KeysOnly(true)
	logging.Debugf(ctx, "query: %s..%s limit %s", low, high, shardSweepItemsLimit)
	var items []*internal.Reminder
	err := ds.Run(ctx, q, func(k *ds.Key) {
		items = append(items, &internal.Reminder{ID: k.StringID()})
	})
	switch {
	case err == nil:
		return items, nil
	case err == context.DeadlineExceeded:
		return items, err
	default:
		return items, errors.Annotate(err, "failed to fetch Reminder keys").Tag(transient.Tag).Err()
	}
}

func (d *Datastore) RunInTransaction(ctx context.Context, clbk func(context.Context) error) error {
	err := ds.RunInTransaction(ctx, clbk, &ds.TransactionOptions{Attempts: 5})
	if err != nil {
		return errors.Annotate(err, "failed transaction").Tag(transient.Tag).Err()
	}
	return nil
}

const defaultShardsNumber = 16

// shardSweepItemsLimit is the number of keys that can be fetched & processed
// within the 1 minute deadline by a single shard worker.
//
// This is determined practically via a load test.
const shardSweepItemsLimit = 2048
const tooLargePartitionSplitInto = 4
const maxShardFanout = 128
const batchSize = 50
const sweepFrequency = 1 * time.Minute

type LeasesRoot struct {
	_kind string `gae:"$kind,ttq.LeasesRoot"`
	ID    string `gae:"$id"`
}

func leasesRootEntity(shardId int) *LeasesRoot {
	return &LeasesRoot{ID: strconv.Itoa(shardId)}
}

func leasesRootKey(ctx context.Context, shardId int) *ds.Key {
	return ds.KeyForObj(ctx, leasesRootEntity(shardId))
}

func (d *Datastore) LoadLeases(ctx context.Context, shardIndex int) (leases []*internal.Lease, err error) {
	q := ds.NewQuery("ttq.Lease").Ancestor(leasesRootKey(ctx, shardIndex))
	if err = ds.GetAll(ctx, q, &leases); err != nil {
		err = errors.Annotate(err, "failed to fetch leases").Tag(transient.Tag).Err()
	}
	return
}

func (d *Datastore) SaveLease(ctx context.Context, l *internal.Lease, shardId int) error {
	u, err := uuid.NewRandom()
	if err != nil {
		return errors.Annotate(err, "failed to generate Lease ID").Tag(transient.Tag).Err()
	}
	l.ID = u.String()
	l.Parent = leasesRootKey(ctx, shardId)
	if err = ds.Put(ctx, leasesRootEntity(shardId), l); err != nil {
		return errors.Annotate(err, "failed to save a new lease").Tag(transient.Tag).Err()
	}
	return nil
}

func (d *Datastore) ReturnLease(ctx context.Context, l *internal.Lease, _ int) error {
	// Transaction isn't neccessary -- it's fine if leasing decision of ongoing
	// transaction is made as if this lease is still active.
	if err := ds.Delete(ctx, l); err != nil {
		return errors.Annotate(err, "failed to delete the Lease entity").Err()
	}
	return nil
}
