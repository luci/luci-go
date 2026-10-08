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

	"github.com/google/uuid"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/gce/api/config/v1"
	"go.chromium.org/luci/gce/appengine/backend/internal/metrics"
	"go.chromium.org/luci/gce/appengine/model"
)

// Multi-zone fallback instance creation overview:
//
// When a VM config specifies `fallback_zones` alongside the primary `zone`,
// GCE Provider attempts to create the instance in the primary zone first and
// sequentially falls back to `fallback_zones` whenever GCP returns a hard
// capacity stockout (HTTP 503 or async operation errors such as
// ZONE_RESOURCE_POOL_EXHAUSTED or ZONE_RESOURCE_POOL_EXHAUSTED_WITH_DETAILS).
//
// Per-VM Fallback State (`model.VM` in Datastore):
//   - `Attributes.Zone`: The active candidate zone cursor currently being
//     tried or polled for this VM.
//   - `FallbackOriginalZone`: The immutable primary zone configured when the
//     VM entity was expanded in `createVM`. Preserved across rotations so the
//     VM can wrap back around to its original zone if all fallback zones fail.
//   - `FallbackFailedZones`: The ordered per-VM trace of candidate zones that
//     failed with a stockout during the current rotation pass. Initialized to
//     nil in `createVM`, appended to on each stockout in `rotateVMZone`, and
//     reset to nil when all candidate zones are exhausted and the VM wraps
//     around to `FallbackOriginalZone`.
//   - `FallbackZoneRotations`: The cumulative count of times this VM rotated
//     to a new zone attempt due to stockouts across all passes. Initialized to
//     0 in `createVM`, incremented on every stockout rotation in
//     `rotateVMZone`, persisted via `setActiveVMZone` (while async creation is
//     pending or wrapping around) and `updateVMZone` (when creation succeeds),
//     used to disambiguate GCE `requestId`s across wrap-around passes, and
//     reported via `gce/instances/zone_rotations` in `checkInstance`.
//
// Template Preservation Across Asynchronous Ticks:
//   - While `op.GetStatus() != "DONE"`, `{{.Zone}}` templates in `Disk.Type`
//     and `MachineType` remain unexpanded in Datastore (`setActiveVMZone`), and
//     each attempt uses an in-memory clone (`cloneVMForZone`) bound to
//     `targetZone`.
//   - Once `op.GetStatus() == "DONE"`, `updateVMZone` expands `{{.Zone}}`
//     templates via `SetZone(targetZone)` and persists the winning zone and
//     rotation history.

// originalVMZone returns the initially configured primary zone for the VM.
func originalVMZone(vm *model.VM) string {
	if vm.FallbackOriginalZone != "" {
		return vm.FallbackOriginalZone
	}
	return vm.Attributes.GetZone()
}

// allCandidateZones returns the full ordered, deduplicated list of candidate
// zones configured for the VM, starting with its original primary zone.
func allCandidateZones(vm *model.VM) []string {
	all := []string{originalVMZone(vm)}
	for _, fb := range vm.Attributes.GetFallbackZones() {
		if fb != "" && !slices.Contains(all, fb) {
			all = append(all, fb)
		}
	}
	return all
}

// candidateZones returns the ordered, deduplicated list of remaining zones to
// try for creating the given VM, skipping any zones recorded in
// vm.FallbackFailedZones during the current rotation pass and starting from the
// active zone cursor. If all configured zones are in vm.FallbackFailedZones, it
// wraps around to the full candidate list starting with the original primary
// zone.
func candidateZones(vm *model.VM) []string {
	all := allCandidateZones(vm)
	var candidates []string
	for _, z := range all {
		if !slices.Contains(vm.FallbackFailedZones, z) {
			candidates = append(candidates, z)
		}
	}
	if len(candidates) == 0 {
		candidates = all
	}
	if curZone := vm.Attributes.GetZone(); curZone != "" {
		if idx := slices.Index(candidates, curZone); idx > 0 {
			candidates = candidates[idx:]
		}
	}
	return candidates
}

// createInstanceRequestID generates a deterministic GCE request ID based on
// the VM hostname, target zone, and cumulative fallback rotation count.
// Including FallbackZoneRotations when > 0 ensures that if a VM exhausts all
// candidate zones and wraps back around to a previously tried zone within GCE's
// 24-hour requestId cache window, GCE executes a fresh Insert call instead of
// re-returning the cached failed operation.
func createInstanceRequestID(vm *model.VM, targetZone string) uuid.UUID {
	reqKey := fmt.Sprintf("create-%s-%s", vm.Hostname, targetZone)
	if vm.FallbackZoneRotations > 0 {
		reqKey = fmt.Sprintf("create-%s-%s-%d", vm.Hostname, targetZone, vm.FallbackZoneRotations)
	}
	return uuid.NewSHA1(uuid.Nil, []byte(reqKey))
}

