// Copyright 2019 The LUCI Authors.
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

package backend

import (
	"context"
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/memcache"
	swarmingpb "go.chromium.org/luci/swarming/proto/api_v2"

	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/backend/internal/metrics"
	"go.chromium.org/luci/gce/appengine/model"
)

// utcRFC3339 is the timestamp format used by Swarming.
// Similar to RFC3339 but with an implicit UTC time zone.
const utcRFC3339 = "2006-01-02T15:04:05"

// botListLimit is the maximum number of bots to list per query. It should be <= 1000.
const botListLimit = 500

// deleteStaleSwarmingBotBatchSize is set to 2 because it was found that on average the
// deleteStaleSwarmingBot function took about 300ms to run and the queues are limited at
// 500/s. This means we still have time left over in every run to process another bot.
// It is possible to try with 3 bots, But might be a little too close and these numbers
// are averages.
const deleteStaleSwarmingBotBatchSize = 2

// logGrpcError logs an error response from Swarming.
func logGrpcError(c context.Context, name string, err error) {
	logging.Errorf(c, "failure for %s: %s", name, err)
}

// setConnected sets the Swarming bot as connected in the datastore if it isn't already.
func setConnected(c context.Context, id, hostname string, at time.Time) error {
	// Check dscache in case vm.Connected is set now, to avoid a costly transaction.
	vm := &model.VM{
		ID: id,
	}
	switch err := datastore.Get(c, vm); {
	case err != nil:
		return errors.Fmt("failed to fetch VM with id: %q: %w", id, err)
	case vm.Hostname != hostname:
		return errors.Fmt("bot %q does not exist", hostname)
	case vm.Connected > 0:
		return nil
	}
	put := false
	err := datastore.RunInTransaction(c, func(c context.Context) error {
		put = false
		switch err := datastore.Get(c, vm); {
		case err != nil:
			return errors.Fmt("failed to fetch VM with id: %q: %w", id, err)
		case vm.Hostname != hostname:
			return errors.Fmt("bot %q does not exist", hostname)
		case vm.Connected > 0:
			return nil
		}
		vm.Connected = at.Unix()
		if err := datastore.Put(c, vm); err != nil {
			return errors.Fmt("failed to store VM: %w", err)
		}
		put = true
		return nil
	}, nil)
	if put && err == nil {
		metrics.ReportConnectionTime(c, float64(vm.Connected-vm.Created), vm.Prefix, vm.Attributes.GetProject(), vm.Swarming, vm.Attributes.GetZone())
		metrics.ReportBotConnectionTime(c, float64(vm.Connected-vm.ConfigExpanded), vm.Prefix, vm.Attributes.GetProject(), vm.Attributes.GetLabel()["resource_group"], vm.Swarming, vm.Attributes.GetZone())
	}
	return err
}

// minPendingForBotConnected is the minimal minutes (10 minutes) to wait for the swarming bot in the VM to connect to Swarming.
const minPendingForBotConnected = 10 * time.Minute

// manageMissingBot manages a missing Swarming bot.
func manageMissingBot(c context.Context, vm *model.VM) error {
	// Set that the bot has not yet connected to Swarming.
	switch {
	// the case that the VM is drained is not handled here due to b/264921632, that some newly created VM
	// can be set to drained immidiately, but hasn't been connected to Swarming. If we destroy it here, there's
	// a small chance that the bot is up during the destroyment and pick up a test but fail it.
	case vm.Lifetime > 0 && vm.Created+vm.Lifetime < time.Now().Unix():
		logging.Debugf(c, "deadline %d exceeded", vm.Created+vm.Lifetime)
		return destroyInstanceAsync(c, vm.ID, vm.URL)
	case vm.Timeout > 0 && vm.Created+vm.Timeout < time.Now().Unix():
		logging.Debugf(c, "timeout %d exceeded", vm.Created+vm.Timeout)
		return destroyInstanceAsync(c, vm.ID, vm.URL)
	default:
		return nil
	}
}

// manageBotQueue is the name of the manage bot task handler queue.
const manageBotQueue = "manage-bot"

