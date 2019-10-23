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

	"cloud.google.com/go/spanner"

	"github.com/golang/protobuf/ptypes"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// validateFinalizeInvocationRequest returns a non-nil error if req is determined
// to be invalid.
func validateFinalizeInvocationRequest(req *pb.FinalizeInvocationRequest) error {
	if _, err := pbutil.ParseInvocationName(req.Name); err != nil {
		return errors.Annotate(err, "name").Err()
	}

	return nil
}

// FinalizeInvocation implements pb.RecorderServer.
func (s *recorderServer) FinalizeInvocation(ctx context.Context, in *pb.FinalizeInvocationRequest) (*pb.Invocation, error) {
	if err := validateFinalizeInvocationRequest(in); err != nil {
		return nil, errors.Annotate(err, "bad request").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	invID := pbutil.MustParseInvocationName(in.Name)

	var ret *pb.Invocation
	err := mutateInvocation(ctx, invID, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		var err error
		if ret, err = span.ReadInvocationFull(ctx, txn, invID); err != nil {
			return err
		}

		state := pb.Invocation_COMPLETED
		if in.Interrupted {
			state = pb.Invocation_INTERRUPTED
		}

		now := clock.Now(ctx)

		ret.State = state
		if ret.FinalizeTime, err = ptypes.TimestampProto(now); err != nil {
			return err
		}

		// TODO(chanli): Also update all inclusions that include this invocation.
		return txn.BufferWrite([]*spanner.Mutation{
			spanner.UpdateMap("Invocations", span.ToSpannerMap(map[string]interface{}{
				"InvocationId": invID,
				"State":        state,
				"FinalizeTime": now,
			})),
		})
	})

	if err != nil {
		return nil, err
	}
	return ret, nil
}
