// Copyright 2022 The LUCI Authors.
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

package model

import (
	"context"
	"time"

	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/deploy/api/modelpb"
	"go.chromium.org/luci/deploy/api/rpcpb"
)

// ActuationEndOp collects changes to transactionally apply to the datastore
// to end an existing actuation.
type ActuationEndOp struct {
	actuation *Actuation
	assets    map[string]*Asset
	history   *historyRecorder
	now       time.Time
}

// NewActuationEndOp starts a datastore operation to finish an actuation.
//
// Takes ownership of `actuation` mutating it.
func NewActuationEndOp(ctx context.Context, actuation *Actuation) (*ActuationEndOp, error) {
	// Fetch all assets associated with this actuation in BeginActuation. We'll
	// need to update their `Actuation` field with the final status of the
	// actuation.
	assets, err := fetchAssets(ctx, actuation.AssetIDs(), true)
	if err != nil {
		return nil, err
	}

	// Filter out assets that are already handled by another actuation. Can happen
	// due to races, crashes, expirations, etc.
	active := make(map[string]*Asset, len(assets))
	for assetID, ent := range assets {
		if ent.Asset.LastActuation.GetId() == actuation.ID {
			active[assetID] = ent
		} else {
			logging.Warningf(ctx, "Skipping asset %q: it was already handled by another actuation %q", assetID, ent.Asset.LastActuation.GetId())
		}
	}

	return &ActuationEndOp{
		actuation: actuation,
		assets:    active,
		history:   &historyRecorder{actuation: actuation.Actuation},
		now:       clock.Now(ctx),
	}, nil
}

// UpdateActuationStatus is called to report the status of the actuation.
func (op *ActuationEndOp) UpdateActuationStatus(ctx context.Context, status *statuspb.Status, logURL string) {
	op.actuation.Actuation.Finished = timestamppb.New(op.now)
	if status.GetCode() == int32(codes.OK) {
		op.actuation.Actuation.State = modelpb.Actuation_SUCCEEDED
	} else {
		op.actuation.Actuation.State = modelpb.Actuation_FAILED
		op.actuation.Actuation.Status = status
	}
	if logURL != "" {
		op.actuation.Actuation.LogUrl = logURL
	}
}

// HandleActuatedState is called to report the post-actuation asset state.
//
// Ignores assets not associated with this actuation anymore. Takes ownership
// of `asset` mutating it.
func (op *ActuationEndOp) HandleActuatedState(ctx context.Context, assetID string, asset *rpcpb.ActuatedAsset) {
	// If the asset is not in `op.assets`, then it is already handled by another
	// actuation and we should not modify it. Such assets were already logged in
	// NewActuationEndOp.
	ent, ok := op.assets[assetID]
	if !ok {
		return
	}

	reported := asset.State
	if reported.Timestamp == nil {
		reported.Timestamp = timestamppb.New(op.now)
	}
	reported.Deployment = op.actuation.Actuation.Deployment
	reported.Actuator = op.actuation.Actuation.Actuator

	ent.Asset.PostActuationStatus = reported.Status
	if reported.Status.GetCode() == int32(codes.OK) {
		ent.Asset.ReportedState = reported
		ent.Asset.ActuatedState = reported
		ent.ConsecutiveFailures = 0
	} else {
		ent.ConsecutiveFailures += 1
	}

	// If was recording a history entry, close and commit it.
	if ent.IsRecordingHistoryEntry() {
		ent.HistoryEntry.Actuation = op.actuation.Actuation
		ent.HistoryEntry.PostActuationState = reported
		op.history.recordAndNotify(ent.finalizeHistoryEntry())
	}
}

// Expire marks the actuation as expired.
func (op *ActuationEndOp) Expire(ctx context.Context) {
	op.actuation.Actuation.Finished = timestamppb.New(op.now)
	op.actuation.Actuation.State = modelpb.Actuation_EXPIRED
	op.actuation.Actuation.Status = &statuspb.Status{
		Code:    int32(codes.DeadlineExceeded),
		Message: "the actuation didn't finish before its expiration timeout",
	}

	// Append historic records to all assets that were being actuated.
	for _, asset := range op.assets {
		asset.ConsecutiveFailures += 1
		if asset.IsRecordingHistoryEntry() {
			asset.HistoryEntry.Actuation = op.actuation.Actuation
			op.history.recordAndNotify(asset.finalizeHistoryEntry())
		}
	}
}

// Apply stores all updated or created datastore entities.
func (op *ActuationEndOp) Apply(ctx context.Context) error {
	var toPut []interface{}

	// Embed the up-to-date Actuation snapshot into Asset entities.
	for _, ent := range op.assets {
		ent.Asset.LastActuation = op.actuation.Actuation
		if ent.Asset.LastActuateActuation.GetId() == op.actuation.ID {
			ent.Asset.LastActuateActuation = op.actuation.Actuation
		}
		if op.actuation.Actuation.State == modelpb.Actuation_SUCCEEDED {
			ent.Asset.AppliedState = ent.Asset.IntendedState
		}
		toPut = append(toPut, ent)
	}

	// Update the Actuation entity to match the Actuation proto.
	op.actuation.State = op.actuation.Actuation.State
	toPut = append(toPut, op.actuation)

	// Prepare AssetHistory entities. Note they refer to op.actuation by pointer
	// inside already and will pick up all changes made to the Actuation proto.
	history, err := op.history.commit(ctx)
	if err != nil {
		return err
	}
	toPut = append(toPut, history...)

	return datastore.Put(ctx, toPut...)
}
