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

package recorder

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// validateFinalizeInvocationRequest returns a non-nil error if req is determined
// to be invalid.
func validateFinalizeInvocationRequest(req *pb.FinalizeInvocationRequest) error {
	if _, err := pbutil.ParseInvocationName(req.Name); err != nil {
		return errors.Fmt("name: %w", err)
	}

	return nil
}

// FinalizeInvocation implements pb.RecorderServer.
func (s *recorderServer) FinalizeInvocation(ctx context.Context, in *pb.FinalizeInvocationRequest) (*pb.Invocation, error) {
	if err := validateFinalizeInvocationRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	token, err := extractUpdateToken(ctx)
	if err != nil {
		return nil, err
	}

	invID := invocations.MustParseName(in.Name)
	if err := validateInvocationToken(ctx, token, invID); err != nil {
		return nil, appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}

	var ret *pb.Invocation
	commitTimestamp, err := span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		inv, err := invocations.Read(ctx, invID, invocations.ExcludeExtendedProperties)
		if err != nil {
			return err
		}
		ret = inv

		if ret.State != pb.Invocation_ACTIVE {
			// Finalization already started. Do not start finalization
			// again as doing so would overwrite the existing FinalizeStartTime
			// and create an unnecessary task.
			return nil
		}

		// Finalize as requested.
		ret.State = pb.Invocation_FINALIZING
		tasks.StartInvocationFinalization(ctx, invID, true)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if ret.FinalizeStartTime == nil {
		// We set the invocation to finalizing.
		ret.FinalizeStartTime = timestamppb.New(commitTimestamp)
	}

	return ret, nil
}
