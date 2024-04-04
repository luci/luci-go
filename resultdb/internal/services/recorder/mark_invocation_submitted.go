// Copyright 2023 The LUCI Authors.
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
	"fmt"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/realms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/services/baselineupdater"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func noPermissionsError(inv invocations.ID) string {
	return fmt.Sprintf(`Caller does not have permission to mark invocations/%s submitted (or it might not exist)`, string(inv))
}

func validateMarkInvocationSubmittedPermissions(ctx context.Context, inv invocations.ID) error {
	readCtx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	invRealm, err := invocations.ReadRealm(readCtx, inv)
	if err != nil {
		// If the invocation does not exist, we mask the error with permission
		// denied to avoid leaking resource existence.
		if grpcutil.Code(err) == codes.NotFound {
			return appstatus.Errorf(codes.PermissionDenied, noPermissionsError(inv))
		} else {
			return err
		}
	}

	// Parse the realm to check using format <project>:@project.
	project, _ := realms.Split(invRealm)
	realm := realms.Join(project, realms.ProjectRealm)
	if err := realms.ValidateRealmName(realm, realms.GlobalScope); err != nil {
		return errors.Annotate(err, "invocation: realm").Err()
	}

	switch allowed, err := auth.HasPermission(ctx, permSetSubmittedInvocation, realm, nil); {
	case err != nil:
		return err
	case !allowed:
		return appstatus.Errorf(codes.PermissionDenied, noPermissionsError(inv))
	}

	return nil
}

func markInvocationSubmitted(ctx context.Context, invocation invocations.ID) {
	values := map[string]any{
		"InvocationId": invocation,
		"Submitted":    true,
	}

	span.BufferWrite(ctx, spanutil.UpdateMap("Invocations", values))
}

// MarkInvocationSubmitted implements pb.RecorderServer.
// This adds the test variants to the BaselineTestVariant table if they
// don't exist, or updating the LastUpdated so that its retention is updated.
// The invocation must already be finalized.
func (s *recorderServer) MarkInvocationSubmitted(ctx context.Context, req *pb.MarkInvocationSubmittedRequest) (*emptypb.Empty, error) {
	// Parsing the invocation name also validates its syntax.
	invID, err := pbutil.ParseInvocationName(req.Invocation)
	if err != nil {
		return &emptypb.Empty{}, appstatus.Error(codes.InvalidArgument, errors.Annotate(err, "invocation").Err().Error())
	}
	inv := invocations.ID(invID)

	if err = validateMarkInvocationSubmittedPermissions(ctx, inv); err != nil {
		return &emptypb.Empty{}, err
	}

	_, err = span.ReadWriteTransaction(ctx, func(ctx context.Context) error {
		submitted, err := invocations.ReadSubmitted(ctx, inv)
		if err != nil {
			return err
		}
		if submitted {
			// Invocation already marked submitted
			return nil
		}

		markInvocationSubmitted(ctx, inv)

		state, err := invocations.ReadState(ctx, inv)
		if err != nil {
			return err
		}
		if state != pb.Invocation_FINALIZED {
			// Finalizer will schedule the task if it has not been finalized yet.
			return nil
		}

		// Add this request to the task queue.
		baselineupdater.Schedule(ctx, invID)
		return nil
	})
	if err != nil {
		return &emptypb.Empty{}, err
	}
	return &emptypb.Empty{}, nil
}
