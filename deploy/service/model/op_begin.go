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
	"fmt"
	"strings"
	"time"

	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/deploy/api/modelpb"
	"go.chromium.org/luci/deploy/api/rpcpb"
)

// ActuationExpiry is how long an actuation can run before considered failed.
//
// TODO: Implement expiration cron job or something.
const ActuationExpiry = 20 * time.Minute

// ActuationBeginOp collects changes to transactionally apply to the datastore
// to begin a new actuation.
type ActuationBeginOp struct {
	actuation *modelpb.Actuation
	decisions map[string]*modelpb.ActuationDecision
	assets    map[string]*Asset
	now       time.Time

	actuating bool     // true if have at least one ACTUATE_* decision
	errors    []string // accumulated errors for SKIP_BROKEN decisions
}

// NewActuationBeginOp starts a datastore operation to create an actuation.
//
// Takes ownership of `actuation` mutating it.
func NewActuationBeginOp(ctx context.Context, assets []string, actuation *modelpb.Actuation) (*ActuationBeginOp, error) {
	assetMap, err := fetchAssets(ctx, assets, false)
	if err != nil {
		return nil, err
	}
	return &ActuationBeginOp{
		actuation: actuation,
		decisions: make(map[string]*modelpb.ActuationDecision, len(assetMap)),
		assets:    assetMap,
		now:       clock.Now(ctx),
	}, nil
}

// MakeDecision decides what to do with an asset and records this decision.
//
// Must be called once for every asset passed to NewActuationBeginOp. Takes
// ownership of `asset` mutating it. AssetToActuate fields must already be
// validates at this point.
func (op *ActuationBeginOp) MakeDecision(ctx context.Context, assetID string, asset *rpcpb.AssetToActuate) {
	// TODO: Implement AssetState history.
	// TODO: Implement locks.
	// TODO: Implement anti-stomp protection.
	// TODO: Implement forced actuation.

	var errors []string
	var brokenStatus *statuspb.Status

	// Fill in server-assigned fields and collect error statuses.
	populateAssetState := func(what string, s *modelpb.AssetState) {
		if s.Timestamp == nil {
			s.Timestamp = timestamppb.New(op.now)
		}
		s.Deployment = op.actuation.Deployment
		s.Actuator = op.actuation.Actuator

		if s.Status.GetCode() != int32(codes.OK) {
			errors = append(errors, fmt.Sprintf(
				"asset %q: failed to collect %s: %s",
				assetID, what, status.ErrorProto(s.Status)))
			brokenStatus = s.Status // keep only the last, no big deal
		}
	}
	populateAssetState("intended state", asset.IntendedState)
	populateAssetState("reported state", asset.ReportedState)

	// Update stored AssetState fields only if the new reported values are
	// non-erroneous.
	stored := op.assets[assetID].Asset
	stored.Config = asset.Config
	if asset.IntendedState.Status.GetCode() == int32(codes.OK) {
		stored.IntendedState = asset.IntendedState
	}
	if asset.ReportedState.Status.GetCode() == int32(codes.OK) {
		stored.ReportedState = asset.ReportedState
	}

	// Make the actual decision.
	var decision modelpb.ActuationDecision_Decision
	switch {
	case !IsActuationEnabed(asset.Config, op.actuation.Deployment.Config):
		decision = modelpb.ActuationDecision_SKIP_DISABLED
	case len(errors) != 0:
		op.errors = append(op.errors, errors...)
		decision = modelpb.ActuationDecision_SKIP_BROKEN
	case IsUpToDate(asset.IntendedState, asset.ReportedState):
		decision = modelpb.ActuationDecision_SKIP_UPTODATE
	default:
		op.actuating = true
		decision = modelpb.ActuationDecision_ACTUATE_STALE
	}

	// Record the decision.
	stored.LastDecision = &modelpb.ActuationDecision{
		Decision: decision,
		Status:   brokenStatus,
	}
	op.decisions[assetID] = stored.LastDecision

	// TODO: Put `asset` and `stored.LastDecision` into the history log.
}

// Apply stores all updated or created datastore entities.
//
// Must be called only after all per-asset MakeDecision calls. Returns the
// mapping with recorded decisions.
func (op *ActuationBeginOp) Apply(ctx context.Context) (map[string]*modelpb.ActuationDecision, error) {
	var toPut []interface{}

	// Set the overall actuation state based on decisions made.
	op.actuation.Created = timestamppb.New(op.now)
	if op.actuating {
		op.actuation.State = modelpb.Actuation_EXECUTING
		op.actuation.Expiry = timestamppb.New(op.now.Add(ActuationExpiry))
	} else if len(op.errors) != 0 {
		op.actuation.State = modelpb.Actuation_FAILED
		op.actuation.Finished = timestamppb.New(op.now)
		op.actuation.Status = &statuspb.Status{
			Code:    int32(codes.Internal),
			Message: strings.Join(op.errors, "; "),
		}
	} else {
		op.actuation.State = modelpb.Actuation_SUCCEEDED
		op.actuation.Finished = timestamppb.New(op.now)
	}

	// Embed this Actuation snapshot into Asset entities.
	for _, ent := range op.assets {
		ent.Asset.LastActuation = op.actuation
		toPut = append(toPut, ent)
	}

	// Create the new actuation entity.
	toPut = append(toPut, &Actuation{
		ID:        op.actuation.Id,
		Actuation: op.actuation,
		Decisions: &modelpb.ActuationDecisions{Decisions: op.decisions},
		State:     op.actuation.State,
		Created:   asTime(op.actuation.Created),
		Expiry:    asTime(op.actuation.Expiry),
	})

	if err := datastore.Put(ctx, toPut...); err != nil {
		return nil, err
	}
	return op.decisions, nil
}
