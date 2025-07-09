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

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// BatchFinalizeWorkUnits implements pb.RecorderServer.
func (s *recorderServer) BatchFinalizeWorkUnits(ctx context.Context, in *pb.BatchFinalizeWorkUnitsRequest) (*pb.BatchFinalizeWorkUnitsResponse, error) {
	if err := validateBatchFinalizeWorkUnitsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	return nil, appstatus.Error(codes.Unimplemented, "not yet implemented")
}

func validateBatchFinalizeWorkUnitsRequest(req *pb.BatchFinalizeWorkUnitsRequest) error {
	if err := pbutil.ValidateBatchRequestCount(len(req.Requests)); err != nil {
		return errors.Fmt("requests: %w", err)
	}
	for i, r := range req.Requests {
		if err := validateFinalizeWorkUnitRequest(r); err != nil {
			return errors.Fmt("requests[%d]: %w", i, err)
		}
	}
	return nil
}
