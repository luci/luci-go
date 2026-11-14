// Copyright 2026 The LUCI Authors.
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
	"slices"
	"time"

	"github.com/google/uuid"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	config "go.chromium.org/luci/gce/api/config/v1"
	tasks "go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/backend/internal/metrics"
	"go.chromium.org/luci/gce/appengine/model"
)

const (
	// reimageInstanceQueue is the name of the reimage instance task handler queue.
	reimageInstanceQueue = "reimage-instance"
	// maxReimageDuration bounds how long an in-place reimage may remain in flight
	// before falling back to full VM destruction.
	maxReimageDuration = 20 * time.Minute
)

// recycleInstanceAsync schedules an in-place reimage when configured and
// compatible, falling back to full VM destruction otherwise.
func recycleInstanceAsync(c context.Context, vm *model.VM) error {
	cfg, _, reason, err := reimageConfig(c, vm)
	if err != nil {
		return err
	}
	if cfg == nil {
		// Only emit skipped metrics when REIMAGE_IN_PLACE was configured.
		if reason != "" {
			logging.Debugf(c, "Recycle instance %q: skipping reimage (%s)", vm.Hostname, reason)
			metrics.UpdateReimageCount(c, "skipped", reason, vm)
		}
		return destroyInstanceAsync(c, vm)
	}
	return reimageInstanceAsync(c, vm)
}

// reimageInstanceAsync schedules a task queue task to reimage a GCE instance in place.
func reimageInstanceAsync(c context.Context, vm *model.VM) error {
	return errors.WrapIf(getDispatcher(c).AddTask(c, &tq.Task{
		Payload: &tasks.ReimageInstance{
			Id:      vm.ID,
			Url:     vm.URL,
			Created: vm.Created,
		},
	}), "failed to schedule reimage task")
}

// reimageConfig fetches vm's Config and checks whether vm can be reimaged in
// place, returning the resolved target attributes or a non-empty skip reason
// when RECYCLE_STRATEGY_REIMAGE_IN_PLACE is configured but incompatible.
func reimageConfig(c context.Context, vm *model.VM) (*model.Config, *config.VM, string, error) {
	if vm.Config == "" {
		return nil, nil, "", nil
	}
	cfg := &model.Config{ID: vm.Config}
	switch err := datastore.Get(c, cfg); {
	case errors.Is(err, datastore.ErrNoSuchEntity):
		return nil, nil, "", nil
	case err != nil:
		return nil, nil, "", errors.Fmt("failed to fetch config %q: %w", vm.Config, err)
	case cfg.Config.GetRecycleStrategy() != config.RecycleStrategy_RECYCLE_STRATEGY_REIMAGE_IN_PLACE:
		return nil, nil, "", nil
	case vm.Drained || (vm.DUT != "" && cfg.Config.GetDuts()[vm.DUT] == nil) || (vm.DUT == "" && cfg.Config.GetCurrentAmount() <= vm.Index):
		// Scaled-down or drained VMs must be deleted rather than refreshed.
		return nil, nil, "drained", nil
	case cfg.Config.GetSwarming() != vm.Swarming:
		return nil, nil, "swarming_changed", nil
	}
	target, reason := reimageAttributes(&vm.Attributes, cfg.Config.GetAttributes())
	if reason != "" {
		return nil, nil, reason, nil
	}
	return cfg, target, "", nil
}

// reimageAttributes resolves next to cur's active zone and verifies only disk
// images changed while cur's zone remains in next's configured zones.
func reimageAttributes(cur, next *config.VM) (*config.VM, string) {
	if cur == nil || next == nil {
		return nil, "attributes_changed"
	}
	if cur.GetProject() != next.GetProject() {
		return nil, "project_changed"
	}
	if cur.GetZone() == "" || !slices.Contains(configuredZones(next), cur.GetZone()) {
		return nil, "zone_changed"
	}
	curCmp, target := proto.Clone(cur).(*config.VM), proto.Clone(next).(*config.VM)
	curCmp.SetZone(cur.GetZone())
	target.SetZone(cur.GetZone())
	if len(curCmp.GetDisk()) != len(target.GetDisk()) {
		return nil, "disks_changed"
	}
	targetCmp := proto.Clone(target).(*config.VM)
	for i := range curCmp.Disk {
		if curCmp.Disk[i] == nil || targetCmp.Disk[i] == nil {
			if curCmp.Disk[i] != targetCmp.Disk[i] {
				return nil, "disks_changed"
			}
			continue
		}
		curCmp.Disk[i].Image, targetCmp.Disk[i].Image = "", ""
		if !proto.Equal(curCmp.Disk[i], targetCmp.Disk[i]) {
			return nil, "disks_changed"
		}
	}
	if curCmp.GetMachineType() != targetCmp.GetMachineType() {
		return nil, "machine_type_changed"
	}
	if !proto.Equal(curCmp, targetCmp) {
		return nil, "attributes_changed"
	}
	return target, ""
}