// recordZoneFallback logs and records metrics for a zone fallback transition.
func recordZoneFallback(ctx context.Context, vm *model.VM, fromZone, toZone, reason string) {
	logging.Warningf(ctx, "Create instance %q: stockout in zone %q (%s), falling back to zone %q", vm.Hostname, fromZone, reason, toZone)
	metrics.UpdateZoneFallback(ctx, vm, fromZone, toZone, reason)
}

// rotateVMZone updates vm's per-instance failed zone trace and rotation counter
// when targetZone fails with a stockout. If nextZone is non-empty, it advances
// to nextZone; otherwise, all candidate zones in the current pass are exhausted
// and it wraps around to the original primary zone for the next attempt.
func rotateVMZone(ctx context.Context, vm, attemptVM *model.VM, targetZone, nextZone, reason string) (bool, error) {
	if vm.FallbackOriginalZone == "" {
		vm.FallbackOriginalZone = originalVMZone(vm)
	}
	if !slices.Contains(vm.FallbackFailedZones, targetZone) {
		vm.FallbackFailedZones = append(vm.FallbackFailedZones, targetZone)
	}
	vm.FallbackZoneRotations++
	if nextZone != "" {
		vm.Attributes.Zone = nextZone
		recordZoneFallback(ctx, attemptVM, targetZone, nextZone, reason)
		return true, nil
	}
	origZone := originalVMZone(vm)
	vm.FallbackFailedZones = nil
	vm.Attributes.Zone = origZone
	recordZoneFallback(ctx, attemptVM, targetZone, origZone, reason)
	if err := setActiveVMZone(ctx, vm.ID, origZone, origZone, nil, vm.FallbackZoneRotations); err != nil {
		return false, errors.Fmt("failed to reset zone to original zone %s for instance %s: %w", origZone, vm.Hostname, err)
	}
	return false, errors.Fmt("failed to create instance %s: all candidate zones exhausted, rotating back to original zone %s", vm.Hostname, origZone)
}

// handleInsertError processes an error returned by InsertInstance, recording
// failure and fallback metrics and returning whether the caller should retry on
// nextZone or abort with an error.
func handleInsertError(ctx context.Context, vm, attemptVM *model.VM, err error, targetZone, nextZone string, hasFallbacks bool) (bool, error) {
	logging.Debugf(ctx, "Create instance %q: got error from attempt to create instance %s", vm.Hostname, err)
	gerr, ok := err.(*googleapi.Error)
	if !ok {
		return false, errors.Fmt("failed to create instance %s: %w", vm.Hostname, err)
	}
	logErrors(ctx, "Create instance", vm.Hostname, gerr)
	stockout, reason := isStockoutGoogleAPIError(gerr)
	metrics.UpdateFailures(ctx, gerr.Code, reason, attemptVM)
	// TODO(b/130826296): Remove this once rate limit returns a transient HTTP error code.
	if rateLimitExceeded(gerr) {
		return false, errors.Fmt("rate limit exceeded creating instance %s: %w", vm.Hostname, err)
	}
	if stockout && hasFallbacks {
		return rotateVMZone(ctx, vm, attemptVM, targetZone, nextZone, reason)
	}
	if gerr.Code == http.StatusTooManyRequests || gerr.Code >= 500 {
		return false, errors.Fmt("transiently failed to create instance %s: %w", vm.Hostname, err)
	}
	logging.Debugf(ctx, "Create instance %q: try to delete instance as got error during creation.", vm.Hostname)
	if delErr := deleteVM(ctx, vm.ID, vm.Hostname); delErr != nil {
		logging.Errorf(ctx, "Create instance %q: failed to delete instance %s", vm.Hostname, delErr)
	}
	return false, errors.Fmt("failed to create instance %s: %w", vm.Hostname, err)
}

// handleOperationErrors processes errors inside a GCP Operation returned by
// InsertInstance, recording failure and fallback metrics and returning whether
// the caller should retry on nextZone or abort with an error.
func handleOperationErrors(ctx context.Context, vm, attemptVM *model.VM, opErrors []CommonOpError, targetZone, nextZone string, hasFallbacks bool) (bool, error) {
	logging.Debugf(ctx, "Create instance %q: failed to create instance total %d error received", vm.Hostname, len(opErrors))
	for _, err := range opErrors {
		logging.Errorf(ctx, "create instance %q: failed with code %s: Message %s", vm.Hostname, err.Code, err.Message)
	}
	stockout, reason := isStockoutOperationErrors(opErrors)
	metrics.UpdateFailures(ctx, 200, reason, attemptVM)
	if stockout && hasFallbacks {
		return rotateVMZone(ctx, vm, attemptVM, targetZone, nextZone, reason)
	}
	if err := deleteVM(ctx, vm.ID, vm.Hostname); err != nil {
		return false, errors.Fmt("failed to create instance %s: %w", vm.Hostname, err)
	}
	return false, errors.Fmt("failed to create instance %s", vm.Hostname)
}

