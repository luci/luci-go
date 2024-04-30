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

package resultdb

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/testresults"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func validateListTestResultsRequest(req *pb.ListTestResultsRequest) error {
	if err := pbutil.ValidateInvocationName(req.GetInvocation()); err != nil {
		return errors.Annotate(err, "invocation").Err()
	}

	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Annotate(err, "page_size").Err()
	}

	return nil
}

// ListTestResults implements pb.ResultDBServer.
func (s *resultDBServer) ListTestResults(ctx context.Context, in *pb.ListTestResultsRequest) (*pb.ListTestResultsResponse, error) {
	// Use one transaction for the entire RPC so that we work with a
	// consistent snapshot of the system state. This is important to
	// prevent subtle bugs and TOC-TOU vulnerabilities.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	if err := permissions.VerifyInvocationByName(ctx, in.Invocation, rdbperms.PermListTestResults); err != nil {
		return nil, err
	}

	if err := validateListTestResultsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	readMask, err := testresults.ListMask(in.GetReadMask())
	if err != nil {
		return nil, appstatus.BadRequest(err)
	}

	q := testresults.Query{
		PageSize:      pagination.AdjustPageSize(in.PageSize),
		PageToken:     in.PageToken,
		InvocationIDs: invocations.NewIDSet(invocations.MustParseName(in.Invocation)),
		Mask:          readMask,
	}
	trs, tok, err := q.Fetch(ctx)
	if err != nil {
		return nil, err
	}

	return &pb.ListTestResultsResponse{
		TestResults:   trs,
		NextPageToken: tok,
	}, nil
}