// manageBot manages a Swarming bot.
func manageBot(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.ManageBot)
	switch {
	case !ok:
		return errors.Fmt("unexpected payload %q", payload)
	case task.GetId() == "":
		return errors.New("ID is required")
	}
	vm := &model.VM{
		ID: task.Id,
	}
	switch err := datastore.Get(c, vm); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil
	case err != nil:
		return errors.Fmt("failed to fetch VM with id: %q: %w", task.GetId(), err)
	case vm.URL == "":
		logging.Debugf(c, "instance %q does not exist", vm.Hostname)
		return nil
	}

	logging.Debugf(c, "fetching bot %q: %s", vm.Hostname, vm.Swarming)
	bot, err := getSwarming(c, vm.Swarming).GetBot(c, &swarmingpb.BotRequest{
		BotId: vm.Hostname,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			logging.Debugf(c, "bot not found (%s)", vm.Hostname)
			return manageMissingBot(c, vm)
		}
		logGrpcError(c, vm.Hostname, err)
		return errors.Fmt("failed to fetch bot: %w", err)
	}
	if isDestroyable(c, bot, vm) {
		return destroyInstanceAsync(c, vm.ID, vm.URL)
	}
	return nil
}

// inspectSwarmingAsync collects all the swarming servers and schedules a task for each of them
func inspectSwarmingAsync(c context.Context) error {
	// Collect all the swarming instances
	swarmings := stringset.New(10)
	qC := datastore.NewQuery("Config")
	if err := datastore.Run(c, qC, func(cfg *model.Config) {
		swarmings.Add(cfg.Config.Swarming)
	}); err != nil {
		return errors.Fmt("inspectSwarmingAsync: Failed to query configs: %w", err)
	}
	// Generate all the inspectSwarmingTasks
	var inspectSwarmingTasks []*tq.Task
	swarmings.Iter(func(sw string) bool {
		inspectSwarmingTasks = append(inspectSwarmingTasks, &tq.Task{
			Payload: &tasks.InspectSwarming{
				Swarming: sw,
			},
		})
		return true
	})
	// schedule all the inspect swarming tasks
	if len(inspectSwarmingTasks) > 0 {
		if err := getDispatcher(c).AddTask(c, inspectSwarmingTasks...); err != nil {
			return errors.Fmt("inspectSwarmingAsync: failed to schedule task: %w", err)
		}
	}
	return nil
}

// inspectSwarmingQueue is the name of the inspect swarming task queue
const inspectSwarmingQueue = "inspect-swarming"

// inspectSwarming is the task queue handler for inspect swarming queue
func inspectSwarming(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.InspectSwarming)
	switch {
	case !ok:
		return errors.Fmt("InspectSwarming: unexpected payload %q", payload)
	case task.GetSwarming() == "":
		return errors.New("InspectSwarming: swarming is required")
	}
	var inpectSwarmingSubtasks []*tq.Task
	listRPCResp, err := getSwarming(c, task.GetSwarming()).ListBots(c, &swarmingpb.BotsRequest{
		Limit:  botListLimit,
		Cursor: task.Cursor,
	})
	if err != nil {
		return errors.Fmt("InspectSwarming: failed to List instances from swarming: %w", err)
	}
	// Schedule the new task with the new cursor
	if listRPCResp.Cursor != "" {
		task.Cursor = listRPCResp.Cursor
		if err := getDispatcher(c).AddTask(c, &tq.Task{Payload: task}); err != nil {
			// Log error and process the bots
			logging.Errorf(c, "InspectSwarming: failed to schedule inspect swarming task. %v", err)
		}
	}
	batchPayload := &tasks.DeleteStaleSwarmingBots{}
	for _, bot := range listRPCResp.Items {
		qV := datastore.NewQuery("VM").Eq("hostname", bot.BotId)
		if err := datastore.Run(c, qV, func(vm *model.VM) {
			if isDestroyable(c, bot, vm) {
				inpectSwarmingSubtasks = append(inpectSwarmingSubtasks, &tq.Task{
					Payload: &tasks.DestroyInstance{
						Id:  vm.ID,
						Url: vm.URL,
					},
				})
			} else {
				batchPayload.Bots = append(batchPayload.Bots, &tasks.DeleteStaleSwarmingBot{
					Id:          vm.ID,
					FirstSeenTs: bot.FirstSeenTs.AsTime().UTC().Format(utcRFC3339),
				})
				if len(batchPayload.GetBots()) == deleteStaleSwarmingBotBatchSize {
					// Schedule a task to check and delete the bot if needed
					inpectSwarmingSubtasks = append(inpectSwarmingSubtasks, &tq.Task{
						Payload: batchPayload,
					})
					batchPayload = &tasks.DeleteStaleSwarmingBots{}
				}
			}
		}); err != nil {
			logging.Debugf(c, "bot %s does not exist in datastore?", bot.BotId)
		}
	}
	if len(batchPayload.GetBots()) > 0 {
		// Schedule a task to check and delete the bot if needed
		inpectSwarmingSubtasks = append(inpectSwarmingSubtasks, &tq.Task{
			Payload: batchPayload,
		})
	}
	// Dispatch all the tasks
	if len(inpectSwarmingSubtasks) > 0 {
		if err := getDispatcher(c).AddTask(c, inpectSwarmingSubtasks...); err != nil {
			return errors.Fmt("InspectSwarming: failed to schedule sub task(s): %w", err)
		}
	}
	return nil
}

