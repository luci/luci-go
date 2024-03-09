// Copyright 2023 The LUCI Authors.
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
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/gae/service/datastore"

	apipb "go.chromium.org/luci/swarming/proto/api_v2"
)

// BotEventType identifies various known bot events.
type BotEventType string

// Bot events that happen outside the scope of a task.
const (
	BotEventConnected BotEventType = "bot_connected"
	BotEventError     BotEventType = "bot_error"
	BotEventIdle      BotEventType = "bot_idle"
	BotEventLog       BotEventType = "bot_log"
	BotEventMissing   BotEventType = "bot_missing"
	BotEventPolling   BotEventType = "bot_polling"
	BotEventRebooting BotEventType = "bot_rebooting"
	BotEventShutdown  BotEventType = "bot_shutdown"
	BotEventTerminate BotEventType = "bot_terminate"
)

// Bot events representing polling outcomes.
const (
	BotEventRestart BotEventType = "request_restart"
	BotEventSleep   BotEventType = "request_sleep"
	BotEventTask    BotEventType = "request_task"
	BotEventUpdate  BotEventType = "request_update"
)

// Bot events related to running tasks.
const (
	BotEventTaskCompleted BotEventType = "task_completed"
	BotEventTaskError     BotEventType = "task_error"
	BotEventTaskKilled    BotEventType = "task_killed"
	BotEventTaskUpdate    BotEventType = "task_update"
)

// BotStateEnum is used to represent state of the bot in datastore.
//
// See comment for BotCommon.Composite. Individual values should not leak in any
// public APIs, it is an implementation detail.
type BotStateEnum int64

// Possible categories of bot state.
const (
	BotStateBusy             BotStateEnum = 1 << 0
	BotStateIdle             BotStateEnum = 1 << 1
	BotStateQuarantined      BotStateEnum = 1 << 2
	BotStateHealthy          BotStateEnum = 1 << 3
	BotStateUnused1          BotStateEnum = 1 << 4
	BotStateUnused2          BotStateEnum = 1 << 5
	BotStateDead             BotStateEnum = 1 << 6
	BotStateAlive            BotStateEnum = 1 << 7
	BotStateInMaintenance    BotStateEnum = 1 << 8
	BotStateNotInMaintenance BotStateEnum = 1 << 9
)

// StateFilter represents a filter over the possible bot states.
//
// Each field is a filter on one aspect of the bot state with possible values
// being TRUE (meaning "yes"), FALSE (meaning "no") and NULL (meaning "don't
// care").
type StateFilter struct {
	// Quarantined filters bots based on whether they are quarantined.
	Quarantined apipb.NullableBool
	// InMaintenance filters bots based on whether they are in maintenance mode.
	InMaintenance apipb.NullableBool
	// IsDead filters bots based on whether they are connected or not.
	IsDead apipb.NullableBool
	// IsBusy filters bots based on whether they execute any task or not.
	IsBusy apipb.NullableBool
}

// BotRoot is an entity group root of entities representing a single bot.
//
// Presence of this entity indicates there are BotEvent entities for this bot.
//
// TODO(vadimsh): This entity is unnecessary complication. Old entities cleanup
// should happen via Cloud Datastore TTL feature, then this entity is not
// needed.
type BotRoot struct {
	// Extra are entity properties that didn't match any declared ones below.
	//
	// Should normally be empty.
	Extra datastore.PropertyMap `gae:"-,extra"`

	// Key is derived based on the bot ID, see BotRootKey.
	Key *datastore.Key `gae:"$key"`

	// LegacyCurrent is no longer used.
	LegacyCurrent LegacyProperty `gae:"current"`
}

// BotRootKey is a root key of an entity group with info about a bot.
func BotRootKey(ctx context.Context, botID string) *datastore.Key {
	return datastore.NewKey(ctx, "BotRoot", botID, 0, nil)
}

