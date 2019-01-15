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

// drainVMQueue is the name of the drain VM task handler queue.
const drainVMQueue = "drain-vm"

// drainVM drains a given VM entity.
func drainVM(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.DrainVM)
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

// ensureVMQueue is the name of the ensure VM task handler queue.
const ensureVMQueue = "ensure-vm"

// ensureVM creates or updates a given VM entity.
func ensureVM(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.EnsureVM)
	switch {
	case !ok:
		return errors.Reason("unexpected payload type %T", payload).Err()
	case task.GetConfig() == "":
		return errors.Reason("config is required").Err()
	}
	id := fmt.Sprintf("%s-%d", task.Config, task.Index)
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
		vm.Config = task.Config
		vm.Drained = false
		vm.Index = task.Index
		vm.Lifetime = task.Lifetime
		vm.Prefix = task.Prefix
		vm.Swarming = task.Swarming
		if err := datastore.Put(c, vm); err != nil {
			return errors.Annotate(err, "failed to store VM").Err()
		}
		return nil
	}, nil)
}

// processConfigQueue is the name of the process config task handler queue.
const processConfigQueue = "process-config"

// processConfig creates task queue tasks to process each VM in the given config.
func processConfig(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.ProcessConfig)
	switch {
	case !ok:
		return errors.Reason("unexpected payload type %T", payload).Err()
	case task.GetId() == "":
		return errors.Reason("ID is required").Err()
	}
	cfg, err := getConfig(c).Get(c, &config.GetRequest{Id: task.Id})
	if err != nil {
		return errors.Annotate(err, "failed to get config").Err()
	}
	logging.Debugf(c, "found %d VMs", cfg.Amount)
	// Trigger tasks to create VMs.
	t := make([]*tq.Task, cfg.Amount)
	for i := int32(0); i < cfg.Amount; i++ {
		t[i] = &tq.Task{
			Payload: &tasks.EnsureVM{
				Attributes: cfg.Attributes,
				Config:     task.Id,
				Index:      i,
				Lifetime:   cfg.GetSeconds(),
				Prefix:     cfg.Prefix,
				Swarming:   cfg.Swarming,
			},
		}
	}
	logging.Debugf(c, "ensuring %d VMs", len(t))
	if err := getDispatcher(c).AddTask(c, t...); err != nil {
		return errors.Annotate(err, "failed to schedule ensure tasks").Err()
	}
	// Trigger tasks to drain excess VMs.
	t = make([]*tq.Task, 0)
	q := datastore.NewQuery(model.VMKind).Eq("config", task.Id)
	if err := datastore.Run(c, q, func(vm *model.VM) error {
		if vm.Index < cfg.Amount {
			return nil
		}
		t = append(t, &tq.Task{
			Payload: &tasks.DrainVM{
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