// cloneVMForZone returns a shallow copy of vm with a deep-copied Attributes
// proto bound to the given target zone via SetZone.
func cloneVMForZone(vm *model.VM, zone string) *model.VM {
	cloned := *vm
	if attrs, ok := proto.Clone(&vm.Attributes).(*config.VM); ok && attrs != nil {
		attrs.SetZone(zone)
		cloned.Attributes = *attrs
	}
	return &cloned
}

// setActiveVMZone persists the active candidate zone, original zone, failed
// zone trace, and rotation count to the VM entity in datastore while an
// asynchronous GCE creation operation is pending or wrapping around. This
// preserves "{{.Zone}}" templates in disk and machine types so subsequent
// fallback zones can still be expanded.
func setActiveVMZone(ctx context.Context, id, zone, origZone string, failedZones []string, rotations int64) error {
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		cur := &model.VM{ID: id}
		if err := datastore.Get(ctx, cur); err != nil {
			return errors.Fmt("failed to fetch VM with id %q: %w", id, err)
		}
		cur.Attributes.Zone = zone
		if origZone != "" {
			cur.FallbackOriginalZone = origZone
		}
		cur.FallbackFailedZones = append([]string(nil), failedZones...)
		cur.FallbackZoneRotations = rotations
		if err := datastore.Put(ctx, cur); err != nil {
			return errors.Fmt("failed to store VM %q: %w", id, err)
		}
		return nil
	}, nil)
}

// updateVMZone persists the selected zone, expanded "{{.Zone}}" templates, and
// zone rotation history to the VM entity in datastore so subsequent check,
// audit, and destroy tasks target the zone where the instance was created.
func updateVMZone(ctx context.Context, id, zone, origZone string, failedZones []string, rotations int64) error {
	return datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		cur := &model.VM{ID: id}
		if err := datastore.Get(ctx, cur); err != nil {
			return errors.Fmt("failed to fetch VM with id %q: %w", id, err)
		}
		if origZone != "" {
			cur.FallbackOriginalZone = origZone
		}
		cur.FallbackFailedZones = append([]string(nil), failedZones...)
		cur.FallbackZoneRotations = rotations
		cur.Attributes.SetZone(zone)
		cur.IndexAttributes()
		if err := datastore.Put(ctx, cur); err != nil {
			return errors.Fmt("failed to store VM %q: %w", id, err)
		}
		return nil
	}, nil)
}

// insertInstanceWithFallback attempts to insert the GCE instance across the
// VM's candidate zones, rotating on stockout errors and persisting either the
// finalized zone (when op is DONE) or the active zone cursor (when op is still
// pending).
func insertInstanceWithFallback(ctx context.Context, vm *model.VM) (Operation, error) {
	origZone := originalVMZone(vm)
	vm.FallbackOriginalZone = origZone
	initialZone := vm.Attributes.GetZone()
	initialRotations := vm.FallbackZoneRotations
	candidates := candidateZones(vm)
	hasFallbacks := len(vm.Attributes.GetFallbackZones()) > 0
	srv := getCompute(ctx)

	var op Operation
	var targetZone string
	for i, zone := range candidates {
		targetZone = zone
		var nextZone string
		if i+1 < len(candidates) {
			nextZone = candidates[i+1]
		}
		attemptVM := cloneVMForZone(vm, targetZone)
		instance := attemptVM.GetInstance()
		rID := createInstanceRequestID(vm, targetZone)

		var err error
		op, err = srv.InsertInstance(ctx, attemptVM.Attributes.GetProject(), targetZone, instance, rID.String())
		if err != nil {
			fallback, retErr := handleInsertError(ctx, vm, attemptVM, err, targetZone, nextZone, hasFallbacks)
			if fallback {
				continue
			}
			return op, retErr
		}
		logging.Debugf(ctx, "Create instance %q: received response from GCP, waiting execution", vm.Hostname)
		if operationsErrors := op.GetErrors(); len(operationsErrors) > 0 {
			fallback, retErr := handleOperationErrors(ctx, vm, attemptVM, operationsErrors, targetZone, nextZone, hasFallbacks)
			if fallback {
				continue
			}
			return op, retErr
		}
		break
	}
	if op.GetStatus() == "DONE" {
		if targetZone != initialZone || hasFallbacks || vm.FallbackZoneRotations > 0 {
			if err := updateVMZone(ctx, vm.ID, targetZone, origZone, vm.FallbackFailedZones, vm.FallbackZoneRotations); err != nil {
				return op, errors.Fmt("failed to update zone for instance %s: %w", vm.Hostname, err)
			}
			vm.Attributes.SetZone(targetZone)
		}
		return op, nil
	}
	if targetZone != initialZone || vm.FallbackZoneRotations != initialRotations {
		if err := setActiveVMZone(ctx, vm.ID, targetZone, origZone, vm.FallbackFailedZones, vm.FallbackZoneRotations); err != nil {
			return op, errors.Fmt("failed to set active zone for instance %s: %w", vm.Hostname, err)
		}
		vm.Attributes.Zone = targetZone
	}
	return op, nil
}
