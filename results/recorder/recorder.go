// Copyright 2019 The LUCI Authors.
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

package main

import (
	"context"
	"errors"

	empty "github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/luci/common/logging"

	resultspb "go.chromium.org/luci/results/proto/v1"
)

// RecorderServer implements resultspb.Recorder API.
//
// This is not typically used directly by the end client, but by intermediaries
// such as the Uploader, which handles uploading test results from swarming
// tasks.
type RecorderServer struct {
}

// DeriveInvocationFromSwarming derives an invocation given swarming task ID (see corresponding RPC).
func (s *RecorderServer) DeriveInvocationFromSwarming(ctx context.Context, in *resultspb.DeriveInvocationFromSwarmingRequest) (*resultspb.Invocation, error) {
	logging.Warningf(ctx, "InsertInvocationFromSwarming called with ID %s", in.Task)
	return nil, errors.New("unimplemented")
}

// InsertInvocation creates a new invocation (see corresponding RPC).
func (s *RecorderServer) InsertInvocation(ctx context.Context, in *resultspb.Invocation) (*resultspb.Invocation, error) {
	logging.Warningf(ctx, "InsertInvocation called")
	return nil, errors.New("unimplemented")
}

// UpdateInvocation updates an existing non-final invocation (see corresponding RPC).
func (s *RecorderServer) UpdateInvocation(ctx context.Context, in *resultspb.UpdateInvocationRequest) (*empty.Empty, error) {
	logging.Warningf(ctx, "UpdateInvocation called")
	return nil, errors.New("unimplemented")
}
