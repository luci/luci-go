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

package ttqds

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/ttq"
	"go.chromium.org/luci/ttq/internal"
)

type DS struct{}

func New() ttq.Database {
	return &DS{}
}

const happyPathDuration = time.Minute

const keySpaceBytes = 16 // Half of SHA2.

const dsReminderKind = "ttq.Reminder"

type DSReminder struct {
	_kind   string `gae:"$kind,ttq.Reminder"`
	ID      string `gae:"$id"`
	Payload []byte `gae:",noindex"`
}

func (d *DSReminder) fromReminder(r *internal.Reminder) *DSReminder {
	d.ID = fmt.Sprintf("%s_%d", r.ID, r.FreshUntil.UnixNano())
	d.Payload = r.Payload
	return d
}

func (d DSReminder) toReminder(r *internal.Reminder) *internal.Reminder {
	parts := strings.Split(d.ID, "_")
	if len(parts) != 2 {
		panic(errors.Reason("malformed DSReminder ID %q", d.ID).Err())
	}
	ns, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		panic(errors.Reason("malformed DSReminder ID %q: %s", d.ID, err).Err())
	}
	if r == nil {
		r = &internal.Reminder{}
	}
	r.ID = parts[0]
	r.FreshUntil = time.Unix(0, ns)
	r.Payload = d.Payload
	return r
}

func (d *DS) Kind() string {
	return "datastore"
}

func (d *DS) SaveReminder(ctx context.Context, r *internal.Reminder) error {
	v := DSReminder{}
	v.fromReminder(r)
	if err := ds.Put(ctx, &v); err != nil {
		return errors.Annotate(err, "failed to persist to datastore").Tag(transient.Tag).Err()
	}
	return nil
}

func (d *DS) DeleteReminder(ctx context.Context, r *internal.Reminder) error {
	v := DSReminder{}
	v.fromReminder(r)
	if err := ds.Delete(ctx, &v); err != nil {
		return errors.Annotate(err, "failed to delete the Reminder %s", r.ID).Err()
	}
	return nil
}

func (d *DS) FetchReminderPayloads(ctx context.Context, batch []*internal.Reminder) ([]*internal.Reminder, error) {
	vs := make([]*DSReminder, len(batch))
	for i, r := range batch {
		vs[i] = (&DSReminder{}).fromReminder(r)
	}
	err := ds.Get(ctx, vs)
	merr, ok := err.(errors.MultiError)
	if err != nil && !ok {
		return nil, errors.Annotate(err, "failed to fetch reminders").Err()
	}

	// Move to the left reminders in a batch that were read,
	// and errors in MultiError which aren't expected.
	bi := 0
	ei := 0
	for i, v := range vs {
		switch {
		case merr == nil || merr[i] == nil:
			v.toReminder(batch[bi])
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

func (d *DS) FetchReminderMeta(ctx context.Context, low, high string, limit int) ([]*internal.Reminder, error) {
	q := ds.NewQuery("ttq.Reminder").Order("__key__")
	q = q.Gte("__key__", ds.KeyForObj(ctx, &DSReminder{ID: low}))
	q = q.Lt("__key__", ds.KeyForObj(ctx, &DSReminder{ID: high}))
	q = q.Limit(int32(limit)).KeysOnly(true)
	logging.Debugf(ctx, "query: %s..%s limit %s", low, high, limit)
	var items []*internal.Reminder
	err := ds.Run(ctx, q, func(k *ds.Key) {
		items = append(items, DSReminder{ID: k.StringID()}.toReminder(nil))
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

func (d *DS) RunInTransaction(ctx context.Context, clbk func(context.Context) error) error {
	err := ds.RunInTransaction(ctx, clbk, &ds.TransactionOptions{Attempts: 5})
	if err != nil {
		return errors.Annotate(err, "failed transaction").Tag(transient.Tag).Err()
	}
	return nil
}

type DSLeasesRoot struct {
	_kind   string `gae:"$kind,ttq.LeasesRoot"`
	ShardID string `gae:"$id"` // string to avoid having 0 integer keys.
}

type DSLease struct {
	_kind string `gae:"$kind,ttq.Lease"`

	LeaseID   string    `gae:"$id"`
	Parent    *ds.Key   `gae:"$parent"` // DSLeasesRoot.
	Parts     []string  `gae:",noindex"`
	ExpiresAt time.Time `gae:",noindex"`
}

func leasesRootEntity(shardId int) *DSLeasesRoot {
	return &DSLeasesRoot{ShardID: strconv.Itoa(shardId)}
}

func leasesRootKey(ctx context.Context, shardId int) *ds.Key {
	return ds.KeyForObj(ctx, leasesRootEntity(shardId))
}

func (d *DS) LoadLeases(ctx context.Context, shardIndex int) ([]*internal.Lease, error) {
	var vs []*DSLease
	q := ds.NewQuery("ttq.Lease").Ancestor(leasesRootKey(ctx, shardIndex))
	if err := ds.GetAll(ctx, q, &vs); err != nil {
		return nil, errors.Annotate(err, "failed to fetch leases").Tag(transient.Tag).Err()
	}
	leases := make([]*internal.Lease, len(vs))
	for i, v := range vs {
		leases[i] = &internal.Lease{
			Parts:     v.Parts,
			ExpiresAt: v.ExpiresAt,
			Impl:      v,
		}
	}
	return leases, nil
}

func (d *DS) SaveLease(ctx context.Context, l *internal.Lease, shardId int) error {
	u, err := uuid.NewRandom()
	if err != nil {
		return errors.Annotate(err, "failed to generate Lease ID").Tag(transient.Tag).Err()
	}
	v := &DSLease{
		LeaseID:   u.String(),
		Parent:    leasesRootKey(ctx, shardId),
		Parts:     l.Parts,
		ExpiresAt: l.ExpiresAt,
	}
	if err = ds.Put(ctx, leasesRootEntity(shardId), v); err != nil {
		return errors.Annotate(err, "failed to save a new lease").Tag(transient.Tag).Err()
	}
	l.Impl = v
	return nil
}

func (d *DS) ReturnLease(ctx context.Context, l *internal.Lease) error {
	// Transaction isn't neccessary -- it's fine if leasing decision of ongoing
	// transaction is made as if this lease is still active.
	v, ok := l.Impl.(*DSLease)
	if !ok {
		return errors.Reason("lease %s wasn't loaded or saved by the Datastore", l).Err()
	}
	if err := ds.Delete(ctx, v); err != nil {
		return errors.Annotate(err, "failed to delete %s", l).Err()
	}
	return nil
}

func (d *DS) DeleteExpiredLeases(ctx context.Context, leases []*internal.Lease) error {
	vs := make([]*DSLease, len(leases))
	for i, l := range leases {
		v, ok := l.Impl.(*DSLease)
		if !ok {
			return errors.Reason("lease %s wasn't loaded or saved by the Datastore", l).Err()
		}
		vs[i] = v
	}
	if err := ds.Delete(ctx, vs); err != nil {
		return errors.Annotate(err, "failed to delete %d expired leases", len(leases)).Err()
	}
	return nil
}