// BotCommon contains properties that are common to both BotInfo and BotEvent.
//
// It is not meant to be stored in the datastore on its own, only as an embedded
// struct inside BotInfo or BotEvent.
type BotCommon struct {
	// State is a free form JSON dict with the bot state as reported by the bot.
	//
	// Swarming itself mostly ignores this information, but it is exposed via API
	// and UI, allowing bots to report extended information about themselves to
	// Swarming clients.
	State []byte `gae:"state,noindex"`

	// ExternalIP is the bot's IP address as seen by the server.
	ExternalIP string `gae:"external_ip,noindex"`

	// AuthenticatedAs is the bot's credentials as seen by the server.
	AuthenticatedAs identity.Identity `gae:"authenticated_as,noindex"`

	// Version of the bot code the bot is running.
	Version string `gae:"version,noindex"`

	// Quarantined means the bot is unhealthy and should not receive tasks.
	//
	// It is set when either:
	// - dimensions['quarantined'] or state['quarantined'] is set by the bot.
	// - API requests from the bot appear to be malformed.
	Quarantined bool `gae:"quarantined,noindex"`

	// Maintenance message if the bot is in maintenance.
	//
	// Maintenance state, just like quarantined state, means the bot should not
	// receive tasks. The difference is that maintenance is an expected condition:
	//   - The bot moves into maintenance state in expected moments.
	//   - It is expected to be short and end automatically.
	Maintenance string `gae:"maintenance_msg,noindex"`

	// TaskID is the packed TaskRunResult key of the relevant task, if any.
	//
	// For BotInfo, it identifies the current TaskRunResult being executed by
	// the bot.
	//
	// For BotEvent, it is relevant for event types `request_task`, `task_killed`,
	// `task_completed`, `task_error`.
	//
	// Note that it is **not** a packed TaskResultSummary. This `task_id` ends in
	// `1` instead of `0`.
	//
	// TODO(vadimsh): This is unfortunate, since this field ends up in BQ exports
	// where it causes confusion: task IDs in other BQ exports are "packed
	// TaskResultSummary ID", i.e. end in 0. This complicates joining BQ tables.
	TaskID string `gae:"task_id,noindex"`

	// LastSeen is the last time the bot contacted the server, if ever.
	//
	// Note that it is unindexed to avoid hotspotting the datastore, see
	// https://chromium.googlesource.com/infra/luci/luci-py/+/4e9aecba
	LastSeen datastore.Optional[time.Time, datastore.Unindexed] `gae:"last_seen_ts"`

	// IdleSince is when the bot became idle last time, if ever.
	//
	// It is unset when running the task or hooks.
	IdleSince datastore.Optional[time.Time, datastore.Unindexed] `gae:"idle_since_ts"`

	// LegacyProperties is no longer used.
	LegacyLeaseID LegacyProperty `gae:"lease_id"`

	// LegacyLeaseExpiration is no longer used.
	LegacyLeaseExpiration LegacyProperty `gae:"lease_expiration_ts"`

	// LegacyLeasedIndefinitely is no longer used.
	LegacyLeasedIndefinitely LegacyProperty `gae:"leased_indefinitely"`

	// LegacyMachineType is no longer used.
	LegacyMachineType LegacyProperty `gae:"machine_type"`

	// LegacyMachineLease is no longer used.
	LegacyMachineLease LegacyProperty `gae:"machine_lease"`

	// LegacyStateJSON is no longer used.
	LegacyStateJSON LegacyProperty `gae:"state_json"`

	// LegacyDimensions is no longer used.
	LegacyDimensions LegacyProperty `gae:"dimensions"`

	// LegacyIsBusy is no longer used.
	LegacyIsBusy LegacyProperty `gae:"is_busy"`
}

// BotInfo contains the latest information about a bot.
type BotInfo struct {
	BotCommon

	// Extra are entity properties that didn't match any declared ones below.
	//
	// Should normally be empty.
	Extra datastore.PropertyMap `gae:"-,extra"`

	// Key is derived based on the bot ID, see BotInfoKey.
	Key *datastore.Key `gae:"$key"`

	// Dimensions is a list of dimensions reported by the bot.
	//
	// Dimensions are used for task selection. They are encoded as a sorted list
	// of `key:value` strings. Keep in mind that the same key can be used
	// multiple times.
	//
	// The index is used to filter bots by their dimensions in bot listing API.
	Dimensions []string `gae:"dimensions_flat"`

	// Composite encodes the current state of the bot.
	//
	// For datastore performance reasons it encodes multiple aspects of the state
	// in a single indexed multi-valued field, resulting in a somewhat weird
	// semantics.
	//
	// The slice always have 4 items, with following meaning:
	//
	// Composite[0] is one of:
	//    BotStateInMaintenance    = 1 << 8  # 256
	//    BotStateNotInMaintenance = 1 << 9  # 512
	// Composite[1] is one of:
	//    BotStateDead  = 1 << 6  # 64
	//    BotStateAlive = 1 << 7  # 128
	// Composite[2] is one of:
	//    BotStateQuarantined = 1 << 2  # 4
	//    BotStateHealthy     = 1 << 3  # 8
	// Composite[3] is one of:
	//    BotStateBusy = 1 << 0  # 1
	//    BotStateIdle = 1 << 1  # 2
	Composite []BotStateEnum `gae:"composite"`

	// FirstSeen is when the bot was seen for the first time.
	FirstSeen time.Time `gae:"first_seen_ts,noindex"`

	// TaskName matches TaskRequest.Name of the task the the bot executes now.
	//
	// In other words its the title of the task identified by BotCommon.TaskID.
	// Empty if the bot is not executing any tasks now.
	TaskName string `gae:"task_name,noindex"`
}

