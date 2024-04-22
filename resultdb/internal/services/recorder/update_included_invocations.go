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
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/services/exportnotifier"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/tasks/taskspb"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// validateUpdateIncludedInvocationsRequest returns a non-nil error if req is
// determined to be invalid.
func validateUpdateIncludedInvocationsRequest(req *pb.UpdateIncludedInvocationsRequest) error {
	if _, err := pbutil.ParseInvocationName(req.IncludingInvocation); err != nil {
		return errors.Annotate(err, "including_invocation").Err()
	}
	for _, name := range req.AddInvocations {
		if name == req.IncludingInvocation {
			return errors.Reason("cannot include itself").Err()
		}
		if _, err := pbutil.ParseInvocationName(name); err != nil {
			return errors.Annotate(err, "add_invocations: %q", name).Err()
		}
	}

	if len(req.RemoveInvocations) > 0 {
		return errors.Reason("remove_invocations: invocation removal has been deprecated and is not permitted").Err()
	}

	return nil
}

// UpdateIncludedInvocations implements pb.RecorderServer.
func (s *recorderServer) UpdateIncludedInvocations(ctx context.Context, in *pb.UpdateIncludedInvocationsRequest) (*emptypb.Empty, error) {
	if err := validateUpdateIncludedInvocationsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	including := invocations.MustParseName(in.IncludingInvocation)
	add := invocations.MustParseNames(in.AddInvocations)

	err := mutateInvocation(ctx, including, func(ctx context.Context) error {
		// To include invocation A into invocation B, in addition to checking the
		// update token for B in mutateInvocation below, verify that the caller has
		// permission 'resultdb.invocation.include' on A's realm.
		// Perform this check in the same transaction as the update to avoid
		// TOC-TOU vulnerabilities.
		if err := permissions.VerifyInvocations(ctx, add, permIncludeInvocation); err != nil {
			return err
		}

		ms := make([]*spanner.Mutation, 0, len(add))

		switch states, err := invocations.ReadStateBatch(ctx, add); {
		case err != nil:
			return err
		// Ensure every included invocation exists.
		case len(states) != len(add):
			return appstatus.Errorf(codes.NotFound, "at least one of the included invocations does not exist")
		}
		var addedInvocationIDs []string
		for aInv := range add {
			ms = append(ms, spanutil.InsertOrUpdateMap("IncludedInvocations", map[string]any{
				"InvocationId":         including,
				"IncludedInvocationId": aInv,
			}))
			addedInvocationIDs = append(addedInvocationIDs, string(aInv))
		}
		span.BufferWrite(ctx, ms...)

		// Invocations may have been added to export root(s) directly
		// or indirectly. Send notifications as appropriate.
		exportnotifier.EnqueueTask(ctx, &taskspb.RunExportNotifications{
			InvocationId:          string(including),
			IncludedInvocationIds: addedInvocationIDs,
		})

		return nil
	})

	return &emptypb.Empty{}, err
}
