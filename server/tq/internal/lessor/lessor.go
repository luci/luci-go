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

// Package lessor defines common lessor interface.
package lessor

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/server/tq/internal/partition"
)

// WithLeaseCB executes with active lease on the provided SortedPartitions.
//
// SortedPartitions may be empty slice, meaning there were existing active
// leases cumulatively covering the entire desired partition.
// Context deadline is set before the lease expires.
type WithLeaseCB func(context.Context, partition.SortedPartitions)

// Lessor abstracts out different implementations aimed to prevent concurrent
// processing of the same range of Reminders.
//
// Lessors are used by the distributed sweep implementation.
type Lessor interface {
	// WithLease acquires the lease and executes WithLeaseCB.
	//
	// The obtained lease duration may be shorter than requested.
	// The obtained lease may be only for some parts of the desired Partition.
	//
	// The given `sectionID` identifies a transactionally updated object that
	// actually stores records about the currently leased sub-partitions of
	// `part`. Each such section is independent of another. In other words, if
	// some range of keys is covered by two different sections, it may be leased
	// to two different callers at the same time, there's no synchronization in
	// such case.
	WithLease(ctx context.Context, sectionID string, part *partition.Partition, dur time.Duration, cb WithLeaseCB) error
}

var lessors = map[string]func(ctx context.Context) (Lessor, error){}

// Register registers a lessor implementation.
//
// Preferably IDs should match the corresponding database.Database
// implementations, since by default if the TQ uses a database "<X>" it will use
// the lessor "<X>" as well. But there may be IDs that are no associated with
// any database implementation (e.g. a Redis-based lessor). Such lessors need
// an explicit opt-in to be used.
//
// Must be called during init time.
func Register(id string, factory func(ctx context.Context) (Lessor, error)) {
	if lessors[id] != nil {
		panic(fmt.Sprintf("lessor kind %q is already registered", id))
	}
	lessors[id] = factory
}

// Get returns a particular Lessor implementation given its ID.
func Get(ctx context.Context, id string) (Lessor, error) {
	if factory := lessors[id]; factory != nil {
		return factory(ctx)
	}
	return nil, errors.Fmt("no lessor kind %q is registered in the process", id)
}
