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
	"fmt"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/appstatus"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// validateIncludeRequest returns a non-nil error if req is determined
// to be invalid.
func validateIncludeRequest(req *pb.IncludeRequest) error {
	if _, err := pbutil.ParseInvocationName(req.IncludingInvocation); err != nil {
		return errors.Annotate(err, "including_invocation").Err()
	}

	if _, err := pbutil.ParseInvocationName(req.IncludedInvocation); err != nil {
		return errors.Annotate(err, "included_invocation").Err()
	}

	if req.IncludedInvocation == req.IncludingInvocation {
		return errors.Reason("cannot include itself").Err()
	}

	return nil
}

var errRepeatedRequest = fmt.Errorf("this request was handled before")

// Include implements pb.RecorderServer.
func (s *recorderServer) Include(ctx context.Context, in *pb.IncludeRequest) (*empty.Empty, error) {
	if err := validateIncludeRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	including := span.MustParseInvocationName(in.IncludingInvocation)
	included := span.MustParseInvocationName(in.IncludedInvocation)

	err := mutateInvocation(ctx, including, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		// Ensure the included invocation exists and is finalized.
		switch includedState, err := span.ReadInvocationState(ctx, txn, included); {
		case err != nil:
			return err
		case includedState != pb.Invocation_FINALIZED:
			return appstatus.Errorf(codes.FailedPrecondition, "%s is not finalized", in.IncludedInvocation)
		}

		// Insert a new inclusion.
		return txn.BufferWrite([]*spanner.Mutation{
			span.InsertMap("IncludedInvocations", map[string]interface{}{
				"InvocationId":         including,
				"IncludedInvocationId": included,
			}),
		})
	})

	if spanner.ErrCode(err) == codes.AlreadyExists {
		// Perhaps this request was served before.
		err = nil
	}
	return &empty.Empty{}, err
}