// reimageInstance handles a ReimageInstance task queue task by dispatching
// initial GCE disk updates, polling in-flight operations, or abandoning timed-out updates.
func reimageInstance(c context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.ReimageInstance)
	switch {
	case !ok:
		return errors.Fmt("unexpected payload %q", payload)
	case task.GetId() == "":
		return errors.New("ID is required")
	case task.GetUrl() == "":
		return errors.New("URL is required")
	}
	vm := &model.VM{ID: task.Id}
	switch err := datastore.Get(c, vm); {
	case errors.Is(err, datastore.ErrNoSuchEntity) || vm.URL == "":
		return nil
	case err != nil:
		return errors.Fmt("reimage instance: failed to fetch VM %q: %w", vm.ID, err)
	case vm.URL != task.GetUrl():
		// Instance was replaced with a different GCE instance URL; ignore stale task.
		logging.Debugf(c, "Reimage instance %q: URL mismatch (%s != %s)", vm.Hostname, task.GetUrl(), vm.URL)
		return nil
	case task.GetCreated() != 0 && vm.Created != task.GetCreated():
		// Instance already started a new lifetime; ignore duplicate/late task.
		logging.Debugf(c, "Reimage instance %q: stale lifetime (%d != %d)", vm.Hostname, task.GetCreated(), vm.Created)
		return nil
	case vm.Reimaging > 0 && clock.Now(c).Sub(time.Unix(vm.Reimaging, 0)) > maxReimageDuration:
		// Bound in-place reimage retries so wedged GCE updates fall back to recreate.
		return abandonReimage(c, vm, "timeout", "reimage deadline exceeded")
	case vm.ReimageOp != "":
		return checkReimage(c, vm)
	default:
		return sendReimage(c, vm)
	}
}

// timePeriodSeconds converts an optional TimePeriod proto to seconds.
func timePeriodSeconds(tp *config.TimePeriod) (int64, error) {
	if tp.GetTime() == nil {
		return 0, nil
	}
	return tp.ToSeconds()
}

