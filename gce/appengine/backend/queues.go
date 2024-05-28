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
	"math/rand"
	"time"

	"github.com/golang/protobuf/proto"

	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/appengine/tq"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/rand/mathrand"
	"go.chromium.org/luci/common/data/stringset"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

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
	case errors.Is(err, datastore.ErrNoSuchEntity):
	case err != nil:
		return errors.Annotate(err, "failed to fetch config").Err()
	default:
		vms.AddConfigured(int(cfg.Config.CurrentAmount), cfg.Config.Attributes.Project)
	}

	// Get the actual (connected, created) counts.
	vm := &model.VM{}
	q := datastore.NewQuery(model.VMKind).Eq("config", task.Id)
	if err := datastore.Run(c, q, func(k *datastore.Key) error {
		id := k.StringID()
		vm.ID = id
		switch err := datastore.Get(c, vm); {
		case errors.Is(err, datastore.ErrNoSuchEntity):
			return nil
		case err != nil:
			return errors.Annotate(err, "failed to fetch VM with id: %q", task.GetId()).Err()
		default:
			if vm.Created > 0 {
				vms.AddCreated(1, vm.Attributes.Project, vm.Attributes.Zone)
			}
			if vm.Connected > 0 {
				vms.AddConnected(1, vm.Attributes.Project, vm.Swarming, vm.Attributes.Zone)
			}
			return nil
		}
	}); err != nil {
		return errors.Annotate(err, "failed to fetch VMs").Err()
	}
	if err := vms.Update(c, task.Id, cfg.Config.GetAttributes().GetLabel()["resource_group"]); err != nil {
		return errors.Annotate(err, "failed to update count").Err()
	}
	return nil
}

// drainVMQueue is the name of the drain VM task handler queue.
const drainVMQueue = "drain-vm"

func drainVMQueueHandler(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.DrainVM)
	switch {
	case !ok:
		return errors.Reason("unexpected payload %q", payload).Err()
	case task.GetId() == "":
		return errors.Reason("ID is required").Err()
	}
	vm := &model.VM{
		ID: task.Id,
	}
	switch err := datastore.Get(c, vm); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil
	case err != nil:
		return errors.Annotate(err, "failed to fetch VM with id: %q", task.GetId()).Err()
	case vm.URL == "":
		logging.Debugf(c, "instance %q does not exist", vm.Hostname)
		return nil
	}
	return drainVM(c, vm)
}