// BotInfoKey builds a BotInfo key given the bot ID.
func BotInfoKey(ctx context.Context, botID string) *datastore.Key {
	return datastore.NewKey(ctx, "BotInfo", "info", 0, BotRootKey(ctx, botID))
}

// BotID extracts the bot ID from the entity key.
func (b *BotInfo) BotID() string {
	return b.Key.Parent().StringID()
}

// IsDead is true if this bot is considered dead.
func (b *BotInfo) IsDead() bool {
	return len(b.Composite) > 1 && b.Composite[1] == BotStateDead
}

// IsInMaintenance is true if this bot is in maintenance.
func (b *BotInfo) IsInMaintenance() bool {
	return len(b.Composite) > 0 && b.Composite[0] == BotStateInMaintenance
}

// GetStatus returns the bot status.
func (b *BotInfo) GetStatus() string {
	for _, v := range b.Composite {
		switch v {
		case BotStateInMaintenance:
			return "maintenance"
		case BotStateQuarantined:
			return "quarantined"
		case BotStateDead:
			return "dead"
		case BotStateBusy:
			return "running"
		}
	}
	return "ready"
}

// DimenionsByKey returns a list of dimension values with the given key.
func (b *BotInfo) DimenionsByKey(k string) (values []string) {
	pfx := k + ":"
	for _, kv := range b.Dimensions {
		if val, ok := strings.CutPrefix(kv, pfx); ok {
			values = append(values, val)
		}
	}
	return values
}

// ToProto converts BotInfo to apipb.BotInfo.
func (b *BotInfo) ToProto() *apipb.BotInfo {
	info := &apipb.BotInfo{
		BotId:           b.BotID(),
		TaskId:          b.TaskID,
		TaskName:        b.TaskName,
		ExternalIp:      b.ExternalIP,
		AuthenticatedAs: string(b.AuthenticatedAs),
		IsDead:          b.IsDead(),
		Quarantined:     b.Quarantined,
		MaintenanceMsg:  b.Maintenance,
		Dimensions:      dimensionsFlatToPb(b.Dimensions),
		Version:         b.Version,
		State:           string(b.State),
	}
	if !b.FirstSeen.IsZero() {
		info.FirstSeenTs = timestamppb.New(b.FirstSeen)
	}
	if ts := b.LastSeen.Get(); !ts.IsZero() {
		info.LastSeenTs = timestamppb.New(ts)
	}
	return info
}

// BotInfoQuery prepares a query that fetches BotInfo entities.
func BotInfoQuery() *datastore.Query {
	return datastore.NewQuery("BotInfo")
}

// FilterBotsByDimensions limits a BotInfo query to return bots matching these
// dimensions.
//
// For complex filters this may split the query into multiple queries that need
// to run in parallel with their results merged. See SplitForQuery() in Filter
// for more details.
func FilterBotsByDimensions(q *datastore.Query, mode SplitMode, dims Filter) []*datastore.Query {
	return dims.Apply(q, "dimensions_flat", mode)
}

// FilterBotsByState limits a BotInfo query to return bots in particular state.
func FilterBotsByState(q *datastore.Query, state StateFilter) *datastore.Query {
	switch state.Quarantined {
	case apipb.NullableBool_NULL:
		// Don't filter.
	case apipb.NullableBool_TRUE:
		q = q.Eq("composite", BotStateQuarantined)
	case apipb.NullableBool_FALSE:
		q = q.Eq("composite", BotStateHealthy)
	}

	switch state.InMaintenance {
	case apipb.NullableBool_NULL:
		// Don't filter.
	case apipb.NullableBool_TRUE:
		q = q.Eq("composite", BotStateInMaintenance)
	case apipb.NullableBool_FALSE:
		q = q.Eq("composite", BotStateNotInMaintenance)
	}

	switch state.IsBusy {
	case apipb.NullableBool_NULL:
		// Don't filter.
	case apipb.NullableBool_TRUE:
		q = q.Eq("composite", BotStateBusy)
	case apipb.NullableBool_FALSE:
		q = q.Eq("composite", BotStateIdle)
	}

	switch state.IsDead {
	case apipb.NullableBool_NULL:
		// Don't filter.
	case apipb.NullableBool_TRUE:
		q = q.Eq("composite", BotStateDead)
	case apipb.NullableBool_FALSE:
		q = q.Eq("composite", BotStateAlive)
	}

	return q
}