// terminateBotAsync schedules a task queue task to terminate a Swarming bot.
func terminateBotAsync(c context.Context, id, hostname string) error {
	t := &tq.Task{
		Payload: &tasks.TerminateBot{
			Id:       id,
			Hostname: hostname,
		},
	}
	if err := getDispatcher(c).AddTask(c, t); err != nil {
		return errors.Fmt("failed to schedule terminate task: %w", err)
	}
	return nil
}

// terminateBotQueue is the name of the terminate bot task handler queue.
const terminateBotQueue = "terminate-bot"

// terminateBot terminates an existing Swarming bot.
func terminateBot(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.TerminateBot)
	switch {
	case !ok:
		return errors.Fmt("unexpected payload %q", payload)
	case task.GetId() == "":
		return errors.New("ID is required")
	case task.GetHostname() == "":
		return errors.New("hostname is required")
	}
	vm := &model.VM{
		ID: task.Id,
	}
	switch err := datastore.Get(c, vm); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil
	case err != nil:
		return errors.Fmt("failed to fetch VM with id: %q: %w", task.GetId(), err)
	case vm.Hostname != task.Hostname:
		// Instance is already destroyed and replaced. Don't terminate the new bot.
		logging.Debugf(c, "Terminate bot %q: vm does not exist. Skipping...", task.Hostname)
		return nil
	}
	// 1 terminate task should normally suffice to terminate a bot, but in case
	// users or bugs interfere, resend terminate task after 1 hour. Even if
	// memcache content is cleared, more frequent terminate requests can now be
	// handled by the swarming server and after crbug/982840 will be auto-deduped.
	mi := memcache.NewItem(c, fmt.Sprintf("terminate-%s/%s", vm.Swarming, vm.Hostname))
	if err := memcache.Get(c, mi); err == nil {
		logging.Debugf(c, "Terminate bot %q: already has a terminate task from us", task.Hostname)
		return nil
	}
	logging.Debugf(c, "Terminate bot %q: target swarming %q", vm.Hostname, vm.Swarming)
	_, err := getSwarming(c, vm.Swarming).TerminateBot(c, &swarmingpb.TerminateRequest{
		BotId:  vm.Hostname,
		Reason: "GCE Provider",
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// Bot is already deleted.
			logging.Debugf(c, "Terminate bot %q: bot not found in swarming. We are good to skip...", vm.Hostname)
			return nil
		}
		logGrpcError(c, vm.Hostname, err)
		return errors.Fmt("terminate bot %q: %w", vm.Hostname, err)
	}
	if err := memcache.Set(c, mi.SetExpiration(time.Hour)); err != nil {
		logging.Warningf(c, "Terminate bot %q: failed to record task in memcache: %s", vm.Hostname, err)
	}
	return nil
}

// deleteBotAsync schedules a task queue task to delete a Swarming bot.
func deleteBotAsync(c context.Context, id, hostname string) error {
	t := &tq.Task{
		Payload: &tasks.DeleteBot{
			Id:       id,
			Hostname: hostname,
		},
	}
	if err := getDispatcher(c).AddTask(c, t); err != nil {
		return errors.Fmt("Destroy instance %q: failed to schedule delete bot task.: %w", hostname, err)
	}
	logging.Debugf(c, "Destroy instance %q: scheduled task to delete bot.", hostname)
	return nil
}

