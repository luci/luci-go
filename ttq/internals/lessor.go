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

package internals

import (
	"context"
	"time"

	"go.chromium.org/luci/ttq/internals/partition"
)

// WithLeaseClbk executes with active lease on the provided SortedPartitions.
// SortedPartitions may be empty slice, meaning there were existing active
// leases cumulatively covering the entire desired partition.
// Context deadline is set before the lease expires.
type WithLeaseClbk func(context.Context, partition.SortedPartitions)

// Lessor abstracts out different implementations aimed to prevent concurrent
// processing of the same range of Reminders.
//
// Lessors are used by some sweepdriver implementations.
type Lessor interface {
	// WithLease acquires the lease and executes WithLeaseClbk.
	// The obtained lease duration may be shorter than requested.
	// The obtained lease may be only for some parts of the desired Partition.
	WithLease(ctx context.Context, shard int, part *partition.Partition, dur time.Duration, clbk WithLeaseClbk) error
}