// BotEvent captures information about the bot during some state transition.
//
// Entities of this kind are immutable. They essentially form a log with the
// bot history. Entries are indexed by the timestamp to allow querying this log
// in the chronological order.
type BotEvent struct {
	BotCommon

	// Extra are entity properties that didn't match any declared ones below.
	//
	// Should normally be empty.
	Extra datastore.PropertyMap `gae:"-,extra"`

	// Key identifies the bot and this particular event.
	//
	// ID is auto-generated by the datastore. The bot is identified via the
	// parent key, which can be constructed via BotRootKey(...).
	Key *datastore.Key `gae:"$key"`

	// Timestamp of when this event happened.
	//
	// The index is used in a bunch of places:
	// 1. For ordering events chronologically when listing them.
	// 2. Pagination for BQ exports.
	// 3. Old event cleanup cron.
	Timestamp time.Time `gae:"ts"`

	// EventType describes what has happened.
	EventType BotEventType `gae:"event_type,noindex"`

	// Message is an optional free form message associated with the event.
	Message string `gae:"message,noindex"`

	// Dimensions is a list of dimensions reported by the bot.
	//
	// TODO(vadimsh): Stop indexing this after turning down native Swarming
	// scheduler. This index is only used in has_capacity(...) implementation,
	// which is a part of the native Swarming scheduler and it not used when
	// running on top of RBE. This index is pretty big (~6 TB) and getting rid
	// of it may also speed up the bot event insertion transaction.
	Dimensions []string `gae:"dimensions_flat"`
}

// ToProto converts BotEvent to apipb.BotEventResponse.
func (e *BotEvent) ToProto() *apipb.BotEventResponse {
	return &apipb.BotEventResponse{
		Ts:              timestamppb.New(e.Timestamp),
		EventType:       string(e.EventType),
		Message:         e.Message,
		Dimensions:      dimensionsFlatToPb(e.Dimensions),
		State:           string(e.State),
		ExternalIp:      e.ExternalIP,
		AuthenticatedAs: string(e.AuthenticatedAs),
		Version:         e.Version,
		Quarantined:     e.Quarantined,
		MaintenanceMsg:  e.Maintenance,
		TaskId:          e.TaskID,
	}
}

// BotEventsQuery prepares a query that fetches BotEvent entities for a bot.
//
// Most recent events are returned first.
func BotEventsQuery(ctx context.Context, botID string) *datastore.Query {
	return datastore.NewQuery("BotEvent").Ancestor(BotRootKey(ctx, botID)).Order("-ts")
}

// BotDimensions is a map with bot dimensions as `key => [values]`.
//
// This type represents bot dimensions in the datastore as a JSON-encoded
// unindexed blob. There's an alternative "flat" indexed representation as a
// list of `key:value` pairs. It is used in BotCommon.Dimensions property.
type BotDimensions map[string][]string

// ToProperty stores the value as a JSON-blob property.
func (p *BotDimensions) ToProperty() (datastore.Property, error) {
	return ToJSONProperty(p)
}

// FromProperty loads a JSON-blob property.
func (p *BotDimensions) FromProperty(prop datastore.Property) error {
	return FromJSONProperty(prop, p)
}

// ToProto returns []apipb.StringListPair, sorted by keys.
func (p BotDimensions) ToProto() []*apipb.StringListPair {
	return MapToStringListPair((map[string][]string)(p), true)
}

// ToStructPB returns a structpb.Struct.
func (p BotDimensions) ToStructPB() *structpb.Struct {
	s := &structpb.Struct{
		Fields: make(map[string]*structpb.Value, len(p)),
	}
	for key, valList := range p {
		vals := make([]*structpb.Value, 0, len(valList))
		for _, val := range valList {
			vals = append(vals, &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: val}})
		}
		s.Fields[key] = &structpb.Value{
			Kind: &structpb.Value_ListValue{
				ListValue: &structpb.ListValue{Values: vals},
			},
		}
	}
	return s
}
