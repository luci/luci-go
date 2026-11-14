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
	"slices"

	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	config "go.chromium.org/luci/gce/api/config/v1"
	tasks "go.chromium.org/luci/gce/api/tasks/v1"
	"go.chromium.org/luci/gce/appengine/backend/internal/metrics"
	"go.chromium.org/luci/gce/appengine/model"
)

// reimageInstanceQueue is the name of the reimage instance task handler queue.
const reimageInstanceQueue = "reimage-instance"

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

// reimageInstance handles a ReimageInstance task queue task. The GCE update and
// operation polling implementation lands in subsequent CLs of this stack.
func reimageInstance(_ context.Context, payload proto.Message) error {
	task, ok := payload.(*tasks.ReimageInstance)
	switch {
	case !ok:
		return errors.Fmt("unexpected payload %q", payload)
	case task.GetId() == "":
		return errors.New("ID is required")
	case task.GetUrl() == "":
		return errors.New("URL is required")
	}
	return nil
}
