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

	"github.com/golang/protobuf/proto"
	"github.com/google/uuid"

	"google.golang.org/api/compute/v1"
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
	if vm.Hostname == "" {
		// Generate a new hostname and record it so future calls are idempotent.
		hostname := fmt.Sprintf("%s-%s", vm.ID, getSuffix(c))
		if err := datastore.RunInTransaction(c, func(c context.Context) error {
			if err := datastore.Get(c, vm); err != nil {
				return errors.Annotate(err, "failed to fetch VM").Err()
			}
			// Double-check inside transaction. Hostname may already be generated.
			if vm.Hostname == "" {
				vm.Hostname = hostname
				if err := datastore.Put(c, vm); err != nil {
					return errors.Annotate(err, "failed to store VM").Err()
				}
			}
			return nil
		}, nil); err != nil {
			return nil, err
		}
	}
	return vm, nil
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
	srv := compute.NewInstancesService(getCompute(c))
	call := srv.Insert(vm.Attributes.GetProject(), vm.Attributes.GetZone(), vm.GetInstance())
	op, err := call.RequestId(rID.String()).Context(c).Do()
	if err != nil {
		logErrors(c, err.(*googleapi.Error))
		return errors.Reason("failed to create instance").Err()
	}
	if op.Error != nil {
		for _, err := range op.Error.Errors {
			logging.Errorf(c, "%s: %s", err.Code, err.Message)
		}
		return errors.Reason("failed to create instance").Err()
	}
	if op.Status == "DONE" {
		logging.Debugf(c, "created instance: %s", op.TargetLink)
		if err := setURL(c, task.Id, op.TargetLink); err != nil {
			return err
		}
	}
	return nil
}