// deleteBotQueue is the name of the delete bot task handler queue.
const deleteBotQueue = "delete-bot"

// deleteBot deletes an existing Swarming bot.
func deleteBot(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.DeleteBot)
	switch {
	case !ok:
		return errors.Fmt("delete bot: unexpected payload %q", payload)
	case task.GetId() == "":
		return errors.New("delete bot:ID is required")
	case task.GetHostname() == "":
		return errors.New("delete bot: hostname is required")
	}
	vm := &model.VM{
		ID: task.Id,
	}
	switch err := datastore.Get(c, vm); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil
	case err != nil:
		return errors.Fmt("delete bot: failed to fetch VM with id: %q: %w", task.GetId(), err)
	case vm.Hostname != task.Hostname:
		// Instance is already destroyed and replaced. Don't delete the new bot.
		return errors.Fmt("delete bot %q: does not exist", task.Hostname)
	}
	logging.Debugf(c, "Delete bot %q: on swarming %s.", vm.Hostname, vm.Swarming)
	_, err := getSwarming(c, vm.Swarming).DeleteBot(c, &swarmingpb.BotRequest{
		BotId: vm.Hostname,
	})
	if err != nil {
		if status.Code(err) == codes.NotFound {
			// Bot is already deleted.
			logging.Debugf(c, "Delete bot %q: not found in swarming %s.", vm.Hostname, vm.Swarming)
			return deleteVM(c, task.Id, vm.Hostname)
		}
		logGrpcError(c, vm.Hostname, err)
		return errors.Fmt("failed to delete bot: %w", err)
	}
	logging.Debugf(c, "Delete bot %q: done!", vm.Hostname)
	return deleteVM(c, task.Id, vm.Hostname)
}

// deleteVM deletes the given VM from the datastore if it exists.
func deleteVM(c context.Context, id, hostname string) error {
	vm := &model.VM{
		ID: id,
	}
	logging.Debugf(c, "Delete VM record %q: starting...", hostname)
	return datastore.RunInTransaction(c, func(c context.Context) error {
		switch err := datastore.Get(c, vm); {
		case errors.Is(err, datastore.ErrNoSuchEntity):
			return nil
		case err != nil:
			return errors.Fmt("delete VM %q: failed to fetch VM with id: %q: %w", hostname, id, err)
		case vm.Hostname != hostname:
			logging.Debugf(c, "Delete VM %q: not found.", hostname)
			return nil
		}
		logging.Debugf(c, "Delete VM %q: VM found and ready to delete.", hostname)
		if err := datastore.Delete(c, vm); err != nil {
			return errors.Fmt("failed to delete VM: %w", err)
		}
		logging.Debugf(c, "Deleted VM %q: done!", hostname)
		return nil
	}, nil)
}

// deleteStaleSwarmingBotsQueue is the name of the queue that deletes stale swarming bots in batches
const deleteStaleSwarmingBotsQueue = "delete-stale-swarming-bots"

func deleteStaleSwarmingBots(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.DeleteStaleSwarmingBots)
	switch {
	case !ok:
		return errors.Fmt("deleteStaleSwarmingBots: unexpected payload %q", payload)
	case task.GetBots() == nil:
		return errors.New("deleteStaleSwarmingBots: No Bots to process")
	}
	var errs []error
	for _, dssb := range task.GetBots() {
		if err := deleteStaleSwarmingBot(c, dssb); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.NewMultiError(errs...).AsError()
	}
	return nil
}

