// Copyright 2025 The LUCI Authors.
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
	"slices"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/botstate"
)

const (
	// oldBotEventsCutOff defines age of BotEvent entities for the TTL policy.
	oldBotEventsCutOff = 24 * 4 * 7 * time.Hour
	// oldBotInfoCutoff defines age of BotInfo entities for the TTL policy.
	oldBotInfoCutoff = oldBotEventsCutOff + 4*time.Hour
)

// Events that happen very often and which are not worth to record every time.
//
// They are recorded only if something noteworthy happens at the same time
// (e.g. the bot is changing its dimensions).
var frequentEvents = map[BotEventType]bool{
	BotEventIdle:       true,
	BotEventPolling:    true,
	BotEventSleep:      true,
	BotEventTaskUpdate: true,
}

// Events that may result in creation of new BotInfo entities (i.e. a new bot
// appearing). Most often this will just be BotEventConnected, but other events
// are theoretically possible too in case the bot was deleted while it was
// still running.
var healthyBotEvents = map[BotEventType]bool{
	BotEventConnected: true,
	BotEventIdle:      true,
	BotEventPolling:   true,
	BotEventRestart:   true,
	BotEventSleep:     true,
	BotEventTask:      true,
	BotEventUpdate:    true,
}

// Events indicating that the bot is idle (not running any tasks, waits for new
// tasks).
//
// Note that BotEventConnected is not here because when the bot connects it
// doesn't really know yet if there are any pending tasks. If it is really idle,
// it will report BotEventIdle separately.
//
// Also BotEventPolling means the bot is just pinging Swarming while still
// processing the queue of RBE tasks. The bot is not idle in that case. If it
// finds the RBE queue is empty, it will report BotEventIdle separately.
//
// BotEventSleep is for when Swarming itself instructs the bot to stop fetching
// RBE tasks and just sleep instead.
var idleBotEvents = map[BotEventType]bool{
	BotEventIdle:  true,
	BotEventSleep: true,
}

// Events indicating that the bot process is gone for good.
//
// The bot will move into "idle" state as soon as these events are reported.
var stoppedBotEvents = map[BotEventType]bool{
	BotEventShutdown: true,
	BotEventMissing:  true,
}

// Events that happen when the bot just wants to log some information.
//
// They do not affect idleness status of the bot.
var loggingBotEvents = map[BotEventType]bool{
	BotEventConnected: true, // preserve the previous idleness status on reconnect
	BotEventError:     true,
	BotEventLog:       true,
}

// BotInfoUpdate is a change to BotInfo that may also create a BotEvent.
//
// Submitting BotInfoUpdate is the only valid way to change BotInfo entities.
type BotInfoUpdate struct {
	// BotID is the ID of the bot being updated.
	//
	// Required.
	BotID string

	// Current is an existing BotInfo entity to use when in a transaction.
	//
	// If set, the update must be prepared via Prepare and submitted as a part of
	// a transaction. Must not be set when not in a transaction (i.e. when using
	// Submit).
	Current *BotInfo

	// EventType is what event is causing this BotInfo update to be recorded.
	//
	// Required.
	EventType BotEventType

	// EventDedupKey is an optional string used to skip duplicate events.
	//
	// This is best effort. Only sequential duplicate events are deduplicated.
	// This key is joined with EventType to get the full dedup key stored in
	// BotInfo.
	EventDedupKey string

	// EventMessage is an optional free form string to store with the event.
	//
	// It will show up in the UI as the description of the event.
	EventMessage string

	// Dimensions is a sorted list of dimensions to assign to the bot.
	//
	// This is derived from the dimensions passed by the bot when it polls for
	// tasks (in particular with events BotEventPolling and BotEventIdle). Must
	// have at least "id:<BotID>" dimension present.
	//
	// If empty, the current dimensions will be left unchanged.
	Dimensions []string

	// BotGroupDimensions are extra dimension associated with the bot in the
	// bot group config in bots.cfg.
	//
	// These are used only if Dimensions are unset and this update is the first
	// update ever that registers the BotInfo. We need to associate some
	// dimensions with a new bot, and dimensions in the config is all we have at
	// this point. The type matches cfg.BotGroup(...).Dimensions.
	//
	// Can be omitted if Dimensions are set.
	BotGroupDimensions map[string][]string

	// State is a JSON dict with the bot state as reported by the bot itself.
	//
	// If nil, the current recorded bot state won't be changed.
	State *botstate.Dict

	// CallInfo is information about the current bot API call, if any.
	//
	// It is absent when BotInfo is updated by the server itself (e.g. from
	// a cron job). The latest known call info will be left unchanged in that
	// case.
	//
	// Additionally, every update that has this field will bump bot's LastSeen
	// timestamp (other updates wont, since they are made by the server).
	CallInfo *BotEventCallInfo

	// HealthInfo is bot's health status to assign to the bot in the datastore.
	//
	// If nil, the current health status won't be updated.
	//
	// This is usually derived from the dimensions and state as reported by the
	// bot.
	HealthInfo *BotHealthInfo

	// TaskInfo is information about the task assigned to the bot.
	//
	// If nil, the current values won't be touched.
	//
	// If a pointer to an empty struct, the current values will be reset. This is
	// used when the bot becomes idle.
	TaskInfo *BotEventTaskInfo
}

