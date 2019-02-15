// Copyright 2018 The LUCI Authors.
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

package model

import (
	"context"
	"fmt"
	"sort"
	"time"

	"cloud.google.com/go/bigquery"
	"github.com/golang/protobuf/proto"
	"google.golang.org/appengine"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/bqlog"
	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/server/auth"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/metadata"
)

var eventsLog = bqlog.Log{
	QueueName:           "bqlog-events", // see queue.yaml
	DatasetID:           "cipd",         // see push_bq_schema.sh
	TableID:             "events",
	DumpEntriesToLogger: true,
	DryRun:              appengine.IsDevAppServer(),
}

// EventsEpoch is used to calculate timestamps used to order entities in the
// datastore.
//
// time.Time is stored with millisecond precision in the datastore. This is not
// enough to preserve the order of cipd.Event protos (that use nanoseconds for
// ordering events within a transactional group).
//
// So instead we store (event.When-EventsEpoch).Nanos() as int64. The original
// timestamp is still available in cipd.Event proto. This is only for datastore
// index.
var EventsEpoch = time.Date(2018, time.January, 1, 0, 0, 0, 0, time.UTC)

// NoInstance is used in place of empty Instance field in Event entity, so that
// we can query for all events that are not associated with instances and only
// them (these are package-level events).
const NoInstance = "NONE"

// Event in a global structured event log.
//
// It exists in both BigQuery (for adhoc queries) and in Datastore (for showing
// in web UI, e.g. for "recent tags" feature).
//
// ID as auto-generated. The parent entity is the corresponding Package entity
// (so that events can be committed transactionally with actual changes).
type Event struct {
	_kind  string                `gae:"$kind,Event"`
	_extra datastore.PropertyMap `gae:"-,extra"`

	ID      int64          `gae:"$id"`      // auto-generated
	Package *datastore.Key `gae:"$parent"`  // see PackageKey()
	Event   []byte         `gae:",noindex"` // serialized cipd.Event proto

	// Fields extracted from cipd.Event for indexing.
	Kind     api.EventKind
	Who      string
	When     int64  // nanoseconds since EventsEpoch
	Instance string // literal 'NONE' for package events, to be able to query them
	Ref      string
	Tag      string
}

// FromProto fills in the entity based on the proto message.
//
// Panics if the proto can't be serialized. This should never happen.
//
// Returns the entity itself for easier chaining.
func (e *Event) FromProto(c context.Context, p *api.Event) *Event {
	blob, err := proto.Marshal(p)
	if err != nil {
		panic(err)
	}
	e.Package = PackageKey(c, p.Package)
	e.Event = blob
	e.Kind = p.Kind
	e.Who = p.Who
	e.When = google.TimeFromProto(p.When).Sub(EventsEpoch).Nanoseconds()
	if p.Instance != "" {
		e.Instance = p.Instance
	} else {
		e.Instance = NoInstance
	}
	e.Ref = p.Ref
	e.Tag = p.Tag
	return e
}

// NewEventsQuery returns an unfiltered query for Event entities.
func NewEventsQuery() *datastore.Query {
	return datastore.NewQuery("Event")
}

// QueryEvents fetches and deserializes all events matching the query, sorting
// them by timestamp (most recent first).
//
// Start the query with NewEventsQuery.
func QueryEvents(c context.Context, q *datastore.Query) ([]*api.Event, error) {
	var ev []Event
	if err := datastore.GetAll(c, q.Order("-When"), &ev); err != nil {
		return nil, transient.Tag.Apply(err)
	}
	out := make([]*api.Event, len(ev))
	for i, e := range ev {
		out[i] = &api.Event{}
		if err := proto.Unmarshal(e.Event, out[i]); err != nil {
			return nil, errors.Annotate(err, "failed to unmarshal Event with ID %d", e.ID).Err()
		}
	}
	return out, nil
}

// Events collects events emitted in some transaction and then flushes them.
type Events struct {
	ev []*api.Event
}