// deleteStaleSwarmingBot manages the existing swarming bot
func deleteStaleSwarmingBot(c context.Context, task *tasks.DeleteStaleSwarmingBot) error {
	switch {
	case task == nil:
		return errors.New("deleteStaleSwarmingBot: Bad input?")
	case task.GetId() == "":
		return errors.New("deleteStaleSwarmingBot: Missing bot id")
	case task.GetFirstSeenTs() == "":
		return errors.New("deleteStaleSwarmingBot: Missing timestamp")
	}
	vm := &model.VM{
		ID: task.GetId(),
	}
	switch err := datastore.Get(c, vm); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil
	case err != nil:
		return errors.Fmt("deleteStaleSwarmingBot: failed to fetch VM %s: %w", task.GetId(), err)
	case vm.URL == "":
		logging.Debugf(c, "deleteStaleSwarmingBot: instance %q does not exist", vm.Hostname)
		return nil
	}
	// bot_terminate occurs when the bot starts the termination task and is normally followed
	// by task_completed and bot_shutdown. Responses also include the full set of dimensions
	// when the event was recorded. Limit response size by fetching only recent events, and only
	// the type of each.
	// A VM may be recreated multiple times with the same name, so it is important to only
	// query swarming for events since the current VM's creation.
	events, err := getSwarming(c, vm.Swarming).ListBotEvents(c, &swarmingpb.BotEventsRequest{
		BotId: vm.Hostname,
		Limit: 5,
		Start: timestamppb.New(time.Unix(vm.Created, 0)),
	})
	if err != nil {
		logGrpcError(c, vm.Hostname, err)
		return errors.Fmt("deleteStaleSwarmingBot: failed to fetch bot events for %s: %w", vm.Hostname, err)
	}
	for _, e := range events.Items {
		if e.EventType == "bot_terminate" {
			logging.Debugf(c, "deleteStaleSwarmingBot: bot terminated (%s)", vm.Hostname)
			return destroyInstanceAsync(c, vm.ID, vm.URL)
		}
	}
	switch {
	case vm.Lifetime > 0 && vm.Created+vm.Lifetime < time.Now().Unix():
		logging.Debugf(c, "deleteStaleSwarmingBot: %s deadline %d exceeded", vm.ID, vm.Created+vm.Lifetime)
		return terminateBotAsync(c, vm.ID, vm.Hostname)
	case vm.Drained:
		logging.Debugf(c, "deleteStaleSwarmingBot: VM %s drained", vm.ID)
		return terminateBotAsync(c, vm.ID, vm.Hostname)
	}
	// This value of vm.Connected may be several seconds old, because the VM was fetched
	// prior to sending an RPC to Swarming. Still, check it here to save a costly operation
	// in setConnected, since DeleteStaleSwarmingBot may be called thousands of times per minute.
	if vm.Connected == 0 {
		t, err := time.Parse(utcRFC3339, task.GetFirstSeenTs())
		if err != nil {
			return errors.Fmt("deleteStaleSwarmingBot: %s failed to parse bot connection time: %w", vm.ID, err)
		}
		if err := setConnected(c, vm.ID, vm.Hostname, t); err != nil {
			return errors.Fmt("deleteStaleSwarmingBot: %s failed to set connected time: %w", vm.ID, err)
		}
	}
	return nil
}

// isDestroyable returns true if the given instance can be destroyed.
// A given instance can be destroyed if
//  1. Bot is dead and has a termination_task_id
//  2. Bot is dead and was last seen more than vm.Timeout time ago
//
// An exception to the above logic is on VM first boot. The VM gets at
// least 10 minute timeout (irrespective of the vm config timeout)
func isDestroyable(ctx context.Context, bot *swarmingpb.BotInfo, vm *model.VM) bool {
	// botIsDeeeead represents that the bot is in dead state and was last seen more than `vm.Timeout` seconds ago
	botIsDeeeead := bot.IsDead && time.Since(bot.LastSeenTs.AsTime()) > time.Duration(vm.Timeout*int64(time.Second))
	// botIsTerminated is set if it has been terminated in swarming
	botIsTerminated := bot.IsDead && bot.GetTerminationTaskId() != ""
	if botIsDeeeead || bot.Deleted || botIsTerminated {
		// If the bot is dead or deleted, schedule a task to destroy the instance
		logging.Debugf(ctx, "isDestroyable: bot %s is_dead[%v] deleted[%v] terminated[%v]  time since last_seen[%s] timeout[%v]. Destroying instance", bot.BotId, bot.IsDead, bot.Deleted, bot.TerminationTaskId, time.Since(bot.LastSeenTs.AsTime()).String(), vm.Timeout)
		// A bot may be returned as deleted or dead if a bot with the same ID was previously connected to Swarming, but this new VM's bot hasn't connected yet
		if time.Since(time.Unix(vm.Created, 0)) <= minPendingForBotConnected {
			logging.Debugf(ctx, "isDestroyable: bot %s is newly created, wait for %s minutes at least to destroy", vm.Hostname, minPendingForBotConnected.Minutes())
			return false
		}
		return true
	}
	return false
}
