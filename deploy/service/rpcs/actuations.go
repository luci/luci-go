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

	"google.golang.org/protobuf/encoding/prototext"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/deploy/api/rpcpb"
	"go.chromium.org/luci/server/auth"
)

// Actuations is an implementation of deploy.service.Actuations service.
type Actuations struct {
	rpcpb.UnimplementedActuationsServer
}

// BeginActuation implements the corresponding RPC method.
func (srv *Actuations) BeginActuation(ctx context.Context, req *rpcpb.BeginActuationRequest) (*rpcpb.BeginActuationResponse, error) {
	blob, err := (prototext.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
	}).Marshal(req)
	if err != nil {
		return nil, err
	}
	logging.Infof(ctx, "BeginActuation from %q:\n%s", auth.CurrentIdentity(ctx), blob)

	decisions := map[string]*rpcpb.ActuationDecision{}
	for name := range req.Assets {
		decisions[name] = &rpcpb.ActuationDecision{
			Decision: rpcpb.ActuationDecision_ACTUATE_FORCE,
		}
	}

	return &rpcpb.BeginActuationResponse{Decisions: decisions}, nil
}

// EndActuation implements the corresponding RPC method.
func (srv *Actuations) EndActuation(ctx context.Context, req *rpcpb.EndActuationRequest) (*rpcpb.EndActuationResponse, error) {
	blob, err := (prototext.MarshalOptions{
		Multiline: true,
		Indent:    "  ",
	}).Marshal(req)
	if err != nil {
		return nil, err
	}
	logging.Infof(ctx, "EndActuation from %q:\n%s", auth.CurrentIdentity(ctx), blob)
	return &rpcpb.EndActuationResponse{}, nil
}
