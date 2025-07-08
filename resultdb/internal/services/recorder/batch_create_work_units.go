// Copyright 2025 The LUCI Authors.
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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// BatchCreateWorkUnits implements pb.RecorderServer.
func (s *recorderServer) BatchCreateWorkUnits(ctx context.Context, in *pb.BatchCreateWorkUnitsRequest) (*pb.BatchCreateWorkUnitsResponse, error) {
	if err := validateBatchCreateWorkUnitsPermissions(ctx, in); err != nil {
		return nil, err
	}
	if err := validateBatchCreateWorkUnitsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	// TODO: verify update tokens.

	return nil, appstatus.Error(codes.Unimplemented, "not yet implemented")
}

func validateBatchCreateWorkUnitsPermissions(ctx context.Context, req *pb.BatchCreateWorkUnitsRequest) error {
	// Only perform minimal validation necessary to verify permissions. Full validation
	// will be performed in validateBatchCreateWorkUnitsRequest.

	if err := pbutil.ValidateBatchRequestCount(len(req.Requests)); err != nil {
		return appstatus.BadRequest(errors.Fmt("requests: %w", err))
	}
	for i, r := range req.Requests {
		if err := verifyCreateWorkUnitPermissions(ctx, r); err != nil {
			// Wrap the app-status error if any by inserting the wrapping text inside the error.
			st, ok := appstatus.Get(err)
			if ok {
				return appstatus.Error(st.Code(), fmt.Sprintf("requests[%d]: %s", i, st.Message()))
			}
			return errors.Fmt("requests[%d]: %w", i, err)
		}
	}
	return nil
}

func validateBatchCreateWorkUnitsRequest(req *pb.BatchCreateWorkUnitsRequest) error {
	if req.RequestId == "" {
		// Request ID is required to ensure requests are treated idempotently
		// in case of inevitable retries.
		return errors.Fmt("request_id: unspecified (please provide a per-request UUID to ensure idempotence)")
	}
	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Fmt("request_id: %w", err)
	}

	if err := pbutil.ValidateBatchRequestCount(len(req.Requests)); err != nil {
		return errors.Fmt("requests: %w", err)
	}
	for i, r := range req.Requests {
		// Validate the sub-request.
		// The request ID is not specified on the sub-request as it is already
		// specified on the parent.
		if err := validateCreateWorkUnitRequest(r, false /*requireRequestID*/); err != nil {
			return errors.Fmt("requests[%d]: %w", i, err)
		}
		if r.RequestId != "" && r.RequestId != req.RequestId {
			return errors.Fmt("requests[%d]: request_id: inconsistent with top-level request_id", i)
		}
	}
	return nil
}