// BotEventCallInfo is information describing the bot API call.
//
// It is present only if the bot is actually calling the server and absent for
// all server-generated events.
type BotEventCallInfo struct {
	// SessionID is the ID of the current bot session.
	SessionID string
	// Version of the bot code the bot is running, if known.
	Version string
	// ExternalIP is the bot's IP address as seen by the server.
	ExternalIP string
	// AuthenticatedAs is the bot's credentials as seen by the server.
	AuthenticatedAs identity.Identity
}

// BotHealthInfo is health status of a bot.
//
// It is usually extracted from dimensions and the state as reported by the bot.
// If any of the fields are set, the bot won't be picking up any tasks.
type BotHealthInfo struct {
	// Quarantined is a quarantine message if the bot is in quarantine.
	//
	// A bot can report itself as being in quarantine if it can't run tasks
	// anymore due to some "unexpected" condition (for example, its disk is full).
	//
	// This state can be set from bot hooks.
	Quarantined string

	// Maintenance is a maintenance message if the bot is in maintenance state.
	//
	// Unlike quarantine, occasionally being in a maintenance state is expected.
	// This state is used to indicate that the bot is doing some periodic
	// maintenance work that can take some time.
	//
	// This state can be set from bot hooks.
	Maintenance string
}

// BotEventTaskInfo is information about the task assigned to the bot.
type BotEventTaskInfo struct {
	// TaskID is the packed TaskRunResult key of the task assigned to the bot.
	TaskID string
	// TaskName matches TaskRequest.Name of the task identified by TaskID.
	TaskName string
}

// PreparedBotInfoUpdate is details about a bot info update.
type PreparedBotInfoUpdate struct {
	// BotInfo is the BotInfo entity stored in the datastore after the update.
	//
	// It either an updated BotInfo or an existing one if the update is not
	// necessary. Always set.
	BotInfo *BotInfo

	// BotEvent is the BotEvent entity stored in the datastore.
	//
	// Set only if the event was actually recorded.
	BotEvent *BotEvent

	// EntitiesToPut is a list of entities to put as a part of the transaction.
	//
	// It contains BotInfo and/or BotEvent, or nothing if the update is not
	// necessary. For convenience of calling datastore.Put(...).
	EntitiesToPut []any
}

// Submit runs a transaction that applies this update to the BotInfo entity
// currently stored in the datastore, potentially also recording it as a new
// BotEvent entity.
//
// Returns datastore errors. All such errors are transient.
func (u *BotInfoUpdate) Submit(ctx context.Context) (*PreparedBotInfoUpdate, error) {
	if datastore.CurrentTransaction(ctx) != nil {
		panic("Submit must not be used in a transaction, use Prepare instead")
	}
	if u.Current != nil {
		panic("Current can only be used when reusing an existing transaction")
	}
	var prepared *PreparedBotInfoUpdate
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		var err error
		if prepared, err = u.Prepare(ctx); err != nil {
			return err
		}
		if len(prepared.EntitiesToPut) != 0 {
			if err := datastore.Put(ctx, prepared.EntitiesToPut...); err != nil {
				return err
			}
			prepared.EntitiesToPut = nil // dealt with them already
		}
		return err
	}, nil)
	if err != nil {
		return nil, err
	}
	return prepared, nil
}

