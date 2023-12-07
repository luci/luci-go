// Copyright 2021 The LUCI Authors.
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
	"math"
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"

	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/server/tq/internal/lessor"
	"go.chromium.org/luci/server/tq/internal/partition"
)

// spanLessor implements lessor.Lessor on top of Cloud Spanner.
type spanLessor struct {
}

// WithLease acquires the lease and executes WithLeaseCB.
// The obtained lease duration may be shorter than requested.
// The obtained lease may be only for some parts of the desired Partition.
func (l *spanLessor) WithLease(ctx context.Context, sectionID string, part *partition.Partition, dur time.Duration, clbk lessor.WithLeaseCB) error {
	expiresAt := clock.Now(ctx).Add(dur)
	if d, ok := ctx.Deadline(); ok && expiresAt.After(d) {
		expiresAt = d
	}

	lease, err := l.acquire(ctx, sectionID, part, expiresAt)
	if err != nil {
		return err
	}
	defer lease.remove(ctx) // failure to remove is logged & ignored.

	lctx, cancel := clock.WithDeadline(ctx, lease.ExpiresAt)
	defer cancel()
	clbk(lctx, lease.parts)
	return nil
}

func (*spanLessor) acquire(ctx context.Context, sectionID string, desired *partition.Partition, expiresAt time.Time) (*lease, error) {
	var acquired *lease
	deletedExpired := 0

	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		deletedExpired = 0 // reset in case of retries.
		all, err := loadAll(ctx, sectionID)
		if err != nil {
			return errors.Annotate(err, "failed to read leases").Err()
		}
		active, expired := activeAndExpired(ctx, all)
		if len(expired) > 0 {
			// Deleting >= 1 lease every time a new one is created suffices to avoid
			// accumulating garbage above O(active leases).
			if len(expired) > 50 {
				expired = expired[:50]
			}
			remove(ctx, expired)
			deletedExpired = len(expired)
		}
		parts, err := availableForLease(desired, active)
		if err != nil {
			return errors.Annotate(err, "failed to decode available leases").Err()
		}
		acquired = save(ctx, sectionID, expiresAt, parts, maxLeaseID(all))
		return nil
	})
	if err != nil {
		return nil, errors.Annotate(err, "failed to transact a lease").Tag(transient.Tag).Err()
	}
	if deletedExpired > 0 {
		// If this is logged frequently, something is wrong either with the leasing
		// process or the lessees are holding to lease longer than they should.
		logging.Warningf(ctx, "deleted %d expired leases", deletedExpired)
	}
	return acquired, nil
}

type lease struct {
	SectionID       string
	LeaseID         int64
	SerializedParts []string
	ExpiresAt       time.Time

	// Set only when lease object is created in save().
	parts partition.SortedPartitions
}

func save(ctx context.Context, sectionID string, expiresAt time.Time, parts partition.SortedPartitions, max int64) *lease {
	if len(parts) == 0 {
		return &lease{
			ExpiresAt: expiresAt,
			parts:     parts,
		} // no need to save noop lease.
	}

	l := &lease{
		SectionID:       sectionID,
		SerializedParts: make([]string, len(parts)),
		ExpiresAt:       expiresAt.UTC(),
		parts:           parts,
	}
	for i, p := range parts {
		l.SerializedParts[i] = p.String()
	}

	// Strictly increase the leaseID until it reaches to math.MaxInt64 then
	// go back and increase from 1 again.
	var leaseID int64
	switch {
	case max < math.MaxInt64:
		leaseID = max + 1
	default:
		leaseID = 1
	}

	l.LeaseID = leaseID
	m := spanner.InsertMap("TQLeases", map[string]any{
		"SectionID":       l.SectionID,
		"LeaseID":         leaseID,
		"SerializedParts": l.SerializedParts,
		"ExpiresAt":       l.ExpiresAt,
	})
	span.BufferWrite(ctx, m)

	return l
}

func (l *lease) remove(ctx context.Context) {
	if l.LeaseID == 0 {
		return
	}

	_, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		remove(ctx, []*lease{l})
		return nil
	})
	if err != nil {
		// Log only. Once lease expires, it'll garbage-collected next time a new
		// lease is acquired for the same sectionID.
		logging.Warningf(ctx, "failed to remove lease %v", l)
	}
}

func query(ctx context.Context, sectionID string) ([]*lease, error) {
	st := spanner.NewStatement(`
		SELECT SectionID, LeaseID, SerializedParts, ExpiresAt
		FROM TQLeases
		WHERE SectionID = @sectionID
	`)
	st.Params = map[string]any{
		"sectionID": sectionID,
	}

	var all []*lease
	err := span.Query(ctx, st).Do(
		func(row *spanner.Row) error {
			l := &lease{}
			if err := row.Columns(&l.SectionID, &l.LeaseID, &l.SerializedParts, &l.ExpiresAt); err != nil {
				return err
			}
			all = append(all, l)
			return nil
		},
	)
	return all, err
}

func loadAll(ctx context.Context, sectionID string) ([]*lease, error) {
	all, err := query(ctx, sectionID)
	if err != nil {
		return nil, errors.Annotate(err, "failed to fetch leases").Tag(transient.Tag).Err()
	}
	return all, nil
}

func activeAndExpired(ctx context.Context, all []*lease) (active, expired []*lease) {
	// Partition active leases in the front and expired at the end of the slice.
	i, j := 0, len(all)
	now := clock.Now(ctx)
	for i < j {
		if all[i].ExpiresAt.After(now) {
			i++
			continue
		}
		j--
		all[i], all[j] = all[j], all[i]
	}
	return all[:i], all[i:]
}

func maxLeaseID(all []*lease) int64 {
	var max int64 = 0
	for _, l := range all {
		if l.LeaseID > max {
			max = l.LeaseID
		}
	}
	return max
}

func availableForLease(desired *partition.Partition, active []*lease) (partition.SortedPartitions, error) {
	builder := partition.NewSortedPartitionsBuilder(desired)
	// Exclude from desired all partitions under currently active leases.
	// TODO(tandrii): constrain number of partitions per lease to avoid excessive
	// runtime here.
	for _, l := range active {
		for _, s := range l.SerializedParts {
			p, err := partition.FromString(s)
			if err != nil {
				return nil, err
			}
			builder.Exclude(p)
			if builder.IsEmpty() {
				break
			}
		}
	}
	return builder.Result(), nil
}

func remove(ctx context.Context, ls []*lease) {
	ms := make([]*spanner.Mutation, 0, len(ls))
	for _, l := range ls {
		if l.LeaseID == 0 {
			continue
		}
		m := spanner.Delete("TQLeases", spanner.Key{l.SectionID, l.LeaseID})
		ms = append(ms, m)
	}
	span.BufferWrite(ctx, ms...)
}