// drainVM drains a given VM if necessary.
func drainVM(c context.Context, vm *model.VM) error {
	if vm.Drained {
		return nil
	}
	cfg := &model.Config{
		ID: vm.Config,
	}
	switch err := datastore.Get(c, cfg); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		logging.Debugf(c, "config %q does not exist", cfg.ID)
	case err != nil:
		return errors.Annotate(err, "failed to fetch config").Err()
	}
	if vm.DUT != "" {
		// DUT is still present in config.
		// Index is not available for VM mapped to DUT due to different sequences of creation.
		duts := cfg.Config.GetDuts()
		if _, ok := duts[vm.DUT]; ok {
			return nil
		}
		logging.Debugf(c, "config %q only specifies %d VMs", cfg.ID, cfg.Config.GetCurrentAmount())
	} else {
		// This VM is below the currentAmount threshold and should not be drained.
		if cfg.Config.GetCurrentAmount() > vm.Index {
			return nil
		}
		logging.Debugf(c, "config %q only specifies %d VMs", cfg.ID, cfg.Config.GetCurrentAmount())
	}
	return datastore.RunInTransaction(c, func(c context.Context) error {
		switch err := datastore.Get(c, vm); {
		case errors.Is(err, datastore.ErrNoSuchEntity):
			vm.Drained = true
			return nil
		case err != nil:
			return errors.Annotate(err, "failed to fetch VM prefix: %q", vm.Config).Err()
		case vm.Drained:
			return nil
		}
		vm.Drained = true
		logging.Debugf(c, "set VM %s as drained in db", vm.Hostname)
		if err := datastore.Put(c, vm); err != nil {
			return errors.Annotate(err, "failed to store VM prefix: %q", vm.Config).Err()
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
	case task.GetId() == "":
		return errors.Reason("ID is required").Err()
	case task.GetConfig() == "":
		return errors.Reason("config is required").Err()
	}

	// VMs paired with DUTs cannot rely on index for hostname uniqueness.
	// Instead, we rely on the DUT name.
	var hostname string
	if task.DUT != "" {
		hostname = fmt.Sprintf("%s-%s", task.Id, getSuffix(c))
	} else {
		hostname = fmt.Sprintf("%s-%d-%s", task.Prefix, task.Index, getSuffix(c))
	}
	logging.Debugf(c, "Staring create VM: hostname:%q, task ID:%q, prefix:%q", hostname, task.GetId(), task.GetPrefix())
	vm := &model.VM{
		ID: task.Id,
	}

	// createVM is called repeatedly, so do a fast check outside the transaction.
	// In most cases, this will skip the more expensive transactional check.
	switch err := datastore.Get(c, vm); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		logging.Debugf(c, "Create VM: VM not exists, so proceed with creation: hostname:%q, ID:%q, prefix:%q", hostname, task.GetId(), task.GetPrefix())
	case err != nil:
		return errors.Annotate(err, "failed to fetch VM %q", hostname).Err()
	default:
		logging.Debugf(c, "Create VM: VM already exists: hostname:%q, ID:%q, prefix:%q", vm.Hostname, vm.ID, vm.Prefix)
		return nil
	}
	return datastore.RunInTransaction(c, func(c context.Context) error {
		switch err := datastore.Get(c, vm); {
		case errors.Is(err, datastore.ErrNoSuchEntity):
		case err != nil:
			return errors.Annotate(err, "failed to fetch VM %q", hostname).Err()
		default:
			logging.Debugf(c, "Create VM: VM found: hostname:%q, ID:%q, prefix:%q", vm.Hostname, vm.ID, vm.Prefix)
			return nil
		}

		logging.Debugf(c, "Create VM: updating VM data: hostname:%q, ID:%q, prefix:%q", hostname, task.GetId(), task.GetPrefix())
		vm.Config = task.Config
		vm.ConfigExpanded = task.ConfigExpandTime.AsTime().Unix()
		vm.DUT = task.DUT
		vm.Hostname = hostname
		vm.Index = task.Index
		vm.Lifetime = task.Lifetime
		vm.Prefix = task.Prefix
		vm.Revision = task.Revision
		vm.Swarming = task.Swarming
		vm.Timeout = task.Timeout

		if task.Attributes != nil {
			vm.Attributes = *task.Attributes
			// TODO(crbug/942301): Auto-select zone if zone is unspecified.
			vm.Attributes.SetZone(vm.Attributes.GetZone())
			vm.IndexAttributes()
		}

		if err := datastore.Put(c, vm); err != nil {
			return errors.Annotate(err, "failed to store VM").Err()
		}
		logging.Debugf(c, "VM created: hostname:%q, task ID:%q, prefix:%q", hostname, task.GetId(), task.GetPrefix())
		return nil
	}, nil)
}

