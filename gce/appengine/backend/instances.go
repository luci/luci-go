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
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"

	"google.golang.org/api/googleapi"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/tq"
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

// getVM returns a VM from the datastore. Hostname is guaranteed to be set.
func getVM(c context.Context, id string) (*model.VM, error) {
	vm := &model.VM{
		ID: id,
	}
	if err := datastore.Get(c, vm); err != nil {
		return nil, errors.Annotate(err, "failed to fetch VM").Err()
	}
	switch {
	case vm.Hostname != "":
		return vm, nil
	case vm.Drained:
		// If the VM is drained, don't generate a new name.
		return nil, errors.Reason("VM drained").Err()
	}
	// Generate a new hostname and record it so future calls are idempotent.
	hostname := fmt.Sprintf("%s-%s", vm.ID, getSuffix(c))
	if err := datastore.RunInTransaction(c, func(c context.Context) error {
		if err := datastore.Get(c, vm); err != nil {
			return errors.Annotate(err, "failed to fetch VM").Err()
		}
		// Double-check inside transaction.
		// Hostname may already be generated or the VM may be drained.
		switch {
		case vm.Hostname != "":
			return nil
		case vm.Drained:
			return errors.Reason("VM drained").Err()
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

// setCreated sets the GCE instance as created in the datastore if it isn't already.
func setCreated(c context.Context, id, url string, at time.Time) error {
	vm := &model.VM{
		ID: id,
	}
	return datastore.RunInTransaction(c, func(c context.Context) error {
		if err := datastore.Get(c, vm); err != nil {
			return errors.Annotate(err, "failed to fetch VM").Err()
		}
		if vm.URL != "" {
			// Already created.
			return nil
		}
		vm.Deadline = at.Unix() + vm.Lifetime
		vm.URL = url
		if err := datastore.Put(c, vm); err != nil {
			return errors.Annotate(err, "failed to store VM").Err()
		}
		return nil
	}, nil)
}

// logErrors logs the errors in the given *googleapi.Error.
func logErrors(c context.Context, err *googleapi.Error) {
	for _, err := range err.Errors {
		logging.Errorf(c, "%s", err.Message)
	}
}

// createInstanceQueue is the name of the create instance task handler queue.
const createInstanceQueue = "create-instance"

// createInstance creates a GCE instance.
func createInstance(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.CreateInstance)
	switch {
	case !ok:
		return errors.Reason("unexpected payload %q", payload).Err()
	case task.GetId() == "":
		return errors.Reason("ID is required").Err()
	}
	vm, err := getVM(c, task.Id)
	switch {
	case err != nil:
		return err
	case vm.URL != "":
		logging.Debugf(c, "instance exists: %s", vm.URL)
		return nil
	}
	logging.Debugf(c, "creating instance %q", vm.Hostname)
	// Generate a request ID based on the hostname.
	// Ensures duplicate operations aren't created in GCE.
	rID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("create-%s", vm.Hostname)))
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
			t, err := time.Parse(time.RFC3339, inst.CreationTimestamp)
			if err != nil {
				return errors.Annotate(err, "failed to parse instance creation time").Err()
			}
			return setCreated(c, task.Id, inst.SelfLink, t)
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
		t, err := time.Parse(time.RFC3339, op.EndTime)
		if err != nil {
			return errors.Annotate(err, "failed to parse instance creation time").Err()
		}
		return setCreated(c, task.Id, op.TargetLink, t)
	}
	// Instance creation is pending.
	return nil
}

// destroyInstanceAsync schedules a task queue task to destroy a GCE instance.
func destroyInstanceAsync(c context.Context, id, url string) error {
	t := &tq.Task{
		Payload: &tasks.DestroyInstance{
			Id:  id,
			Url: url,
		},
	}
	if err := getDispatcher(c).AddTask(c, t); err != nil {
		return errors.Annotate(err, "failed to schedule destroy task").Err()
	}
	return nil
}

// destroyInstanceQueue is the name of the destroy instance task handler queue.
const destroyInstanceQueue = "destroy-instance"

// destroyInstance destroys a GCE instance.
func destroyInstance(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.DestroyInstance)
	switch {
	case !ok:
		return errors.Reason("unexpected payload type %T", payload).Err()
	case task.GetId() == "":
		return errors.Reason("ID is required").Err()
	case task.GetUrl() == "":
		return errors.Reason("URL is required").Err()
	}
	vm, err := getVM(c, task.Id)
	switch {
	case err != nil:
		return nil
	case vm.URL != task.Url:
		// Instance is already destroyed and replaced. Don't destroy the new one.
		return errors.Reason("instance does not exist: %s", task.Url).Err()
	}
	logging.Debugf(c, "destroying instance %q", vm.Hostname)
	// Generate a request ID based on the hostname.
	// Ensures duplicate operations aren't created in GCE.
	rID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("destroy-%s", vm.Hostname)))
	srv := getCompute(c).Instances
	call := srv.Delete(vm.Attributes.GetProject(), vm.Attributes.GetZone(), vm.Hostname)
	op, err := call.RequestId(rID.String()).Context(c).Do()
	if err != nil {
		gerr := err.(*googleapi.Error)
		logErrors(c, gerr)
		if gerr.Code == http.StatusNotFound {
			// Instance is already destroyed.
			logging.Debugf(c, "instance does not exist: %s", vm.URL)
			return deleteBotAsync(c, task.Id, vm.Hostname)
		}
		return errors.Reason("failed to destroy instance").Err()
	}
	if op.Error != nil && len(op.Error.Errors) > 0 {
		for _, err := range op.Error.Errors {
			logging.Errorf(c, "%s: %s", err.Code, err.Message)
		}
		return errors.Reason("failed to destroy instance").Err()
	}
	if op.Status == "DONE" {
		logging.Debugf(c, "destroyed instance: %s", op.TargetLink)
		return deleteBotAsync(c, task.Id, vm.Hostname)
	}
	// Instance destruction is pending.
	return nil
}
