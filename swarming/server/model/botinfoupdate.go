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
	"strings"
	"time"

	"go.chromium.org/luci/auth/identity"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/tsmon/field"
	"go.chromium.org/luci/common/tsmon/metric"
	"go.chromium.org/luci/common/tsmon/types"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/swarming/server/botstate"
)

var botInfoTxnCount = metric.NewCounter(
	"swarming/botinfo/txn",
	"Counter of BotInfo transactions",
	&types.MetricMetadata{},
	// The BotEventType that caused the transaction.
	field.String("event"),
	// One of: ok, timeout, canceled, conflict, skipped, other.
	field.String("outcome"),
	// The number of attempts used (can be 0 if the first BeginTransaction RPC fails).
	field.Int("attempts"),
)

// errSkippedUpdate is used to signal the transaction that it should rollback.
var errSkippedUpdate = errors.Reason("update is skipped by Prepare callback").Err()

const (
	// oldBotEventsCutOff defines age of BotEvent entities for the TTL policy.
	oldBotEventsCutOff = 24 * 4 * 7 * time.Hour
	// oldBotInfoCutoff defines age of BotInfo entities for the TTL policy.
	oldBotInfoCutoff = oldBotEventsCutOff + 4*time.Hour
	// maxTxnAttempts is how many times to retry BotInfo transaction.
	maxTxnAttempts = 10
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
// The bot will move into "idle" state as soon as these events are reported. Any
// task still associated with the bot will be moved into BOT_DIED state.
var stoppedBotEvents = map[BotEventType]bool{
	BotEventShutdown: true,
	BotEventMissing:  true,
	BotEventDeleted:  true,
}

// Events that happen when the bot just wants to log some information.
//
// They do not affect idleness status of the bot.
var loggingBotEvents = map[BotEventType]bool{
	BotEventConnected: true, // preserve the previous idleness status on reconnect
	BotEventError:     true,
	BotEventLog:       true,
}

// Events that indicate the bot has finished the task (successfully or not).
// If TaskID gets reset by any other event, it means the bot has abandoned the
// task (aka "bot died").
//
// All BotInfo updates that report these events **must** be a part of
// a transaction that also moves the task to a finished state. Use Prepare
// callback to set it up.
var taskCompletionEvents = map[BotEventType]bool{
	BotEventTaskCompleted: true,
	BotEventTaskError:     true,
	BotEventTaskKilled:    true,
}

// BotInfoUpdate is a change to BotInfo that may also create a BotEvent.
//
// Submitting BotInfoUpdate is the only valid way to change or delete BotInfo
// entities.
//
// All events other than BotEventDeleted may create BotInfo, if it is missing.
// BotEventDeleted will delete it (if it is present).
//
// The value of TaskInfo field is tightly coupled to the event kind. Some events
// are expected to set new TaskID (by passing a populated TaskInfo), some are
// resetting it (and should not be passing TaskInfo), and some are leaving it
// untouched (and they should also not be passing TaskInfo).
//
// See TaskChangeAspects for how events are split into these categories.
type BotInfoUpdate struct {
	// BotID is the ID of the bot being updated.
	//
	// Required.
	BotID string

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

	// Prepare is an optional callback called in the transaction after fetching
	// the BotInfo entity, but before applying the update.
	//
	// It can do any additional transactional work that touches entities other
	// than BotInfo, and/or decide if the update should be skipped. The callback
	// must not modify BotInfo itself.
	//
	// If this is the first update for this bot, `bot` will be nil.
	//
	// If the callback returns false, this update will be silently skipped and
	// all transactional work done by the callback (if any) rolled back.
	//
	// Returning an error aborts the update as well, with the error propagated.
	Prepare func(ctx context.Context, bot *BotInfo) (proceed bool, err error)

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
	// If TaskChangeAspects[EventType] is TaskChangeSet, it must be populated and
	// have non-empty TaskID. Otherwise it should be nil.
	//
	// When it is nil, if TaskChangeAspects[EventType] is TaskChangeReset, the
	// task ID assigned to the bot will be reset. Otherwise it will be left
	// untouched.
	TaskInfo *BotEventTaskInfo

	// EffectiveBotIDInfo can be used to change the bot ID in RBE sessions.
	//
	// It is derived from the bot dimensions and RBE config in the
	// BotAPIServer.Poll handler.
	//
	// Takes effect only if non-nil and Dimensions are also populated. If non-nil
	// but the actual ID is empty, the RBE session will just use BotID as its
	// bot ID.
	EffectiveBotIDInfo *RBEEffectiveBotIDInfo
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
	// TaskFlags hold aspects of the task, see TaskFlag*.
	TaskFlags TaskFlags
}

// RBEEffectiveBotIDInfo carries the bot ID to use in RBE sessions.
type RBEEffectiveBotIDInfo struct {
	// RBEEffectiveBotID is the bot ID to use in RBE sessions.
	//
	// An empty string means to use the standard bot ID.
	RBEEffectiveBotID string
}

// SubmittedBotInfoUpdate is details about a submitted bot info update.
type SubmittedBotInfoUpdate struct {
	// BotInfo is the BotInfo entity with an update applied to it.
	//
	// It either an updated BotInfo or an existing one if the update is not
	// necessary.
	//
	// For all events other than BotEventDeleted, it is what ends up stored in
	// the datastore after the update. For BotEventDeleted, there's no BotInfo in
	// the datastore anymore and this is just a snapshot of the just deleted
	// BotInfo. It can be nil if there were no BotInfo to begin with.
	BotInfo *BotInfo

	// BotEvent is the BotEvent entity stored in the datastore.
	//
	// Set only if the event was actually recorded.
	BotEvent *BotEvent

	// AbandonedTaskID is set if this task was abandoned in reaction to the event.
	AbandonedTaskID string

	// entitiesToPut is a list of entities to put in the transaction.
	entitiesToPut []any
	// entitiesToDelete is a list of entities to delete in the transaction.
	entitiesToDelete []any
}

// PanicIfInvalid panics if this BotInfoUpdate violates the contract documented
// in BotInfoUpdate comments.
//
// This can only happen in presence of bugs. Intended to be called from tests
// that mock out BotInfoUpdate.Submit().
func (u *BotInfoUpdate) PanicIfInvalid() {
	aspect, found := TaskChangeAspects[u.EventType]
	if !found {
		panic(fmt.Sprintf("unrecognized event %s", u.EventType))
	}
	if aspect == TaskChangeSet {
		if u.TaskInfo == nil {
			panic(fmt.Sprintf("event %s is missing TaskInfo", u.EventType))
		}
		if u.TaskInfo.TaskID == "" {
			panic(fmt.Sprintf("event %s is missing TaskID in TaskInfo", u.EventType))
		}
	} else {
		if u.TaskInfo != nil {
			panic(fmt.Sprintf("event %s is unexpectedly passing TaskInfo ", u.EventType))
		}
	}

	if len(u.Dimensions) != 0 {
		if !slices.IsSorted(u.Dimensions) {
			panic("Dimensions must be sorted")
		}
		if _, found := slices.BinarySearch(u.Dimensions, "id:"+u.BotID); !found {
			panic(fmt.Sprintf("id:<BotID> dimension is missing or incorrect in %v", u.Dimensions))
		}
	}
}

// Submit runs a transaction that applies this update to the BotInfo entity
// currently stored in the datastore, potentially also recording it as a new
// BotEvent entity.
//
// If the update was skipped by the Prepare callback, returns (nil, nil).
//
// Returns datastore errors. All such errors are transient.
func (u *BotInfoUpdate) Submit(ctx context.Context) (*SubmittedBotInfoUpdate, error) {
	var submitted *SubmittedBotInfoUpdate
	var attempt int
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		attempt++
		var err error
		if submitted, err = u.execute(ctx); err != nil {
			return err
		}
		if len(submitted.entitiesToPut) != 0 {
			if err := datastore.Put(ctx, submitted.entitiesToPut...); err != nil {
				return err
			}
		}
		if len(submitted.entitiesToDelete) != 0 {
			if err := datastore.Delete(ctx, submitted.entitiesToDelete...); err != nil {
				return err
			}
		}
		if submitted.AbandonedTaskID != "" {
			// TODO: Actually move the task into BOT_DIED state.
			logging.Warningf(ctx, "Abandoning %q", submitted.AbandonedTaskID)
		}
		return err
	}, &datastore.TransactionOptions{
		Attempts:            maxTxnAttempts,
		AllocateIDsOnCommit: true, // avoid unnecessary call to AllocateIDs RPC
	})
	u.reportBotInfoTxn(ctx, attempt, err)
	switch {
	case err == nil:
		return submitted, nil
	case errors.Is(err, errSkippedUpdate):
		return nil, nil
	default:
		return nil, err
	}
}

