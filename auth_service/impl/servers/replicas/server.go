// Copyright 2024 The LUCI Authors.
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

// Package replicas contains Replicas server implementation.
package replicas

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/model"
)

type Server struct {
	rpcpb.UnimplementedReplicasServer
}

func (*Server) ListReplicas(ctx context.Context, _ *emptypb.Empty) (*rpcpb.ListReplicasResponse, error) {
	var replicas []*model.AuthReplicaState
	var primaryState *model.AuthReplicationState
	var dsErr error
	err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
		// Get all replicas.
		replicas, dsErr = model.GetAllReplicas(ctx)
		if dsErr != nil {
			return errors.Annotate(dsErr, "failed getting replica states").Err()
		}

		primaryState, dsErr = model.GetReplicationState(ctx)
		if dsErr != nil {
			return errors.Annotate(dsErr, "failed getting primary replication state").Err()
		}

		return nil
	}, &datastore.TransactionOptions{ReadOnly: true})
	if err != nil {
		return nil, status.Errorf(codes.Internal, err.Error())
	}

	replicaStates := make([]*rpcpb.ReplicaState, len(replicas))
	for i, replica := range replicas {
		replicaStates[i] = replica.ToProto()
	}

	return &rpcpb.ListReplicasResponse{
		PrimaryRevision: &protocol.AuthDBRevision{
			PrimaryId:  info.AppID(ctx),
			AuthDbRev:  primaryState.AuthDBRev,
			ModifiedTs: primaryState.ModifiedTS.UnixMicro(),
		},
		AuthCodeVersion: model.AuthAPIVersion,
		Replicas:        replicaStates,
		ProcessedAt:     timestamppb.New(clock.Now(ctx)),
	}, nil
}