// Emit adds an event to be flushed later in Flush.
//
// 'Who' and 'When' fields are populated in Flush using values from the context.
func (t *Events) Emit(e *api.Event) {
	t.ev = append(t.ev, e)
}

// Flush sends all pending events to the datastore/bigquery.
//
// Returned errors are tagged as transient. On success, clears the pending
// events queue.
func (t *Events) Flush(c context.Context) error {
	if len(t.ev) == 0 {
		return nil
	}

	when := clock.Now(c).UTC()
	who := string(auth.CurrentIdentity(c))

	entities := make([]*Event, len(t.ev))
	rows := make([]bigquery.ValueSaver, len(t.ev))
	for idx, e := range t.ev {
		// Make events in a batch ordered by time by abusing nanoseconds precision.
		e.When = google.NewTimestamp(when.Add(time.Duration(idx)))
		e.Who = who
		entities[idx] = (&Event{}).FromProto(c, e)
		rows[idx] = &bq.Row{Message: e}
	}

	err := parallel.FanOutIn(func(tasks chan<- func() error) {
		tasks <- func() error { return datastore.Put(c, entities) }
		tasks <- func() error { return eventsLog.Insert(c, rows...) }
	})
	if err != nil {
		return transient.Tag.Apply(err)
	}

	t.ev = t.ev[:0]
	return nil
}

// EmitEvent adds a single event to the event log.
//
// Prefer using Events to add multiple events at once.
func EmitEvent(c context.Context, e *api.Event) error {
	ev := Events{}
	ev.Emit(e)
	return ev.Flush(c)
}

// FlushEventsToBQ sends all buffered events to BigQuery.
//
// It is fine to call FlushEventsToBQ concurrently from multiple request
// handlers, if necessary (it will effectively parallelize the flush).
func FlushEventsToBQ(c context.Context) error {
	_, err := eventsLog.Flush(c)
	return err
}

////////////////////////////////////////////////////////////////////////////////

var sortedRoles []api.Role

func init() {
	sortedRoles = make([]api.Role, 0, len(api.Role_name)-1)
	for r := range api.Role_name {
		if role := api.Role(r); role != api.Role_ROLE_UNSPECIFIED {
			sortedRoles = append(sortedRoles, role)
		}
	}
	sort.Slice(sortedRoles, func(i, j int) bool {
		return sortedRoles[i] < sortedRoles[j]
	})
}

// EmitMetadataEvents compares two metadatum of a prefix and emits events for
// fields that have changed.
func EmitMetadataEvents(c context.Context, before, after *api.PrefixMetadata) error {
	if before.Prefix != after.Prefix {
		panic(fmt.Sprintf("comparing metadata for different prefixes: %q != %q", before.Prefix, after.Prefix))
	}
	prefix := after.Prefix

	prev := metadata.GetACLs(before)
	next := metadata.GetACLs(after)

	var granted []*api.PrefixMetadata_ACL
	var revoked []*api.PrefixMetadata_ACL

	for _, role := range sortedRoles {
		if added := stringSetDiff(next[role], prev[role]); len(added) != 0 {
			granted = append(granted, &api.PrefixMetadata_ACL{
				Role:       role,
				Principals: added,
			})
		}
		if removed := stringSetDiff(prev[role], next[role]); len(removed) != 0 {
			revoked = append(revoked, &api.PrefixMetadata_ACL{
				Role:       role,
				Principals: removed,
			})
		}
	}

	if len(granted) == 0 && len(revoked) == 0 {
		return nil
	}

	return EmitEvent(c, &api.Event{
		Kind:        api.EventKind_PREFIX_ACL_CHANGED,
		Package:     prefix,
		GrantedRole: granted,
		RevokedRole: revoked,
	})
}

// stringSetDiff returns sorted(a-b).
func stringSetDiff(a, b stringset.Set) []string {
	diff := a.Difference(b).ToSlice()
	sort.Strings(diff)
	return diff
}
