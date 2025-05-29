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
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/deploy/api/modelpb"
	"go.chromium.org/luci/deploy/api/rpcpb"
	"go.chromium.org/luci/deploy/service/model"
)

// Assets is the implementation of deploy.service.Assets service.
type Assets struct {
	rpcpb.UnimplementedAssetsServer
}

// GetAsset implements the corresponding RPC method.
func (*Assets) GetAsset(ctx context.Context, req *rpcpb.GetAssetRequest) (resp *modelpb.Asset, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	asset, err := fetchAssetEntity(ctx, req.AssetId)
	if err != nil {
		return nil, err
	}
	return asset.Asset, nil
}

// ListAssets implements the corresponding RPC method.
func (*Assets) ListAssets(ctx context.Context, req *rpcpb.ListAssetsRequest) (resp *rpcpb.ListAssetsResponse, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	q := datastore.NewQuery("Asset")

	var entities []*model.Asset
	if err = datastore.GetAll(ctx, q, &entities); err != nil {
		return nil, status.Errorf(codes.Internal, "datastore query to list assets failed: %s", err)
	}

	assets, err := sortedProtoList(entities)
	if err != nil {
		return nil, err
	}

	return &rpcpb.ListAssetsResponse{Assets: assets}, nil
}

// ListAssetHistory implements the corresponding RPC method.
func (*Assets) ListAssetHistory(ctx context.Context, req *rpcpb.ListAssetHistoryRequest) (resp *rpcpb.ListAssetHistoryResponse, err error) {
	defer func() { err = grpcutil.GRPCifyAndLogErr(ctx, err) }()

	asset, err := fetchAssetEntity(ctx, req.AssetId)
	if err != nil {
		return nil, err
	}

	latestID := req.LatestHistoryId
	if latestID == 0 || latestID > asset.LastHistoryID {
		latestID = asset.LastHistoryID
	}

	if req.Limit == 0 {
		req.Limit = 20
	} else if req.Limit > 200 {
		req.Limit = 200
	} else if req.Limit < 0 {
		return nil, status.Errorf(codes.InvalidArgument, "limit can't be negative")
	}

	assetKey := datastore.KeyForObj(ctx, asset)
	q := datastore.NewQuery("AssetHistory").
		Ancestor(assetKey).
		Lte("__key__", datastore.NewKey(ctx, "AssetHistory", "", latestID, assetKey)).
		Order("-__key__").
		Limit(req.Limit)

	var entries []*model.AssetHistory
	if err := datastore.GetAll(ctx, q, &entries); err != nil {
		return nil, grpcutil.InternalTag.Apply(errors.Fmt("querying AssetHistory: %w", err))
	}

	resp = &rpcpb.ListAssetHistoryResponse{Asset: asset.Asset}
	if asset.IsRecordingHistoryEntry() {
		resp.Current = asset.HistoryEntry
	}
	resp.LastRecordedHistoryId = asset.LastHistoryID
	resp.History = make([]*modelpb.AssetHistory, len(entries))
	for i, e := range entries {
		resp.History[i] = e.Entry
	}
	return resp, nil
}

// fetchAssetEntity fetches Asset entity returning gRPC errors on failures.
func fetchAssetEntity(ctx context.Context, assetID string) (*model.Asset, error) {
	entity := &model.Asset{ID: assetID}
	switch err := datastore.Get(ctx, entity); {
	case err == datastore.ErrNoSuchEntity:
		return nil, status.Errorf(codes.NotFound, "no such asset")
	case err != nil:
		return nil, status.Errorf(codes.Internal, "datastore error when fetching the asset: %s", err)
	default:
		if _, err := checkAssetEntity(entity); err != nil {
			return nil, err
		}
		return entity, nil
	}
}

// checkAssetEntity checks the proto payload of the asset entity is correct.
//
// Returns gRPC errors.
func checkAssetEntity(e *model.Asset) (*modelpb.Asset, error) {
	if e.Asset.GetId() != e.ID {
		return nil, status.Errorf(codes.Internal, "asset entity %q has bad proto payload %v", e.ID, e.Asset)
	}
	return e.Asset, nil
}

// sortedProtoList extracts Asset protos and sorts them by ID.
//
// Returns gRPC errors.
func sortedProtoList(entities []*model.Asset) ([]*modelpb.Asset, error) {
	out := make([]*modelpb.Asset, len(entities))
	for i, ent := range entities {
		var err error
		if out[i], err = checkAssetEntity(ent); err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Id < out[j].Id
	})
	return out, nil
}