// sendReimage fetches the live GCE instance, builds an instances.update payload
// that recreates every disk in place with RESTART, and records the operation on vm.
func sendReimage(c context.Context, vm *model.VM) error {
	cfg, targetAttrs, reason, err := reimageConfig(c, vm)
	if err != nil {
		return err
	}
	if cfg == nil {
		// Config or pool changed while queued; abandon if already marked reimaging.
		if vm.Reimaging > 0 {
			if reason == "" {
				reason = "config_changed"
			}
			return abandonReimage(c, vm, reason, "VM no longer eligible for reimage")
		}
		if reason != "" {
			metrics.UpdateReimageCount(c, "skipped", reason, vm)
		}
		return destroyInstanceAsync(c, vm)
	}
	lifetime, err := timePeriodSeconds(cfg.Config.GetLifetime())
	if err != nil {
		return errors.Fmt("reimage instance %q: invalid lifetime: %w", vm.Hostname, err)
	}
	timeout, err := timePeriodSeconds(cfg.Config.GetTimeout())
	if err != nil {
		return errors.Fmt("reimage instance %q: invalid timeout: %w", vm.Hostname, err)
	}
	srv := getCompute(c).Stable.Instances
	proj, zone := vm.Attributes.GetProject(), vm.Attributes.GetZone()
	inst, err := srv.Get(proj, zone, vm.Hostname).Context(c).Do()
	if err != nil {
		return handleReimageError(c, vm, err)
	}
	disks, err := (&model.VM{Attributes: *targetAttrs}).ReimageDisks(inst.Disks)
	if err != nil {
		return abandonReimage(c, vm, "disk_mismatch", err.Error())
	}
	// Mutate the fetched compute.Instance in place to preserve GCE fingerprints.
	inst.Disks = disks
	// Key RequestId on (vm.URL, vm.Created): vm.Created advances on every completed
	// reimage so retries within one lifetime are idempotent while subsequent lifetimes
	// receive a distinct request ID.
	rID := uuid.NewSHA1(uuid.Nil, []byte(fmt.Sprintf("reimage-%s-%d", vm.URL, vm.Created)))
	op, err := srv.Update(proj, zone, vm.Hostname, inst).MostDisruptiveAllowedAction("RESTART").RequestId(rID.String()).Context(c).Do()
	if err != nil {
		return handleReimageError(c, vm, err)
	}
	if op.Error != nil && len(op.Error.Errors) > 0 {
		return abandonReimage(c, vm, "operation_error", op.Error.Errors[0].Code)
	}
	if op.Name == "" {
		return abandonReimage(c, vm, "operation_error", "missing operation name")
	}
	updated, err := mutateReimageVM(c, vm, func(cur *model.VM) {
		if cur.Reimaging == 0 {
			cur.Reimaging = clock.Now(c).Unix()
		}
		cur.ReimageOp = op.Name
		cur.Attributes = *targetAttrs
		cur.IndexAttributes()
		cur.Lifetime = randomizeLifetime(lifetime)
		cur.Timeout = timeout
		cur.Revision = cfg.Config.GetRevision()
		cur.ScalingType = getScalingType(cfg.Config)
	})
	if err != nil || updated == nil || op.Status != "DONE" {
		return err
	}
	return completeReimage(c, updated, op.EndTime)
}

// mutateReimageVM updates vm inside a Datastore transaction if it still exists
// with the same GCE instance URL and lifetime creation timestamp.
func mutateReimageVM(c context.Context, vm *model.VM, fn func(*model.VM)) (*model.VM, error) {
	var out *model.VM
	err := datastore.RunInTransaction(c, func(c context.Context) error {
		cur := &model.VM{ID: vm.ID}
		err := datastore.Get(c, cur)
		if errors.Is(err, datastore.ErrNoSuchEntity) {
			out = nil
			return nil
		}
		if err != nil {
			return err
		}
		if cur.URL != vm.URL || cur.Created != vm.Created {
			out = nil
			return nil
		}
		fn(cur)
		out = cur
		return datastore.Put(c, cur)
	}, nil)
	return out, errors.WrapIf(err, "failed to update VM %q", vm.ID)
}

// handleReimageError classifies GCE API errors, abandoning permanent failures
// while recording Reimaging start time for bounded retries on transient errors.
func handleReimageError(c context.Context, vm *model.VM, err error) error {
	if gerr, ok := err.(*googleapi.Error); ok {
		logErrors(c, "Reimage instance", vm.Hostname, gerr)
		if gerr.Code == http.StatusNotFound {
			return abandonReimage(c, vm, "not_found", gerr.Message)
		}
		// Permanent 4xx errors (excluding conflict, precondition, and GCE 429/403 rate limits) cannot succeed on retry.
		if gerr.Code >= 400 && gerr.Code < 500 &&
			gerr.Code != http.StatusConflict &&
			gerr.Code != http.StatusPreconditionFailed &&
			gerr.Code != http.StatusTooManyRequests &&
			!rateLimitExceeded(gerr) {
			return abandonReimage(c, vm, "api_error", fmt.Sprintf("HTTP %d", gerr.Code))
		}
	}
	// Record Reimaging start time so repeated transient errors still hit maxReimageDuration.
	if vm.Reimaging == 0 {
		if _, mErr := mutateReimageVM(c, vm, func(cur *model.VM) {
			if cur.Reimaging == 0 {
				cur.Reimaging = clock.Now(c).Unix()
			}
		}); mErr != nil {
			logging.Warningf(c, "Reimage instance %q: failed to record reimage start time: %v", vm.Hostname, mErr)
		}
	}
	return errors.Fmt("reimage instance %q: %w", vm.Hostname, err)
}

