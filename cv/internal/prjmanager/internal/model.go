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

	"github.com/google/uuid"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
)

const (
	// ProjectDSKind should match that of actual prjmanager.Project entity
	// for maximal performance since they'd be located next to each other.
	ProjectDSKind = "Project"
	// eventDSKind is kind of `Event` entities.
	eventDSKind = "ProjectEvent"
)

// Event is an immutable entity in Datastore representing incoming events.
type Event struct {
	_kind string `gae:"$kind,ProjectEvent"`

	// Parent refers to the recipient LUCI Project.
	//
	// Make one using ProjectKey().
	Parent *datastore.Key `gae:"$parent"`

	// ID is UUID-generated one.
	// We want to ensure IDs are extremely unlikely to be re-used.
	ID string `gae:"$id"`

	// Payload stores actual event details.
	Payload *Payload

	// CreateTime is exact time of when this entity was created.
	//
	// It's not indexed to avoid hot areas in the index.
	CreateTime time.Time `gae:",noindex"`
}

// Send "sends" event to a Project's mailbox by storing in Datastore.
func Send(ctx context.Context, luciProject string, p *Payload) error {
	e := Event{
		ID:         uuid.New().String(),
		Payload:    p,
		CreateTime: clock.Now(ctx).UTC(),
		Parent:     datastore.MakeKey(ctx, ProjectDSKind, luciProject),
	}
	if err := datastore.Put(ctx, &e); err != nil {
		return errors.Annotate(err, "failed to write Event entity").Tag(transient.Tag).Err()
	}
	return nil
}

// Peek reads up to `limit` outstanding events in no particular order.
//
// Must not be called from a transaction because the events are immutable.
func Peek(ctx context.Context, luciProject string, limit int) (ret []*Event, err error) {
	if datastore.CurrentTransaction(ctx) != nil {
		panic("must be called outside a transaction")
	}
	q := datastore.NewQuery(eventDSKind).Limit(int32(limit)).Ancestor(
		datastore.MakeKey(ctx, ProjectDSKind, luciProject))
	if err = datastore.GetAll(ctx, q, &ret); err != nil {
		err = errors.Annotate(err, "failed to read Events of %q", luciProject).Tag(transient.Tag).Err()
	}
	return
}

// delete deletes events returned by Peek. Idempotent.
//
// Should be called in a transaction with the necessary mutation which saves
// result of processing of the event.
//
// May be called outside a transaction iff the event has definitely been
// processed already.
func Delete(ctx context.Context, events []*Event) error {
	const batchSize = 100
	var batch []*Event
	for {
		switch l := len(events); {
		case l == 0:
			return nil
		case l < batchSize:
			batch, events = events, nil
		default:
			batch, events = events[:batchSize], events[batchSize:]
		}
		if err := datastore.Delete(ctx, batch); err != nil {
			return errors.Annotate(err, "failed to delete %d Events", len(batch)).Tag(transient.Tag).Err()
		}
	}
}