// updateCurrentAmount updates CurrentAmount if necessary.
// Returns up-to-date config entity and the reference timestamp.
func updateCurrentAmount(c context.Context, id string) (cfg *model.Config, err error) {
	cfg = &model.Config{
		ID: id,
	}
	// Avoid transaction if possible.
	if err = datastore.Get(c, cfg); err != nil {
		err = errors.Annotate(err, "failed to fetch config").Err()
		return
	}

	var amt int32
	switch amt, err = cfg.Config.ComputeAmount(cfg.Config.CurrentAmount, clock.Now(c)); {
	case err != nil:
		err = errors.Annotate(err, "failed to parse amount").Err()
		return
	case cfg.Config.CurrentAmount == amt:
		return
	}

	err = datastore.RunInTransaction(c, func(c context.Context) error {
		var err error
		if err = datastore.Get(c, cfg); err != nil {
			return errors.Annotate(err, "failed to fetch config").Err()
		}

		switch amt, err = cfg.Config.ComputeAmount(cfg.Config.CurrentAmount, clock.Now(c)); {
		case err != nil:
			return errors.Annotate(err, "failed to parse amount").Err()
		case cfg.Config.CurrentAmount == amt:
			return nil
		}
		cfg.Config.CurrentAmount = amt
		logging.Debugf(c, "set config %q to allow %d VMs", cfg.ID, cfg.Config.CurrentAmount)
		if err = datastore.Put(c, cfg); err != nil {
			return errors.Annotate(err, "failed to store config").Err()
		}
		return nil
	}, nil)
	return
}

// getCurrentVMsByPrefix returns all the VMs in the datastore by prefix
func getCurrentVMsByPrefix(ctx context.Context, prefix string) ([]*model.VM, error) {
	q := datastore.NewQuery(model.VMKind).Eq("prefix", prefix)
	vms := make([]*model.VM, 0)
	if err := datastore.Run(ctx, q, func(vm *model.VM) {
		vms = append(vms, vm)
	}); err != nil {
		return nil, errors.Annotate(err, "failed to fetch vms for %s", prefix).Err()
	}
	return vms, nil
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
	cfg, err := updateCurrentAmount(c, task.Id)
	if err != nil {
		return err
	}
	// Measure the time taken for this query, For debugging purposes
	start := time.Now()
	vms, err := getCurrentVMsByPrefix(c, cfg.Config.Prefix)
	rt := time.Since(start)
	logging.Debugf(c, "getCurrentVMsByPrefix[%s]: error - %v #VMs - %d", rt, err, len(vms))
	if err != nil {
		return err
	}

	expandTime := &timestamppb.Timestamp{Seconds: task.TriggeredUnixTime}
	var t []*tq.Task
	// DUTs take priority.
	if len(cfg.Config.GetDuts()) > 0 {
		t, err = createTasksPerDUT(c, vms, cfg, expandTime)
	} else {
		t, err = createTasksPerAmount(c, vms, cfg, expandTime)
	}
	if err != nil {
		return err
	}

	logging.Debugf(c, "for config %s, creating %d VMs", cfg.Config.Prefix, len(t))
	if err := getDispatcher(c).AddTask(c, t...); err != nil {
		return errors.Annotate(err, "expend config: failed to schedule tasks").Err()
	}
	return nil
}

// createTasksPerDUT returns a slice of CreateVM tasks based on config.Duts.
func createTasksPerDUT(c context.Context, vms []*model.VM, cfg *model.Config, expandTime *timestamppb.Timestamp) ([]*tq.Task, error) {
	logging.Debugf(c, "CloudBots flow entered for config %s", cfg.Config.Prefix)
	if len(cfg.Config.Duts) == 0 {
		return nil, errors.Reason("config.DUTs cannot be empty").Err()
	}
	existingVMs := make(map[string]string, len(vms))
	for _, vm := range vms {
		existingVMs[vm.DUT] = vm.Hostname
	}
	var t []*tq.Task
	var i int32 = 0
	for dut := range cfg.Config.Duts {
		if vm, ok := existingVMs[dut]; ok {
			logging.Debugf(c, "the DUT %s is already assigned to an existing VM %s, skipping", dut, vm)
			continue
		}
		// The max length for VMs name is 63 chars.
		// Setting abbreviate output to 40 chars max should give us some breathing room.
		id := fmt.Sprintf("%s-%s", cfg.Config.Prefix, abbreviate(dut, 40))
		t = append(t, &tq.Task{
			Payload: &tasks.CreateVM{
				Id:               id,
				Attributes:       cfg.Config.Attributes,
				Config:           cfg.ID,
				ConfigExpandTime: expandTime,
				Created: &timestamppb.Timestamp{
					Seconds: clock.Now(c).Unix(),
				},

				// Index is not needed here.
				// CloudBots flow does not rely on Index for VM hostname uniqueness.
				DUT:      dut,
				Lifetime: randomizeLifetime(cfg.Config.Lifetime.GetSeconds()),
				Prefix:   cfg.Config.Prefix,
				Revision: cfg.Config.Revision,
				Swarming: cfg.Config.Swarming,
				Timeout:  cfg.Config.Timeout.GetSeconds(),
			},
		})
		i++
	}
	return t, nil
}

