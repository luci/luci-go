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
	now       time.Time

	affected []*Asset // assets that were modified
}

// NewActuationEndOp starts a datastore operation to finish an actuation.
//
// Takes ownership of `actuation` mutating it.
func NewActuationEndOp(ctx context.Context, assets []string, actuation *Actuation) (*ActuationEndOp, error) {
	assetMap, err := fetchAssets(ctx, assets, true)
	if err != nil {
		return nil, err
	}
	return &ActuationEndOp{
		actuation: actuation,
		assets:    assetMap,
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
// Must be called once for every asset passed to NewActuationEndOp. Takes
// ownership of `asset` mutating it.
func (op *ActuationEndOp) HandleActuatedState(ctx context.Context, assetID string, asset *rpcpb.ActuatedAsset) {
	// Skip assets that are already handled by another actuation. Can happen due
	// to races, crashes, expirations, etc.
	ent := op.assets[assetID]
	if ent.Asset.LastActuation.GetId() != op.actuation.ID {
		logging.Warningf(ctx, "Skipping asset %q: it was already handled by another actuation %q", assetID, ent.Asset.LastActuation.GetId())
		return
	}
	op.affected = append(op.affected, ent)

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
	}
}

// Apply stores all updated or created datastore entities.
func (op *ActuationEndOp) Apply(ctx context.Context) error {
	var toPut []interface{}

	// Embed the Actuation snapshot into the affected Asset entities.
	for _, ent := range op.affected {
		ent.Asset.LastActuation = op.actuation.Actuation
		if ent.Asset.LastActuateActuation.GetId() == op.actuation.ID {
			ent.Asset.LastActuateActuation = op.actuation.Actuation
		}
		if op.actuation.Actuation.State == modelpb.Actuation_SUCCEEDED {
			ent.Asset.AppliedState = ent.Asset.IntendedState
		}
		toPut = append(toPut, ent)
	}

	// Update the actuation entity to match Actuation proto.
	op.actuation.State = op.actuation.Actuation.State
	toPut = append(toPut, op.actuation)

	return datastore.Put(ctx, toPut...)
}
