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
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// validateGetInvocationRequest returns an error if req is invalid.
func validateGetInvocationRequest(req *pb.GetInvocationRequest) error {
	if req.GetName() == "" {
		return errors.Reason("name missing").Err()
	}

	if err := pbutil.ValidateInvocationName(req.Name); err != nil {
		return errors.Annotate(err, "name").Err()
	}

	return nil
}

// GetInvocation implements pb.ResultDBServer.
func (s *resultDBServer) GetInvocation(ctx context.Context, in *pb.GetInvocationRequest) (*pb.Invocation, error) {
	if err := validateGetInvocationRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	txn := span.Client(ctx).ReadOnlyTransaction()
	defer txn.Close()
	return span.ReadInvocationFull(ctx, txn, span.MustParseInvocationName(in.Name))
}