// execute prepares entities to apply this update.
//
// Returns datastore errors. All errors are transient.
func (u *BotInfoUpdate) execute(ctx context.Context) (*SubmittedBotInfoUpdate, error) {
	u.PanicIfInvalid()

	now := clock.Now(ctx).UTC()
	key := BotInfoKey(ctx, u.BotID)

	// Get the current BotInfo (if any) to update it.
	current := &BotInfo{Key: key}
	switch err := datastore.Get(ctx, current); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		current = nil
	case err != nil:
		return nil, errors.Annotate(err, "fetching current BotInfo").Err()
	}

	// Do any extra transactional work and detect if we should proceed.
	if u.Prepare != nil {
		switch proceed, err := u.Prepare(ctx, current); {
		case err != nil:
			return nil, errors.Annotate(err, "in Prepare callback").Err()
		case !proceed:
			return nil, errSkippedUpdate
		}
	}

	// Do nothing if deleting an already absent BotInfo.
	if u.EventType == BotEventDeleted && current == nil {
		return &SubmittedBotInfoUpdate{
			BotInfo: nil, // indication that the bot was already gone
		}, nil
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
		return &SubmittedBotInfoUpdate{
			BotInfo: current,
		}, nil
	}
	current.LastEventDedupKey = eventDedupKey

	// Bump the expiration time every time the entity is touched. Note that this
	// field is unindexed (Cloud Datastore TTL policy doesn't need an index),
	// the cost of updating it is negligible.
	current.ExpireAt = now.Add(oldBotInfoCutoff)

	// Any additional human readable messages to store in the event.
	var extraMessages []string

	// Detect if dimensions has changed to know if we must emit an event even if
	// the event type is not otherwise interesting. Most often this code path is
	// hit when an idle bot is dynamically changing its dimensions.
	dimensionsChanged := false
	if len(u.Dimensions) != 0 {
		dimensionsChanged = !slices.Equal(current.Dimensions, u.Dimensions)
		current.Dimensions = u.Dimensions
		if u.EffectiveBotIDInfo != nil && current.RBEEffectiveBotID != u.EffectiveBotIDInfo.RBEEffectiveBotID {
			extraMessages = append(extraMessages, fmt.Sprintf("RBE effective bot ID: %q => %q", current.RBEEffectiveBotID, u.EffectiveBotIDInfo.RBEEffectiveBotID))
			current.RBEEffectiveBotID = u.EffectiveBotIDInfo.RBEEffectiveBotID
		}
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

	// We need to abandon the task on BotEventMissing (and similar) even though
	// they may not be changing TaskID. See BotInfo.LastAbandonedTask for details
	// why.
	var abandonedTaskID string
	if stoppedBotEvents[u.EventType] {
		abandonedTaskID = current.TaskID
	}

	// Update the task assigned to the bot (perhaps by resetting it).
	var completedTaskID string
	if taskChange := TaskChangeAspects[u.EventType]; taskChange != TaskChangeNone {
		if current.TaskID != "" {
			// The bot is either switching to idle state (if taskChange is
			// TaskChangeReset) or it is abandoning the current task and starting
			// another one. Either way the current task is done.
			current.LastFinishedTask = LastTaskDetails{
				TaskID:      current.TaskID,
				TaskName:    current.TaskName,
				TaskFlags:   current.TaskFlags,
				FinishedDue: u.EventType,
			}
			if taskCompletionEvents[u.EventType] {
				// A properly completed task ID. Will be reported in the corresponding
				// completion event.
				completedTaskID = current.TaskID
			} else {
				// If the assigned task was changed unexpectedly, we need to abandon it.
				// Most commonly this would be a bot reconnecting after an unexpected
				// reboot.
				abandonedTaskID = current.TaskID
			}
		}
		switch taskChange {
		case TaskChangeSet:
			current.TaskID = u.TaskInfo.TaskID
			current.TaskName = u.TaskInfo.TaskName
			current.TaskFlags = u.TaskInfo.TaskFlags
		case TaskChangeReset:
			current.TaskID = ""
			current.TaskName = ""
			current.TaskFlags = 0
		}
	}

	// Store the current task ID in the BotEvent, unless this is a task completion
	// event. In that case it makes more sense to store the ID of the just
	// completed task (since current.TaskID is "" already).
	eventTaskID := current.TaskID
	if completedTaskID != "" {
		eventTaskID = completedTaskID
	}

	// As another special case, if BotEventMissing or similar "terminal" events
	// caused a task to be abandoned, put the task ID with the event as well.
	// We don't do that in case abandonedTaskID was discovered due to e.g.
	// unexpected BotEventConnected. It would be weird to associate such events
	// with a task.
	if stoppedBotEvents[u.EventType] {
		eventTaskID = abandonedTaskID
	}

	// Make sure to notify the abandoned task at most once.
	if abandonedTaskID != "" && current.LastAbandonedTask != abandonedTaskID {
		current.LastAbandonedTask = abandonedTaskID
		extraMessages = append(extraMessages, fmt.Sprintf("Abandoned %s", abandonedTaskID))
	} else {
		abandonedTaskID = ""
	}

	// Update TerminationTaskID if the bot was shutdown by a termination task.
	if u.EventType == BotEventShutdown && current.LastFinishedTask.TaskFlags&TaskFlagTermination != 0 {
		current.TerminationTaskID = current.LastFinishedTask.TaskID
	} else {
		current.TerminationTaskID = ""
	}

	// Forget the last task history if this is a new bot session. That way if a
	// bot reconnects after a graceful termination, but then immediately
	// terminates again ungracefully, we won't mistakenly have TerminationTaskID
	// set.
	if u.EventType == BotEventConnected {
		current.LastFinishedTask = LastTaskDetails{}
		current.TerminationTaskID = ""
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

	var toPut []any
	var toDelete []any

	if u.EventType == BotEventDeleted {
		toDelete = append(toDelete, current)
	} else {
		// See comment above regarding storeBotInfo.
		if storeBotInfo {
			toPut = append(toPut, current)
		}
	}

	// Store BotEvent only if it looks interesting enough. Otherwise we'll have
	// tons and tons of BotEventIdle events.
	var eventToPut *BotEvent
	if !frequentEvents[u.EventType] || dimensionsChanged || compositeChanged || len(extraMessages) != 0 {
		eventToPut = &BotEvent{
			// Note: this key means the entity ID will be auto-generated when the
			// transaction is committed. We don't expose BotEvent entity IDs anywhere.
			// This auto-generated key is used mostly due to historical reasons.
			Key:        datastore.NewKey(ctx, "BotEvent", "", 0, BotRootKey(ctx, u.BotID)),
			Timestamp:  now,
			EventType:  u.EventType,
			Message:    u.eventMessage(extraMessages),
			Dimensions: current.Dimensions,
			BotCommon: BotCommon{
				State:           current.State,
				SessionID:       current.SessionID,
				ExternalIP:      current.ExternalIP,
				AuthenticatedAs: current.AuthenticatedAs,
				Version:         current.Version,
				Quarantined:     current.Quarantined,
				Maintenance:     current.Maintenance,
				TaskID:          eventTaskID,
				LastSeen:        current.LastSeen,
				IdleSince:       current.IdleSince,
				ExpireAt:        now.Add(oldBotEventsCutOff),
			},
		}
		toPut = append(toPut, eventToPut)
	}

	return &SubmittedBotInfoUpdate{
		BotInfo:          current,
		BotEvent:         eventToPut,
		AbandonedTaskID:  abandonedTaskID,
		entitiesToPut:    toPut,
		entitiesToDelete: toDelete,
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
func (u *BotInfoUpdate) eventMessage(extra []string) string {
	// Note: we pick one "most interesting" message here instead of joining them
	// all together because very often they are all the same or convey the same
	// information.
	var messages []string
	switch {
	case u.EventMessage != "":
		messages = append(messages, u.EventMessage)
	case u.HealthInfo != nil && u.HealthInfo.Maintenance != "":
		messages = append(messages, u.HealthInfo.Maintenance)
	case u.HealthInfo != nil && u.HealthInfo.Quarantined != "":
		messages = append(messages, u.HealthInfo.Quarantined)
	}
	return strings.Join(append(messages, extra...), "\n")
}

// reportBotInfoTxn updates botInfoTxnCount metric.
//
// Also logs errors or excessive retries.
func (u *BotInfoUpdate) reportBotInfoTxn(ctx context.Context, attempt int, err error) {
	var outcome string
	switch {
	case err == nil:
		outcome = "ok"
	case errors.Is(err, datastore.ErrConcurrentTransaction):
		outcome = "conflict"
	case errors.Is(err, context.DeadlineExceeded):
		outcome = "timeout"
	case errors.Is(err, context.Canceled):
		outcome = "canceled"
	case errors.Is(err, errSkippedUpdate):
		outcome = "skipped"
	default:
		outcome = "other"
	}
	botInfoTxnCount.Add(ctx, 1, string(u.EventType), outcome, min(attempt, maxTxnAttempts))
	if err != nil {
		if !errors.Is(err, errSkippedUpdate) {
			logging.Errorf(ctx, "Failed to submit %s after %d attempt(s): %s", u.EventType, attempt, err)
		}
	} else if attempt > 3 {
		logging.Warningf(ctx, "Submitted %s after %d attempt(s)", u.EventType, attempt)
	}
}
