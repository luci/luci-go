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

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"
)

// drainQueue is the name of the drain task handler queue.
const drainQueue = "drain-vm"

// drain drains a given VM entity.
func drain(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.Drain)
	switch {
	case !ok:
		return errors.Reason("unexpected payload type %T", payload).Err()
	case task.GetId() == "":
		return errors.Reason("ID is required").Err()
	}
	return datastore.RunInTransaction(c, func(c context.Context) error {
		vm := &model.VM{
			ID: task.Id,
		}
		switch err := datastore.Get(c, vm); {
		case err == datastore.ErrNoSuchEntity:
			return nil
		case err != nil:
			return errors.Annotate(err, "failed to fetch VM").Err()
		}
		vm.Drained = true
		if err := datastore.Put(c, vm); err != nil {
			return errors.Annotate(err, "failed to store VM").Err()
		}
		return nil
	}, nil)
}

// ensureQueue is the name of the ensure task handler queue.
const ensureQueue = "ensure-vm"

// ensure creates or updates a given VM entity.
func ensure(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.Ensure)
	switch {
	case !ok:
		return errors.Reason("unexpected payload type %T", payload).Err()
	case task.GetVms() == "":
		return errors.Reason("VMs is required").Err()
	}
	id := fmt.Sprintf("%s-%d", task.Vms, task.Index)
	return datastore.RunInTransaction(c, func(c context.Context) error {
		vm := &model.VM{
			ID: id,
		}
		if err := datastore.Get(c, vm); err != nil && err != datastore.ErrNoSuchEntity {
			return errors.Annotate(err, "failed to fetch VM").Err()
		}
		if task.Attributes != nil {
			vm.Attributes = *task.Attributes
		}
		vm.Drained = false
		vm.Index = task.Index
		vm.Lifetime = task.Lifetime
		vm.Prefix = task.Prefix
		vm.Swarming = task.Swarming
		vm.VMs = task.Vms
		if err := datastore.Put(c, vm); err != nil {
			return errors.Annotate(err, "failed to store VM").Err()
		}
		return nil
	}, nil)
}

// processQueue is the name of the process task handler queue.
const processQueue = "process-config"

// process creates task queue tasks to process each VM entity in the given VMs block.
func process(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.Process)
	switch {
	case !ok:
		return errors.Reason("unexpected payload type %T", payload).Err()
	case task.GetId() == "":
		return errors.Reason("ID is required").Err()
	}
	vms, err := getConfig(c).GetVMs(c, &config.GetVMsRequest{Id: task.Id})
	if err != nil {
		return errors.Annotate(err, "failed to get VMs block").Err()
	}
	logging.Debugf(c, "found %d configured VMs", vms.Amount)
	// Trigger tasks to create VMs.
	t := make([]*tq.Task, vms.Amount)
	for i := int32(0); i < vms.Amount; i++ {
		t[i] = &tq.Task{
			Payload: &tasks.Ensure{
				Attributes: vms.Attributes,
				Index:      i,
				Lifetime:   vms.GetSeconds(),
				Prefix:     vms.Prefix,
				Swarming:   vms.Swarming,
				Vms:        task.Id,
			},
		}
	}
	logging.Debugf(c, "ensuring %d VMs", len(t))
	if err := getDispatcher(c).AddTask(c, t...); err != nil {
		return errors.Annotate(err, "failed to schedule ensure tasks").Err()
	}
	// Trigger tasks to drain excess VMs.
	t = make([]*tq.Task, 0)
	q := datastore.NewQuery(model.VMKind).Eq("vms", task.Id)
	if err := datastore.Run(c, q, func(vm *model.VM) error {
		if vm.Index < vms.Amount {
			return nil
		}
		t = append(t, &tq.Task{
			Payload: &tasks.Drain{
				Id: vm.ID,
			},
		})
		return nil
	}); err != nil {
		return errors.Annotate(err, "failed to fetch VM to drain").Err()
	}
	logging.Debugf(c, "draining %d VMs", len(t))
	if err := getDispatcher(c).AddTask(c, t...); err != nil {
		return errors.Annotate(err, "failed to schedule drain tasks").Err()
	}
	return nil
}
