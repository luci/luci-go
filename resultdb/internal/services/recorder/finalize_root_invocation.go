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

// FinalizeRootInvocation implements pb.RecorderServer.
func (s *recorderServer) FinalizeRootInvocation(ctx context.Context, in *pb.FinalizeRootInvocationRequest) (*pb.RootInvocation, error) {
	if err := validateFinalizeRootInvocationRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	// TODO: Validate update token.

	return nil, appstatus.Error(codes.Unimplemented, "not yet implemented")
}

func validateFinalizeRootInvocationRequest(req *pb.FinalizeRootInvocationRequest) error {
	if err := pbutil.ValidateRootInvocationName(req.Name); err != nil {
		return errors.Fmt("name: %w", err)
	}
	return nil
}