// abandonReimage latches Drained on vm, clears in-flight reimage fields, and
// schedules full VM destruction so subsequent ticks do not re-enter reimaging.
func abandonReimage(c context.Context, vm *model.VM, metricReason, detail string) error {
	logging.Warningf(c, "Reimage instance %q: abandoning (%s: %s), destroying instead", vm.Hostname, metricReason, detail)
	metrics.UpdateReimageCount(c, "abandoned", metricReason, vm)
	cur, err := mutateReimageVM(c, vm, func(v *model.VM) {
		v.Drained = true
		v.Reimaging = 0
		v.ReimageOp = ""
	})
	if err != nil || cur == nil {
		return err
	}
	return destroyInstanceAsync(c, cur)
}

// checkReimage polls an in-flight zonal reimage operation and finalizes or
// abandons the reimage once GCE marks the operation DONE.
func checkReimage(c context.Context, vm *model.VM) error {
	proj, zone := vm.Attributes.GetProject(), vm.Attributes.GetZone()
	op, err := getCompute(c).Stable.ZoneOperations.Get(proj, zone, vm.ReimageOp).Context(c).Do()
	if err != nil {
		return handleReimageError(c, vm, err)
	}
	if op.Error != nil && len(op.Error.Errors) > 0 {
		return abandonReimage(c, vm, "operation_error", op.Error.Errors[0].Code)
	}
	if op.Status != "DONE" {
		logging.Debugf(c, "Reimage instance %q: operation %q status is %s", vm.Hostname, vm.ReimageOp, op.Status)
		return nil
	}
	return completeReimage(c, vm, op.EndTime)
}

// completeReimage verifies the updated GCE instance is RUNNING and transitions
// vm to its new lifetime.
func completeReimage(c context.Context, vm *model.VM, endTime string) error {
	proj, zone := vm.Attributes.GetProject(), vm.Attributes.GetZone()
	inst, err := getCompute(c).Stable.Instances.Get(proj, zone, vm.Hostname).Context(c).Do()
	if err != nil {
		return handleReimageError(c, vm, err)
	}
	// If the VM was externally stopped, instances.update recreates disks without
	// starting the guest; destroy and recreate so the pool recovers a running VM.
	if inst.Status != "RUNNING" {
		return abandonReimage(c, vm, "instance_status", inst.Status)
	}
	return finishReimage(c, vm, endTime, inst)
}

// finishReimage advances vm.Created to the operation completion timestamp,
// refreshes network interface IPs, clears in-flight reimage state, and emits metrics.
func finishReimage(c context.Context, vm *model.VM, endTime string, inst *compute.Instance) error {
	nowUnix := clock.Now(c).Unix()
	endUnix := nowUnix
	if endTime != "" {
		if t, err := time.Parse(time.RFC3339, endTime); err == nil {
			endUnix = min(nowUnix, t.Unix())
		}
	}
	var reimageDur float64
	updated, err := mutateReimageVM(c, vm, func(cur *model.VM) {
		// Anchor Created to op.EndTime (clamped to [max(Reimaging, Created+1), now])
		// so bots that reconnect before the next 1m manageBot poll have LastSeenTs >= Created.
		createdUnix := max(cur.Reimaging, cur.Created+1, endUnix)
		if nowUnix > cur.Created {
			createdUnix = min(createdUnix, nowUnix)
		}
		reimageDur = max(0, float64(endUnix-cur.Reimaging))
		if len(inst.NetworkInterfaces) > 0 {
			nics := make([]model.NetworkInterface, len(inst.NetworkInterfaces))
			for i, n := range inst.NetworkInterfaces {
				if len(n.AccessConfigs) > 0 {
					nics[i].ExternalIP = n.AccessConfigs[0].NatIP
				}
				nics[i].InternalIP = n.NetworkIP
			}
			cur.NetworkInterfaces = nics
		}
		cur.ConfigExpanded = createdUnix
		cur.Created = createdUnix
		cur.Connected = 0
		cur.Reimaging = 0
		cur.ReimageOp = ""
	})
	if err != nil || updated == nil {
		return err
	}
	metrics.UpdateReimageCount(c, "success", "none", updated)
	metrics.ReportReimageTime(c, reimageDur, updated.Prefix, updated.Attributes.GetProject(), updated.Attributes.GetZone())
	return nil
}
