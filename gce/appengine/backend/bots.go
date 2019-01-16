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

package backend

import (
	"context"
	"net/http"
	"time"

	"github.com/golang/protobuf/proto"

	"google.golang.org/api/googleapi"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/gce/api/tasks/v1"
)

// manageAction is a possible action to take as a result of manageBot.
type manageAction int

const (
	// none means no action should be taken.
	none manageAction = iota
	// term means the Swarming bot should be terminated.
	term
	// dest means the GCE instance should be destroyed.
	dest
)

// manageBotQueue is the name of the manage bot task handler queue.
const manageBotQueue = "manage-bot"

// manageBot manages an existing Swarming bot.
func manageBot(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.ManageBot)
	switch {
	case !ok:
		return errors.Reason("unexpected payload %q", payload).Err()
	case task.GetId() == "":
		return errors.Reason("ID is required").Err()
	}
	vm, err := getVM(c, task.Id)
	if err != nil {
		return err
	}
	if vm.URL == "" {
		return errors.Reason("instance does not exist: %s", vm.URL).Err()
	}
	logging.Debugf(c, "fetching bot %q: %s", vm.Hostname, vm.Swarming)
	srv := getSwarming(c, vm.Swarming)
	rsp, err := srv.Bot.Get(vm.Hostname).Context(c).Do()
	if err != nil {
		if gerr, ok := err.(*googleapi.Error); ok {
			logErrors(c, gerr)
			if gerr.Code == http.StatusNotFound {
				// Bot hasn't connected to Swarming yet.
				// TODO(smut): Delete the GCE instance if it's been too long.
				logging.Debugf(c, "bot not found")
				return nil
			}
		}
		return errors.Annotate(err, "failed to fetch bot").Err()
	}
	logging.Debugf(c, "found bot")
	// In general, to replace a GCE instance first terminate the Swarming bot, then destroy the
	// GCE instance, then delete the Swarming bot. The Swarming bot must be terminated first so
	// that the GCE instance is never destroyed while executing Swarming workload. However, if
	// the Swarming bot died or was deleted by some external factor, we can skip the termination
	// step because we already know the GCE instance can't execute Swarming workload anymore.
	act := none
	switch {
	case rsp.Deleted:
		// TODO(smut): Delete the Swarming bot.
		logging.Debugf(c, "bot deleted")
		act = dest
	case rsp.IsDead:
		// TODO(smut): Delete the Swarming bot.
		logging.Debugf(c, "bot dead")
		act = dest
	case vm.Deadline > 0 && vm.Deadline < time.Now().Unix():
		logging.Debugf(c, "deadline %d exceeded", vm.Deadline)
		act = term
	}
	switch act {
	case term:
		// TODO(smut): Once the bot is terminated, destroy the instance.
		t := &tq.Task{
			Payload: &tasks.TerminateBot{
				Id:       task.Id,
				Hostname: vm.Hostname,
			},
		}
		if err := getDispatcher(c).AddTask(c, t); err != nil {
			return errors.Annotate(err, "failed to schedule terminate tasks").Err()
		}
		return nil
	case dest:
		t := &tq.Task{
			Payload: &tasks.DestroyInstance{
				Id:  task.Id,
				Url: vm.URL,
			},
		}
		if err := getDispatcher(c).AddTask(c, t); err != nil {
			return errors.Annotate(err, "failed to schedule destroy tasks").Err()
		}
		return nil
	default:
		return nil
	}
}

// terminateBotQueue is the name of the terminate bot task handler queue.
const terminateBotQueue = "terminate-bot"

// terminateBot terminates an existing Swarming bot.
func terminateBot(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.TerminateBot)
	switch {
	case !ok:
		return errors.Reason("unexpected payload %q", payload).Err()
	case task.GetId() == "":
		return errors.Reason("ID is required").Err()
	case task.GetHostname() == "":
		return errors.Reason("hostname is required").Err()
	}
	vm, err := getVM(c, task.Id)
	switch {
	case err != nil:
		return errors.Annotate(err, "failed to fetch VM").Err()
	case vm.Hostname != task.Hostname:
		// Instance is already destroyed and replaced. Don't delete the new bot.
		return errors.Reason("bot %q does not exist", task.Hostname).Err()
	}
	logging.Debugf(c, "terminating bot %q: %s", vm.Hostname, vm.Swarming)
	srv := getSwarming(c, vm.Swarming)
	_, err = srv.Bot.Terminate(vm.Hostname).Context(c).Do()
	if err != nil {
		if gerr, ok := err.(*googleapi.Error); ok {
			logErrors(c, gerr)
			if gerr.Code == http.StatusNotFound {
				// Bot is already deleted.
				logging.Debugf(c, "bot not found")
				return nil
			}
		}
		return errors.Annotate(err, "failed to terminate bot").Err()
	}
	return nil
}