// Prepare prepares entities to apply this update.
//
// It must be called in a transaction. This method is useful if there's already
// a transaction open that makes some other changes.
//
// Returns datastore errors. All errors are transient.
func (u *BotInfoUpdate) Prepare(ctx context.Context) (*PreparedBotInfoUpdate, error) {
	if datastore.CurrentTransaction(ctx) == nil {
		panic("Prepare must be used in a transaction")
	}
	if len(u.Dimensions) != 0 {
		if !slices.IsSorted(u.Dimensions) {
			panic("Dimensions must be sorted")
		}
		if _, found := slices.BinarySearch(u.Dimensions, "id:"+u.BotID); !found {
			panic(fmt.Sprintf("id:<BotID> dimension is missing or incorrect in %v", u.Dimensions))
		}
	}

	now := clock.Now(ctx).UTC()
	key := BotInfoKey(ctx, u.BotID)

	// Get the current BotInfo (if any) to update it.
	var current *BotInfo
	if u.Current != nil {
		if !u.Current.Key.Equal(key) {
			panic(fmt.Sprintf("unexpected key %s for bot ID %s", u.Current.Key, u.BotID))
		}
		// Make a shallow copy since we are going to modify this entity. Modifying
		// it in-place will cause bugs in case Prepare transaction is retried.
		cpy := *u.Current
		current = &cpy
	} else {
		current = &BotInfo{Key: key}
		switch err := datastore.Get(ctx, current); {
		case errors.Is(err, datastore.ErrNoSuchEntity):
			current = nil
		case err != nil:
			return nil, errors.Annotate(err, "fetching current BotInfo").Err()
		}
	}

	// If this is a first update ever, prepopulate dimensions based on the config.
	storeBotInfo := true
	if current == nil {
		current = &BotInfo{
			Key:        key,
			Dimensions: u.connectingBotDims(),
			FirstSeen:  now,
			Composite: []BotStateEnum{
				BotStateNotInMaintenance,
				BotStateAlive,
				BotStateHealthy,
				BotStateIdle,
			},
		}
		// Create BotInfo only if this event indicates the bot is actually alive.
		// This check exists to workaround race conditions when deleting bots (e.g.
		// when a dead bot suddenly comes back to life just to report that it has
		// failed a task and is terminating now).
		storeBotInfo = healthyBotEvents[u.EventType]
		if !storeBotInfo {
			logging.Warningf(ctx, "No BotInfo for %s when storing %s", u.BotID, u.EventType)
		}
	}

	// Do nothing if this event has been processed already.
	eventDedupKey := u.fullEventDedupKey()
	if eventDedupKey != "" && current.LastEventDedupKey == eventDedupKey {
		logging.Warningf(ctx, "Skipping BotInfo update, event has been recorded already: %s", eventDedupKey)
		return &PreparedBotInfoUpdate{
			BotInfo: current,
		}, nil
	}
	current.LastEventDedupKey = eventDedupKey

	// Bump the expiration time every time the entity is touched. Note that this
	// field is unindexed (Cloud Datastore TTL policy doesn't need an index),
	// the cost of updating it is negligible.
	current.ExpireAt = now.Add(oldBotInfoCutoff)

	// Detect if dimensions has changed to know if we must emit an event even if
	// the event type is not otherwise interesting. Most often this code path is
	// hit when an idle bot is dynamically changing its dimensions.
	dimensionsChanged := false
	if len(u.Dimensions) != 0 {
		dimensionsChanged = !slices.Equal(current.Dimensions, u.Dimensions)
		current.Dimensions = u.Dimensions
	}

	// Unlike dimension changes, state changes do not trigger recording of the
	// event, since state is updated on every bot call and we don't want to
	// emit an event for every bot call. We just update the state inside BotInfo.
	if u.State != nil {
		current.State = *u.State
	}

	// Update LastSeen only if the event was originated on the bot. LastSeen is
	// scanned by a cron job to detect dead bots. It is specifically not indexed
	// to avoid hotspotting the datastore.
	if u.CallInfo != nil {
		current.LastSeen = datastore.NewUnindexedOptional(now)
		current.SessionID = u.CallInfo.SessionID
		current.ExternalIP = u.CallInfo.ExternalIP
		current.AuthenticatedAs = u.CallInfo.AuthenticatedAs
		if u.CallInfo.Version != "" {
			current.Version = u.CallInfo.Version
		}
	}

	// Update the health status of the bot. This is used below to calculate
	// values of other fields.
	if u.HealthInfo != nil {
		current.Maintenance = u.HealthInfo.Maintenance
		current.Quarantined = u.HealthInfo.Quarantined != ""
	}

	// Update the task assigned to the bot (or reset it if TaskInfo is an empty
	// struct).
	if u.TaskInfo != nil {
		current.TaskID = u.TaskInfo.TaskID
		current.TaskName = u.TaskInfo.TaskName
	}

	// IdleSince is set only for bots that can potentially run tasks (i.e. they
	// are healthy), but currently don't. An edge case is events indicating the
	// bot has stopped: missing bots are "idle" by definition regardless of any
	// prior state.
	//
	// Logging events can happen for idle or busy bots. They do not affect
	// idleness state of a bot.
	if !loggingBotEvents[u.EventType] {
		idle := stoppedBotEvents[u.EventType] ||
			(idleBotEvents[u.EventType] && !current.Quarantined && current.Maintenance == "")
		if idle {
			if !current.IdleSince.IsSet() {
				current.IdleSince = datastore.NewUnindexedOptional(now)
			}
		} else {
			current.IdleSince.Unset()
		}
	}

	// "IsDead" state is "sticky" and gets updated based on events that happen
	// to the bot. Some events (like BotEventLog) may leave the bot in its current
	// state, whatever it is.
	dead := current.IsDead()
	switch {
	case healthyBotEvents[u.EventType]:
		dead = false
	case stoppedBotEvents[u.EventType]:
		dead = true
	}

	// Recalculate the new indexed state of the bot.
	pick := func(b bool, yes, no BotStateEnum) BotStateEnum {
		if b {
			return yes
		}
		return no
	}
	composite := []BotStateEnum{
		pick(current.Maintenance != "", BotStateInMaintenance, BotStateNotInMaintenance),
		pick(dead, BotStateDead, BotStateAlive),
		pick(current.Quarantined, BotStateQuarantined, BotStateHealthy),
		pick(current.IdleSince.IsSet(), BotStateIdle, BotStateBusy),
	}

	// Detect if the bot state has changed. This indicates this event is
	// "interesting" and should be logged.
	compositeChanged := !slices.Equal(current.Composite, composite)
	current.Composite = composite

	// See comment above regarding storeBotInfo.
	var toPut []any
	if storeBotInfo {
		toPut = append(toPut, current)
	}

	// Store BotEvent only if it looks interesting enough. Otherwise we'll have
	// tons and tons of BotEventIdle events.
	var eventToPut *BotEvent
	if !frequentEvents[u.EventType] || dimensionsChanged || compositeChanged {
		eventToPut = &BotEvent{
			// Note: this means the entity ID will be auto-generated. We don't expose
			// BotEvent entity IDs anywhere. This is mostly due to historical reasons.
			Key:        datastore.NewKey(ctx, "BotEvent", "", 0, BotRootKey(ctx, u.BotID)),
			Timestamp:  now,
			EventType:  u.EventType,
			Message:    u.eventMessage(),
			Dimensions: current.Dimensions,
			BotCommon: BotCommon{
				State:           current.State,
				SessionID:       current.SessionID,
				ExternalIP:      current.ExternalIP,
				AuthenticatedAs: current.AuthenticatedAs,
				Version:         current.Version,
				Quarantined:     current.Quarantined,
				Maintenance:     current.Maintenance,
				TaskID:          current.TaskID,
				LastSeen:        current.LastSeen,
				IdleSince:       current.IdleSince,
				ExpireAt:        now.Add(oldBotEventsCutOff),
			},
		}
		toPut = append(toPut, eventToPut)
	}

	return &PreparedBotInfoUpdate{
		BotInfo:       current,
		BotEvent:      eventToPut,
		EntitiesToPut: toPut,
	}, nil
}

// connectingBotDims is a list of dimensions for a bot seen for the first time.
func (u *BotInfoUpdate) connectingBotDims() []string {
	dims := make([]string, 0, 1+len(u.BotGroupDimensions))
	dims = append(dims, "id:"+u.BotID)
	for key, vals := range u.BotGroupDimensions {
		for _, val := range vals {
			dims = append(dims, fmt.Sprintf("%s:%s", key, val))
		}
	}
	slices.Sort(dims)
	return dims
}

// fullEventDedupKey is the full event ID to store in BotInfo.LastEvent.
func (u *BotInfoUpdate) fullEventDedupKey() string {
	if u.EventDedupKey == "" {
		return ""
	}
	return fmt.Sprintf("%s:%s", u.EventType, u.EventDedupKey)
}

// eventMessage is the final event message to store.
func (u *BotInfoUpdate) eventMessage() string {
	switch {
	case u.EventMessage != "":
		return u.EventMessage
	case u.HealthInfo != nil && u.HealthInfo.Maintenance != "":
		return u.HealthInfo.Maintenance
	case u.HealthInfo != nil && u.HealthInfo.Quarantined != "":
		return u.HealthInfo.Quarantined
	default:
		return ""
	}
}
