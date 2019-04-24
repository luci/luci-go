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
	"github.com/golang/protobuf/ptypes/timestamp"

	"google.golang.org/api/googleapi"

	"go.chromium.org/gae/service/datastore"
	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/backend/internal/metrics"
	"go.chromium.org/luci/gce/appengine/model"
)

// countVMsQueue is the name of the count VMs task handler queue.
const countVMsQueue = "count-vms"

// countVMs counts the VMs for a given config.
func countVMs(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.CountVMs)
	switch {
	case !ok:
		return errors.Reason("unexpected payload type %T", payload).Err()
	case task.GetId() == "":
		return errors.Reason("ID is required").Err()
	}
	// Count VMs per project, server and zone.
	// VMs created from the same config eventually have the same project, server,
	// and zone but may currently exist for a previous version of the config.
	vms := &metrics.InstanceCount{}

	// Get the configured count.
	cfg := &model.Config{
		ID: task.Id,
	}
	switch err := datastore.Get(c, cfg); {
	case err == datastore.ErrNoSuchEntity:
	case err != nil:
		return errors.Annotate(err, "failed to fetch config").Err()
	default:
		amt, err := cfg.Config.Amount.GetAmount(clock.Now(c))
		if err != nil {
			return errors.Annotate(err, "failed to parse amount").Err()
		}
		vms.AddConfigured(int(amt), cfg.Config.Attributes.Project)
	}

	// Get the actual (connected, created) counts.
	var keys []*datastore.Key
	q := datastore.NewQuery(model.VMKind).Eq("config", task.Id)
	if err := datastore.GetAll(c, q, &keys); err != nil {
		return errors.Annotate(err, "failed to fetch VMs").Err()
	}
	vm := &model.VM{}
	for _, k := range keys {
		id := k.StringID()
		vm.ID = id
		switch err := datastore.Get(c, vm); {
		case err == datastore.ErrNoSuchEntity:
		case err != nil:
			return errors.Annotate(err, "failed to fetch VM").Err()
		default:
			if vm.Created > 0 {
				vms.AddCreated(1, vm.Attributes.Project, vm.Attributes.Zone)
			}
			if vm.Connected > 0 {
				vms.AddConnected(1, vm.Attributes.Project, vm.Swarming, vm.Attributes.Zone)
			}
		}
	}
	if err := vms.Update(c, task.Id); err != nil {
		return errors.Annotate(err, "failed to update count").Err()
	}
	return nil
}

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
	}
	switch amt, err := cfg.Config.Amount.GetAmount(clock.Now(c)); {
	case err != nil:
		return errors.Annotate(err, "failed to parse amount").Err()
	case amt > vm.Index:
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

// getSuffix returns a random suffix to use when naming a GCE instance.
func getSuffix(c context.Context) string {
	const allowed = "abcdefghijklmnopqrstuvwxyz0123456789"
	suf := make([]byte, 4)
	for i := range suf {
		suf[i] = allowed[mathrand.Intn(c, len(allowed))]
	}
	return string(suf)
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
		ID:       id,
		Config:   task.Config,
		Hostname: fmt.Sprintf("%s-%s", id, getSuffix(c)),
		Index:    task.Index,
		Lifetime: task.Lifetime,
		Prefix:   task.Prefix,
		Revision: task.Revision,
		Swarming: task.Swarming,
		Timeout:  task.Timeout,
	}
	if task.Attributes != nil {
		vm.Attributes = *task.Attributes
		// TODO(crbug/942301): Auto-select zone if zone is unspecified.
		vm.Attributes.SetZone(vm.Attributes.GetZone())
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
	now := clock.Now(c)
	amt, err := cfg.Amount.GetAmount(now)
	if err != nil {
		return errors.Annotate(err, "failed to parse amount").Err()
	}
	t := make([]*tq.Task, amt)
	for i := int32(0); i < amt; i++ {
		t[i] = &tq.Task{
			Payload: &tasks.CreateVM{
				Attributes: cfg.Attributes,
				Config:     task.Id,
				Created: &timestamp.Timestamp{
					Seconds: now.Unix(),
				},
				Index:    i,
				Lifetime: cfg.Lifetime.GetSeconds(),
				Prefix:   cfg.Prefix,
				Revision: cfg.Revision,
				Swarming: cfg.Swarming,
				Timeout:  cfg.Timeout.GetSeconds(),
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
	mets := stringset.NewFromSlice(p.Config.Metric...)
	regs := stringset.NewFromSlice(p.Config.Region...)
	rsp, err := getCompute(c).Regions.List(p.Config.Project).Context(c).Do()
	if err != nil {
		if gerr, ok := err.(*googleapi.Error); ok {
			logErrors(c, gerr)
			return errors.Reason("failed to fetch quota").Err()
		}
		return errors.Annotate(err, "failed to fetch quota").Err()
	}
	for _, r := range rsp.Items {
		if regs.Has(r.Name) {
			for _, q := range r.Quotas {
				if mets.Has(q.Metric) {
					metrics.UpdateQuota(c, q.Limit, q.Usage, q.Metric, p.Config.Project, r.Name)
				}
			}
		}
	}
	return nil
}
