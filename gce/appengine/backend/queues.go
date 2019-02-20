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

	"google.golang.org/api/googleapi"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/model"
)

// drainVMQueue is the name of the drain VM task handler queue.
const drainVMQueue = "drain-vm"

// drainVM drains a given VM.
func drainVM(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.DrainVM)
	switch {
	case !ok:
		return errors.Reason("unexpected payload type %T", payload).Err()
	case task.GetId() == "":
		return errors.Reason("ID is required").Err()
	}
	vm := &model.VM{
		ID: task.Id,
	}
	switch err := datastore.Get(c, vm); {
	case err == datastore.ErrNoSuchEntity:
		return nil
	case err != nil:
		return errors.Annotate(err, "failed to fetch VM").Err()
	case vm.Drained:
		return nil
	}
	cfg := &model.Config{
		ID: vm.Config,
	}
	switch err := datastore.Get(c, cfg); {
	case err == datastore.ErrNoSuchEntity:
		// Config doesn't exist, drain the VM.
	case err != nil:
		return errors.Annotate(err, "failed to fetch config").Err()
	case cfg.Config.GetAmount().GetDefault() > vm.Index:
		// VM is configured to exist, don't drain the VM.
		return nil
	}
	logging.Debugf(c, "draining VM")
	return datastore.RunInTransaction(c, func(c context.Context) error {
		// Double-check inside transaction.
		// VM may already be drained or deleted.
		switch err := datastore.Get(c, vm); {
		case err == datastore.ErrNoSuchEntity:
			return nil
		case err != nil:
			return errors.Annotate(err, "failed to fetch VM").Err()
		case vm.Drained:
			return nil
		}
		vm.Drained = true
		if err := datastore.Put(c, vm); err != nil {
			return errors.Annotate(err, "failed to store VM").Err()
		}
		return nil
	}, nil)
}

// createVMQueue is the name of the create VM task handler queue.
const createVMQueue = "create-vm"

// createVM creates a VM if it doesn't already exist.
func createVM(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.CreateVM)
	switch {
	case !ok:
		return errors.Reason("unexpected payload type %T", payload).Err()
	case task.GetConfig() == "":
		return errors.Reason("config is required").Err()
	}
	id := fmt.Sprintf("%s-%d", task.Config, task.Index)
	vm := &model.VM{
		ID: id,
	}
	// createVM is called repeatedly, so do a fast check outside the transaction.
	// In most cases, this will skip the more expensive transactional check.
	switch err := datastore.Get(c, vm); {
	case err == datastore.ErrNoSuchEntity:
	case err != nil:
		return errors.Annotate(err, "failed to fetch VM").Err()
	default:
		return nil
	}
	return datastore.RunInTransaction(c, func(c context.Context) error {
		switch err := datastore.Get(c, vm); {
		case err == datastore.ErrNoSuchEntity:
		case err != nil:
			return errors.Annotate(err, "failed to fetch VM").Err()
		default:
			return nil
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

// expandConfigQueue is the name of the expand config task handler queue.
const expandConfigQueue = "expand-config"

// expandConfig creates task queue tasks to create each VM in the given config.
func expandConfig(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.ExpandConfig)
	switch {
	case !ok:
		return errors.Reason("unexpected payload type %T", payload).Err()
	case task.GetId() == "":
		return errors.Reason("ID is required").Err()
	}
	cfg, err := getConfig(c).Get(c, &config.GetRequest{Id: task.Id})
	if err != nil {
		return errors.Annotate(err, "failed to fetch config").Err()
	}
	t := make([]*tq.Task, cfg.GetAmount().GetDefault())
	for i := int32(0); i < cfg.GetAmount().GetDefault(); i++ {
		t[i] = &tq.Task{
			Payload: &tasks.CreateVM{
				Attributes: cfg.Attributes,
				Config:     task.Id,
				Index:      i,
				Lifetime:   cfg.GetLifetime().GetSeconds(),
				Prefix:     cfg.Prefix,
				Swarming:   cfg.Swarming,
			},
		}
	}
	logging.Debugf(c, "creating %d VMs", len(t))
	if err := getDispatcher(c).AddTask(c, t...); err != nil {
		return errors.Annotate(err, "failed to schedule tasks").Err()
	}
	return nil
}

// reportQuotaQueue is the name of the report quota task handler queue.
const reportQuotaQueue = "report-quota"

// reportQuota reports GCE quota utilization.
func reportQuota(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.ReportQuota)
	switch {
	case !ok:
		return errors.Reason("unexpected payload type %T", payload).Err()
	case task.GetId() == "":
		return errors.Reason("ID is required").Err()
	}
	p := &model.Project{
		ID: task.Id,
	}
	if err := datastore.Get(c, p); err != nil {
		return errors.Annotate(err, "failed to fetch project").Err()
	}
	metrics := stringset.NewFromSlice(p.Metrics...)
	regions := stringset.NewFromSlice(p.Regions...)
	rsp, err := getCompute(c).Regions.List(p.Project).Context(c).Do()
	if err != nil {
		gerr := err.(*googleapi.Error)
		logErrors(c, gerr)
		return errors.Reason("failed to fetch quota").Err()
	}
	for _, reg := range rsp.Items {
		if regions.Has(reg.Name) {
			logging.Infof(c, "found region %q", reg.Name)
			for _, q := range reg.Quotas {
				if metrics.Has(q.Metric) {
					logging.Infof(c, "metric %q: %f/%f", q.Metric, q.Usage, q.Limit)
					// TODO(smut): Export metrics.
				}
			}
		}
	}
	return nil
}
