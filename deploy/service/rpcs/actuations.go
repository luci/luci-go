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

package rpcs

import (
	"context"
	"sort"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/deploy/api/modelpb"
	"go.chromium.org/luci/deploy/api/rpcpb"
	"go.chromium.org/luci/deploy/service/model"
)

// Actuations is an implementation of deploy.service.Actuations service.
type Actuations struct {
	rpcpb.UnimplementedActuationsServer
}

// BeginActuation implements the corresponding RPC method.
func (srv *Actuations) BeginActuation(ctx context.Context, req *rpcpb.BeginActuationRequest) (resp *rpcpb.BeginActuationResponse, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	assetIDs, err := validateBeginActuation(req)
	if err != nil {
		return nil, err
	}

	// Fill in the server enforced fields.
	req.Actuation.Actuator.Identity = string(auth.CurrentIdentity(ctx))

	// TODO: Store all new artifacts from req.Artifacts.

	err = model.Txn(ctx, func(ctx context.Context) error {
		// Check if there's already such actuation. This RPC may be a retry.
		act := model.Actuation{ID: req.Actuation.Id}
		switch err := datastore.Get(ctx, &act); {
		case err == nil:
			existing := act.Actuation.GetActuator().GetIdentity()
			if existing == req.Actuation.Actuator.Identity {
				// The same caller: most likely a retry, return the previous response.
				logging.Warningf(ctx, "The actuation %q already exists, assuming it is a retry", req.Actuation.Id)
				resp = &rpcpb.BeginActuationResponse{Decisions: act.Decisions.Decisions}
				return nil
			}
			// A different caller: some kind of a conflict.
			return status.Errorf(codes.FailedPrecondition, "actuation %q already exists and was created by %q", req.Actuation.Id, existing)
		case err != datastore.ErrNoSuchEntity:
			return transient.Tag.Apply(errors.Fmt("fetching Actuation: %w", err))
		}

		// Start a new operation that would update Asset and Actuation entities.
		// Copy only subset of Actuation fields allowed to be set by the actuator.
		op, err := model.NewActuationBeginOp(ctx, assetIDs, &modelpb.Actuation{
			Id:         req.Actuation.Id,
			Deployment: req.Actuation.Deployment,
			Actuator:   req.Actuation.Actuator,
			Triggers:   req.Actuation.Triggers,
			LogUrl:     req.Actuation.LogUrl,
		})
		if err != nil {
			return errors.Fmt("creating Actuation: %w", err)
		}
		for assetID, assetToActuate := range req.Assets {
			op.MakeDecision(ctx, assetID, assetToActuate)
		}
		decisions, err := op.Apply(ctx)
		if err != nil {
			return errors.Fmt("applying changes: %w", err)
		}
		resp = &rpcpb.BeginActuationResponse{Decisions: decisions}
		return nil
	})

	return resp, err
}

// EndActuation implements the corresponding RPC method.
func (srv *Actuations) EndActuation(ctx context.Context, req *rpcpb.EndActuationRequest) (resp *rpcpb.EndActuationResponse, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	assetIDs, err := validateEndActuation(req)
	if err != nil {
		return nil, err
	}

	err = model.Txn(ctx, func(ctx context.Context) error {
		// The actuation must exist already.
		act := &model.Actuation{ID: req.ActuationId}
		switch err := datastore.Get(ctx, act); {
		case err == datastore.ErrNoSuchEntity:
			return status.Errorf(codes.NotFound, "actuation %q doesn't exists", req.ActuationId)
		case err != nil:
			return transient.Tag.Apply(errors.Fmt("fetching Actuation: %w", err))
		}

		// The actuation must have been started by the same identity.
		startedBy := act.Actuation.GetActuator().GetIdentity()
		if startedBy != string(auth.CurrentIdentity(ctx)) {
			return status.Errorf(codes.FailedPrecondition, "actuation %q was created by %q", req.ActuationId, startedBy)
		}

		// If the actuation is already in the terminal state, assume this RPC is
		// a retry and just ignore it.
		if act.Actuation.State != modelpb.Actuation_EXECUTING {
			logging.Warningf(ctx, "The actuation is in the terminal state %q already, skipping the call", act.Actuation.State)
			return nil
		}

		// The set of reported assets must match exactly the set of actively
		// actuated assets as reported by the previous BeginActuation.
		if expected := act.ActuatedAssetIDs(); !model.EqualStrSlice(assetIDs, expected) {
			return status.Errorf(codes.InvalidArgument,
				"the reported set of actuated assets doesn't match the set previously returned by BeginActuation: %q != %q",
				assetIDs, expected)
		}

		// Mutate the state of all actuated assets.
		op, err := model.NewActuationEndOp(ctx, act)
		if err != nil {
			return errors.Fmt("finalizing Actuation: %w", err)
		}
		op.UpdateActuationStatus(ctx, req.Status, req.LogUrl)
		for assetID, actuatedAsset := range req.Assets {
			op.HandleActuatedState(ctx, assetID, actuatedAsset)
		}
		if err := op.Apply(ctx); err != nil {
			return errors.Fmt("applying changes: %w", err)
		}
		return nil
	})

	return &rpcpb.EndActuationResponse{}, err
}

////////////////////////////////////////////////////////////////////////////////

// validateBeginActuation checks the format of the RPC request.
//
// Returns the sorted list of asset IDs on success or gRPC error on failure.
func validateBeginActuation(req *rpcpb.BeginActuationRequest) ([]string, error) {
	actuationPb := req.Actuation
	if actuationPb.GetId() == "" {
		return nil, status.Errorf(codes.InvalidArgument, "`id` is required")
	}
	if actuationPb.GetDeployment() == nil {
		return nil, status.Errorf(codes.InvalidArgument, "`deployment` is required")
	}
	if actuationPb.GetActuator() == nil {
		return nil, status.Errorf(codes.InvalidArgument, "`actuator` is required")
	}

	// Validate AssetState has correct fields populated.
	if len(req.Assets) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "`assets` is required")
	}
	assetIDs := make([]string, 0, len(req.Assets))
	for assetID, assetToActuate := range req.Assets {
		if err := model.ValidateIntendedState(assetID, assetToActuate.IntendedState); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "asset %q: bad intended state - %s", assetID, err)
		}
		if err := model.ValidateReportedState(assetID, assetToActuate.ReportedState); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "asset %q: bad reported state - %s", assetID, err)
		}
		assetIDs = append(assetIDs, assetID)
	}
	sort.Strings(assetIDs)

	return assetIDs, nil
}

// validateEndActuation checks the format of the RPC request.
//
// Returns the sorted list of asset IDs on success or gRPC error on failure.
func validateEndActuation(req *rpcpb.EndActuationRequest) ([]string, error) {
	// Validate the request.
	if req.ActuationId == "" {
		return nil, status.Errorf(codes.InvalidArgument, "`actuation_id` is required")
	}

	// Validate AssetState has correct fields populated.
	if len(req.Assets) == 0 {
		return nil, status.Errorf(codes.InvalidArgument, "`assets` is required")
	}
	assetIDs := make([]string, 0, len(req.Assets))
	for assetID, actuatedAsset := range req.Assets {
		if err := model.ValidateReportedState(assetID, actuatedAsset.State); err != nil {
			return nil, status.Errorf(codes.InvalidArgument, "asset %q: bad reported state - %s", assetID, err)
		}
		assetIDs = append(assetIDs, assetID)
	}
	sort.Strings(assetIDs)

	return assetIDs, nil
}
