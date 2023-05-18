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

package rpc

import (
	"context"
	"encoding/hex"

	"go.chromium.org/luci/analysis/internal/changepoints/server"
	tvbr "go.chromium.org/luci/analysis/internal/changepoints/testvariantbranch"
	pb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/common/errors"
)

// NewTestVariantBranchesServer returns a new pb.TestVariantBranchesServer.
func NewTestVariantBranchesServer() pb.TestVariantBranchesServer {
	return &pb.DecoratedTestVariantBranches{
		// Currently, we only allow LUCI Analysis admins to use this API.
		// In the future, if this end point is used for the UI, we should
		// have proper ACL check.
		Prelude:  checkAllowedAdminPrelude,
		Service:  &testVariantBranchesServer{},
		Postlude: gRPCifyAndLogPostlude,
	}
}

// testVariantBranchesServer implements pb.TestVariantAnalysesServer.
type testVariantBranchesServer struct {
}

// Get fetches Spanner for test variant analysis.
func (*testVariantBranchesServer) Get(ctx context.Context, req *pb.GetTestVariantBranchRequest) (*pb.TestVariantBranch, error) {
	tvbk, err := validateGetTestVariantBranchRequest(req)
	if err != nil {
		return nil, invalidArgumentError(err)
	}
	return server.GetTestVariantBranch(ctx, tvbk)
}

func validateGetTestVariantBranchRequest(req *pb.GetTestVariantBranchRequest) (tvbr.TestVariantBranchKey, error) {
	project, testID, variantHash, refHash, err := parseTestVariantBranchName(req.Name)
	if err != nil {
		return tvbr.TestVariantBranchKey{}, errors.Annotate(err, "name").Err()
	}

	// Check ref hash.
	refHashBytes, err := hex.DecodeString(refHash)
	if err != nil {
		return tvbr.TestVariantBranchKey{}, errors.Reason("ref component must be an encoded hexadecimal string").Err()
	}
	return tvbr.TestVariantBranchKey{
		Project:     project,
		TestID:      testID,
		VariantHash: variantHash,
		RefHash:     tvbr.RefHash(refHashBytes),
	}, nil
}
