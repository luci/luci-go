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

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/appstatus"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

func validateListTestExonerationsRequest(req *pb.ListTestExonerationsRequest) error {
	if err := pbutil.ValidateInvocationName(req.GetInvocation()); err != nil {
		return errors.Annotate(err, "invocation").Err()
	}

	if err := pagination.ValidatePageSize(req.GetPageSize()); err != nil {
		return errors.Annotate(err, "page_size").Err()
	}

	return nil
}

// ListTestExonerations implements pb.ResultDBServer.
func (s *resultDBServer) ListTestExonerations(ctx context.Context, in *pb.ListTestExonerationsRequest) (*pb.ListTestExonerationsResponse, error) {
	if err := validateListTestExonerationsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	q := span.TestExonerationQuery{
		InvocationIDs: span.NewInvocationIDSet(span.MustParseInvocationName(in.Invocation)),
		PageSize:      pagination.AdjustPageSize(in.PageSize),
		PageToken:     in.GetPageToken(),
	}

	txn := span.Client(ctx).ReadOnlyTransaction()
	defer txn.Close()
	tes, tok, err := span.QueryTestExonerations(ctx, txn, q)
	if err != nil {
		return nil, err
	}

	return &pb.ListTestExonerationsResponse{
		TestExonerations: tes,
		NextPageToken:    tok,
	}, nil
}
