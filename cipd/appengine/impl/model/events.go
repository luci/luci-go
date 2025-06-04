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
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"sort"
	"strings"
	"time"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/option"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/encoding/prototext"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/tq"

	api "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/cipd/appengine/impl/metadata"
	"go.chromium.org/luci/cipd/appengine/impl/model/tasks"
)

// textContentTypes is a list of text-like content types (in addition to
// "text/*").
//
// Metadata entries with such content types will have their values exported to
// the event log, see IsTextContentType.
var textContentTypes = stringset.NewFromSlice(
	"application/json",
	"application/jwt",
)

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
	Kind          api.EventKind
	Who           string
	When          int64  // nanoseconds since EventsEpoch
	Instance      string // literal 'NONE' for package events, to be able to query them
	Ref           string
	Tag           string
	MdKey         string
	MdFingerprint string
}

// FromProto fills in the entity based on the proto message.
//
// Panics if the proto can't be serialized. This should never happen.
//
// Returns the entity itself for easier chaining.
func (e *Event) FromProto(ctx context.Context, p *api.Event) *Event {
	blob, err := proto.Marshal(p)
	if err != nil {
		panic(err)
	}
	e.Package = PackageKey(ctx, p.Package)
	e.Event = blob
	e.Kind = p.Kind
	e.Who = p.Who
	e.When = p.When.AsTime().Sub(EventsEpoch).Nanoseconds()
	if p.Instance != "" {
		e.Instance = p.Instance
	} else {
		e.Instance = NoInstance
	}
	e.Ref = p.Ref
	e.Tag = p.Tag
	e.MdKey = p.MdKey
	e.MdFingerprint = p.MdFingerprint
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
func QueryEvents(ctx context.Context, q *datastore.Query) ([]*api.Event, error) {
	var ev []Event
	if err := datastore.GetAll(ctx, q.Order("-When"), &ev); err != nil {
		return nil, transient.Tag.Apply(err)
	}
	out := make([]*api.Event, len(ev))
	for i, e := range ev {
		out[i] = &api.Event{}
		if err := proto.Unmarshal(e.Event, out[i]); err != nil {
			return nil, errors.Fmt("failed to unmarshal Event with ID %d: %w", e.ID, err)
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
func (t *Events) Flush(ctx context.Context) error {
	if len(t.ev) == 0 {
		return nil
	}

	when := clock.Now(ctx).UTC()
	who := string(auth.CurrentIdentity(ctx))

	entities := make([]*Event, len(t.ev))
	for idx, e := range t.ev {
		// Make events in a batch ordered by time by abusing nanoseconds precision.
		e.When = timestamppb.New(when.Add(time.Duration(idx)))
		e.Who = who
		entities[idx] = (&Event{}).FromProto(ctx, e)
	}

	err := parallel.FanOutIn(func(tasks chan<- func() error) {
		tasks <- func() error { return datastore.Put(ctx, entities) }
		tasks <- func() error { return flushToSink(ctx, t.ev) }
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
func EmitEvent(ctx context.Context, e *api.Event) error {
	ev := Events{}
	ev.Emit(e)
	return ev.Flush(ctx)
}

// IsTextContentType checks the metadata content type against a list of
// known-good values.
func IsTextContentType(ct string) bool {
	ct, _, err := mime.ParseMediaType(ct)
	if err != nil {
		return false
	}
	if strings.HasPrefix(ct, "text/") {
		return true
	}
	return textContentTypes.Has(ct)
}

////////////////////////////////////////////////////////////////////////////////

var sinkCtxKey = "cipd model events sink"

type sink func(context.Context, []*api.Event) error

func installSink(ctx context.Context, s sink) context.Context {
	return context.WithValue(ctx, &sinkCtxKey, s)
}

func flushToSink(ctx context.Context, ev []*api.Event) error {
	if s, _ := ctx.Value(&sinkCtxKey).(sink); s != nil {
		return s(ctx, ev)
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////

// BigQueryEventLogger moves events from PubSub to BQ.
type BigQueryEventLogger struct {
	bq *bigquery.Client
}

// NewBigQueryEventLogger constructs a logger that writes to the given project.
func NewBigQueryEventLogger(ctx context.Context, projectID string) (*BigQueryEventLogger, error) {
	tr, err := auth.GetRPCTransport(ctx, auth.AsSelf, auth.WithScopes(auth.CloudOAuthScopes...))
	if err != nil {
		return nil, err
	}
	bq, err := bigquery.NewClient(ctx, projectID, option.WithHTTPClient(&http.Client{Transport: tr}))
	if err != nil {
		return nil, err
	}
	return &BigQueryEventLogger{bq: bq}, nil
}

// RegisterSink registers this event logger as events sink in the context.
//
// It will use the given dispatcher to route messages between processes.
func (l *BigQueryEventLogger) RegisterSink(ctx context.Context, disp *tq.Dispatcher, prod bool) context.Context {
	disp.RegisterTaskClass(tq.TaskClass{
		ID:        "log-events",
		Prototype: &tasks.LogEvents{},
		Kind:      tq.Transactional,
		Topic:     "bigquery-log",
		Custom: func(ctx context.Context, m proto.Message) (*tq.CustomPayload, error) {
			blob, err := (protojson.MarshalOptions{Indent: "\t"}).Marshal(m)
			if err != nil {
				return nil, err
			}
			return &tq.CustomPayload{Body: blob}, nil
		},
	})
	return installSink(ctx, func(ctx context.Context, ev []*api.Event) error {
		task := &tasks.LogEvents{Events: ev}
		if logging.IsLogging(ctx, logging.Debug) {
			blob, err := (prototext.MarshalOptions{Indent: "\t"}).Marshal(task)
			if err != nil {
				logging.Errorf(ctx, "Failed to marshal events to proto text: %s", err)
			} else {
				logging.Debugf(ctx, "Logged events:\n%s", blob)
			}
		}
		if !prod {
			return nil
		}
		return disp.AddTask(ctx, &tq.Task{Payload: task})
	})
}

// HandlePubSubPush handles incoming PubSub push messages produced by the sink.
func (l *BigQueryEventLogger) HandlePubSubPush(ctx context.Context, body io.Reader) error {
	blob, err := io.ReadAll(body)
	if err != nil {
		return errors.Fmt("failed to read the request body: %w", err)
	}
	// See https://cloud.google.com/pubsub/docs/push#receiving_messages
	var msg struct {
		Message struct {
			Attributes map[string]string `json:"attributes"`
			Data       []byte            `json:"data"`
			MessageID  string            `json:"messageId"`
		} `json:"message"`
	}
	if json.Unmarshal(blob, &msg); err != nil {
		return errors.Fmt("failed to unmarshal PubSub message: %w", err)
	}
	task := &tasks.LogEvents{}
	if err := protojson.Unmarshal(msg.Message.Data, task); err != nil {
		return errors.Fmt("failed to unmarshal the task: %w", err)
	}
	return l.insert(ctx, task.Events, msg.Message.MessageID)
}

// insert inserts rows into the BQ table.
func (l *BigQueryEventLogger) insert(ctx context.Context, ev []*api.Event, dedupID string) error {
	rows := make([]bigquery.ValueSaver, len(ev))
	for idx, e := range ev {
		rows[idx] = &bq.Row{
			Message:  e,
			InsertID: fmt.Sprintf("v1:%s:%d", dedupID, idx),
		}
	}
	return l.bq.Dataset("cipd").Table("events").Inserter().Put(ctx, rows)
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
func EmitMetadataEvents(ctx context.Context, before, after *api.PrefixMetadata) error {
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

	return EmitEvent(ctx, &api.Event{
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
