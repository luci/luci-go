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

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/internal/tasks"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
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
		return nil, appstatus.BadRequest(err)
	}

	token, err := extractUpdateToken(ctx)
	if err != nil {
		return nil, err
	}

	invID := span.MustParseInvocationName(in.Name)
	if err := validateInvocationToken(ctx, token, invID); err != nil {
		return nil, appstatus.Errorf(codes.PermissionDenied, "invalid update token")
	}

	var ret *pb.Invocation
	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		inv, err := span.ReadInvocationFull(ctx, txn, invID)
		if err != nil {
			return err
		}
		ret = inv

		if ret.State != pb.Invocation_ACTIVE {
			// Idempotent.
			return nil
		}

		// Finalize as requested.
		ret.State = pb.Invocation_FINALIZING
		return tasks.StartInvocationFinalization(ctx, txn, invID)
	})

	if err != nil {
		return nil, err
	}

	return ret, nil
}
