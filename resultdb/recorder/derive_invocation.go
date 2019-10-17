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
	"golang.org/x/net/context"
	"strings"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb"
	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/recorder/chromium"
)

var urlPrefixes = []string{"http://", "https://"}

// validateDeriveInvocationRequest returns an error if req is invalid.
func validateDeriveInvocationRequest(req *pb.DeriveInvocationRequest) error {
	if req.SwarmingTask == nil {
		return errors.Reason("swarming_task missing").Err()
	}

	if req.SwarmingTask.Hostname == "" {
		return errors.Reason("swarming_task.hostname missing").Err()
	}

	for _, prefix := range urlPrefixes {
		if strings.HasPrefix(req.SwarmingTask.Hostname, prefix) {
			return errors.Reason("swarming_task.hostname should not have prefix %q", prefix).Err()
		}
	}

	if req.SwarmingTask.Id == "" {
		return errors.Reason("swarming_task.id missing").Err()
	}

	if err := resultdb.VariantDefMap(req.GetBaseTestVariant().GetDef()).Validate(); err != nil {
		return errors.Annotate(err, "base_test_variant").Err()
	}

	return nil
}

// DeriveInvocation derives the invocation associated with the given swarming task.
//
// If the task is a dedup of another task, the invocation returned is the underlying one; otherwise,
// the invocation returned is associated with the swarming task itself.
func (s *RecorderServer) DeriveInvocation(ctx context.Context, in *pb.DeriveInvocationRequest) (*pb.Invocation, error) {
	if err := validateDeriveInvocationRequest(in); err != nil {
		return nil, errors.Annotate(err, "bad request").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	// Get the swarming service to use.
	swarmingURL := "https://" + in.SwarmingTask.Hostname
	swarmSvc, err := chromium.GetSwarmSvc(internal.HTTPClient(ctx), swarmingURL)
	if err != nil {
		return nil, err
	}

	// Get the swarming task, deduping if necessary.
	task, err := chromium.GetSwarmingTask(ctx, in.SwarmingTask.Id, swarmSvc)
	if err != nil {
		return nil, err
	}
	if task, err = chromium.GetOriginTask(ctx, task, swarmSvc); err != nil {
		return nil, err
	}

	// Check if we even need to write this invocation: is it finalized?
	// TODO(jchinlee): Get Invocation from Spanner.
	var inv *pb.Invocation
	if inv != nil {
		if !pbutil.IsFinalized(inv.State) {
			return nil, errors.Reason(
				"attempting to derive an existing non-finalized invocation").
				Tag(grpcutil.InternalTag).Err()
		}

		return inv, nil
	}

	inv, _, err = chromium.DeriveProtosForWriting(ctx, task, in)
	if err != nil {
		return nil, err
	}

	// TODO(jchinlee): Write Invocation and TestResults to Spanner.

	return inv, err
}
