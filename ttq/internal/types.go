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

package internal

import (
	"context"
	"time"

	ds "go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
)

// Reminder is a record of transactionally enqueued task.
type Reminder struct {
	_kind string `gae:"$kind,ttq.Task"`
	// ID is a hash of the task name and it is designed to be well
	// distributed in keyspace to avoid hotspots.
	ID string `gae:"$id"`
	// FreshUntil is the expected time by which happy path should complete.
	//
	// The sweeper will ignore a DSReminder while it is fresh.
	FreshUntil time.Time `gae:",noindex"`

	// Payload is a serialized taskspb.Task.
	Payload []byte `gae:",noindex"`
}

type Lease struct {
	_kind string `gae:"$kind,ttq.Lease"`
	ID    string `gae:"$id"`
	// TODO(tandrii): del parent.
	Parent    *ds.Key   `gae:"$parent"`
	Parts     []string  `gae:",noindex"` // each el is a Partition.String()
	ExpiresAt time.Time `gae:",noindex"`
}

func LeaseMaxPossible(ctx context.Context, leases []*Lease, desired *Partition, expiresAt time.Time) (
	available *Lease, sp SortedPartitions, expiredLeases []*Lease) {
	now := clock.Now(ctx)
	builder := NewSortedPartitionsBuilder(desired)
	for _, l := range leases {
		if l.ExpiresAt.Before(now) {
			expiredLeases = append(expiredLeases, l)
			continue
		}
		for _, part := range l.Parts {
			if builder.IsEmpty() {
				break
			}
			p, err := PartitionFromString(part)
			if err != nil {
				// Should not happen in prod, unless the format has changed.
				// Log but proceed as if the lease didn't exist, it's still correct if 2
				// workers process overlapping range.
				logging.Errorf(ctx, "failed to parse a lease: %s", err)
				continue
			}
			builder.Exclude(&p)
		}
	}
	if !builder.IsEmpty() {
		sp = builder.Result()
		available = &Lease{
			ExpiresAt: expiresAt.Round(time.Millisecond).UTC(),
			Parts:     make([]string, len(sp)),
		}
		for i, p := range sp {
			available.Parts[i] = p.String()
		}
	}
	return
}