// createTasksPerAmount returns a slice of CreateVM tasks based on config.CurrentAmount.
func createTasksPerAmount(c context.Context, vms []*model.VM, cfg *model.Config, expandTime *timestamppb.Timestamp) ([]*tq.Task, error) {
	logging.Debugf(c, "default flow entered for config %s", cfg.Config.Prefix)
	if len(cfg.Config.Duts) > 0 {
		return nil, errors.Reason("config.Duts should be empty").Err()
	}
	existingVMs := stringset.New(len(vms))
	for _, vm := range vms {
		existingVMs.Add(vm.ID)
	}
	var t []*tq.Task
	for i := int32(0); i < cfg.Config.CurrentAmount; i++ {
		id := fmt.Sprintf("%s-%d", cfg.Config.Prefix, i)
		if !existingVMs.Has(id) {
			t = append(t, &tq.Task{
				Payload: &tasks.CreateVM{
					Id:               id,
					Attributes:       cfg.Config.Attributes,
					Config:           cfg.ID,
					ConfigExpandTime: expandTime,
					Created: &timestamppb.Timestamp{
						Seconds: clock.Now(c).Unix(),
					},
					Index:    i,
					Lifetime: randomizeLifetime(cfg.Config.Lifetime.GetSeconds()),
					Prefix:   cfg.Config.Prefix,
					Revision: cfg.Config.Revision,
					Swarming: cfg.Config.Swarming,
					Timeout:  cfg.Config.Timeout.GetSeconds(),
				},
			})
		}
	}
	return t, nil
}

// randomizeLifetime randomizes the specified lifetime within an interval.
//
// Randomized lifetime of VMs spreads the load of terminated/respawn VMs.
func randomizeLifetime(lifetime int64) int64 {
	interval := lifetime / 10
	if interval <= 0 { // The lifetime is too short or invalid, so do nothing.
		return lifetime
	}
	return lifetime + rand.Int63n(interval)
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
	rsp, err := getCompute(c).Stable.Regions.List(p.Config.Project).Context(c).Do()
	if err != nil {
		if gerr, ok := err.(*googleapi.Error); ok {
			logErrors(c, task.Id, gerr)
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

// Enum used to make the abbreviate function more readable.
const (
	other = iota
	letter
	dash
)

// abbreviate shortens a hostname (dash-delimited string) by
// removing all letters but the first of each group of successive letters encountered.
// Also, removes any dash ("-") repetition.
// limit represents the max length of the returned string.
// E.g. abbreviate("abc123--def456su", ...) === "a123-d456s"
func abbreviate(hostname string, limit int) string {
	var out []rune
	lastChar := other
	for _, r := range hostname {
		if len(out) == limit {
			break
		}
		// that is  number
		if r >= 48 && r <= 57 {
			out = append(out, r)
			lastChar = other
		} else if r == '-' {
			if lastChar == dash {
				continue
			}
			out = append(out, r)
			lastChar = dash
		} else if lastChar != letter {
			out = append(out, r)
			lastChar = letter
		}
	}
	return string(out)
}
