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
	"fmt"
	"net/http"

	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"

	"google.golang.org/api/googleapi"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"
)

// getSuffix returns a random suffix to use when naming a GCE instance.
func getSuffix(c context.Context) string {
	const allowed = "abcdefghijklmnopqrstuvwxyz0123456789"
	suf := make([]byte, 4)
	for i := range suf {
		suf[i] = allowed[mathrand.Intn(c, len(allowed))]
	}
	return string(suf)
}

// getVM returns a VM entity from the datastore. Hostname is guaranteed to be set.
func getVM(c context.Context, id string) (*model.VM, error) {
	vm := &model.VM{
		ID: id,
	}
	if err := datastore.Get(c, vm); err != nil {
		return nil, errors.Annotate(err, "failed to fetch VM").Err()
	}
	if vm.Hostname != "" {
		return vm, nil
	}
	// Generate a new hostname and record it so future calls are idempotent.
	hostname := fmt.Sprintf("%s-%s", vm.ID, getSuffix(c))
	if err := datastore.RunInTransaction(c, func(c context.Context) error {
		if err := datastore.Get(c, vm); err != nil {
			return errors.Annotate(err, "failed to fetch VM").Err()
		}
		// Double-check inside transaction. Hostname may already be generated.
		if vm.Hostname != "" {
			return nil
		}
		vm.Hostname = hostname
		if err := datastore.Put(c, vm); err != nil {
			return errors.Annotate(err, "failed to store VM").Err()
		}
		return nil
	}, nil); err != nil {
		return nil, err
	}
	return vm, nil
}

// resetVM resets a VM by removing its hostname from the datastore.
func resetVM(c context.Context, id string) error {
	vm := &model.VM{
		ID: id,
	}
	if err := datastore.Get(c, vm); err != nil {
		return errors.Annotate(err, "failed to fetch VM").Err()
	}
	if vm.Hostname == "" {
		return nil
	}
	return datastore.RunInTransaction(c, func(c context.Context) error {
		if err := datastore.Get(c, vm); err != nil {
			return errors.Annotate(err, "failed to fetch VM").Err()
		}
		if vm.Hostname == "" {
			return nil
		}
		vm.Hostname = ""
		if err := datastore.Put(c, vm); err != nil {
			return errors.Annotate(err, "failed to store VM").Err()
		}
		return nil
	}, nil)
}

// setURL sets the VM's URL in the datastore if it isn't already.
func setURL(c context.Context, id, url string) error {
	vm := &model.VM{
		ID: id,
	}
	if err := datastore.RunInTransaction(c, func(c context.Context) error {
		if err := datastore.Get(c, vm); err != nil {
			return errors.Annotate(err, "failed to fetch VM").Err()
		}
		if vm.URL == "" {
			vm.URL = url
			if err := datastore.Put(c, vm); err != nil {
				return errors.Annotate(err, "failed to store VM").Err()
			}
		}
		return nil
	}, nil); err != nil {
		return err
	}
	return nil
}

// logErrors logs the errors in the given *googleapi.Error.
func logErrors(c context.Context, err *googleapi.Error) {
	for _, err := range err.Errors {
		logging.Errorf(c, "%s", err.Message)
	}
}

// createQueue is the name of the create task handler queue.
const createQueue = "create-instance"

// create creates a GCE instance.
func create(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.Create)
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
	if vm.URL != "" {
		logging.Debugf(c, "instance exists: %s", vm.URL)
		return nil
	}
	logging.Debugf(c, "creating instance %q", vm.Hostname)
	// Generate a request ID based on the hostname.
	// Ensures duplicate operations aren't created in GCE.
	rID := uuid.NewSHA1(uuid.Nil, []byte(vm.Hostname))
	srv := getCompute(c).Instances
	call := srv.Insert(vm.Attributes.GetProject(), vm.Attributes.GetZone(), vm.GetInstance())
	op, err := call.RequestId(rID.String()).Context(c).Do()
	if err != nil {
		gerr := err.(*googleapi.Error)
		logErrors(c, gerr)
		if gerr.Code == http.StatusConflict {
			// Conflict with an existing instance. Conflicts arise from name collisions.
			// Hostnames are required to be unique per project. Either this instance already
			// exists, or a same-named instance exists in a different zone. Figure out which.
			call := srv.Get(vm.Attributes.GetProject(), vm.Attributes.GetZone(), vm.Hostname)
			inst, err := call.Context(c).Do()
			if err != nil {
				if gerr, ok := err.(*googleapi.Error); ok {
					logErrors(c, gerr)
					if gerr.Code == http.StatusNotFound {
						// Instance doesn't exist in this zone.
						if err := resetVM(c, task.Id); err != nil {
							return errors.Annotate(err, "instance exists in another zone").Err()
						}
						return errors.Reason("instance exists in another zone").Err()
					}
				}
				return errors.Annotate(err, "failed to fetch instance").Err()
			}
			// Instance exists in this zone.
			logging.Debugf(c, "instance exists: %s", inst.SelfLink)
			return setURL(c, task.Id, inst.SelfLink)
		}
		if err := resetVM(c, task.Id); err != nil {
			return errors.Annotate(err, "failed to create instance").Err()
		}
		return errors.Reason("failed to create instance").Err()
	}
	if op.Error != nil && len(op.Error.Errors) > 0 {
		for _, err := range op.Error.Errors {
			logging.Errorf(c, "%s: %s", err.Code, err.Message)
		}
		if err := resetVM(c, task.Id); err != nil {
			return errors.Annotate(err, "failed to create instance").Err()
		}
		return errors.Reason("failed to create instance").Err()
	}
	if op.Status == "DONE" {
		logging.Debugf(c, "created instance: %s", op.TargetLink)
		return setURL(c, task.Id, op.TargetLink)
	}
	return nil
}

// manageQueue is the name of the manage task handler queue.
const manageQueue = "manage-instance"

// manage manages a created GCE instance.
func manage(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.Manage)
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
		return errors.Reason("instance %q does not exist", vm.URL).Err()
	}
	logging.Debugf(c, "fetching bot %q: %s", vm.Hostname, vm.Swarming)
	srv := getSwarming(c)
	srv.BasePath = vm.Swarming + "/_ah/api/swarming/v1/"
	rsp, err := srv.Bot.Get(vm.Hostname).Context(c).Do()
	if err != nil {
		if gerr, ok := err.(*googleapi.Error); ok {
			logErrors(c, gerr)
			if gerr.Code == http.StatusNotFound {
				// Bot hasn't connected to Swarming yet.
				// TODO(smut): Delete the instance if it's been too long.
				logging.Debugf(c, "bot not found")
				return nil
			}
		}
		return errors.Annotate(err, "failed to fetch bot").Err()
	}
	logging.Debugf(c, "found bot")
	switch {
	case rsp.Deleted:
		// TODO(smut): Delete the instance.
		logging.Debugf(c, "bot deleted")
	case rsp.IsDead:
		// TODO(smut): Delete the bot and the instance.
		logging.Debugf(c, "bot dead")
	}
	return nil
}
