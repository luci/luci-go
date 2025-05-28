// Copyright 2024 The LUCI Authors.
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
	"go.chromium.org/luci/resultdb/internal/invocations/graph"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

// validateQueryRootInvocationNamesRequest returns an error if req is invalid.
func validateQueryRootInvocationNamesRequest(req *pb.QueryRootInvocationNamesRequest) error {
	if req.GetName() == "" {
		return errors.New("name missing")
	}

	if err := pbutil.ValidateInvocationName(req.Name); err != nil {
		return errors.Fmt("name: %w", err)
	}

	return nil
}

// QueryRootInvocationNames implements pb.ResultDBServer.
func (s *resultDBServer) QueryRootInvocationNames(ctx context.Context, in *pb.QueryRootInvocationNamesRequest) (*pb.QueryRootInvocationNamesResponse, error) {
	// Use one transaction for the entire RPC so that we work with a
	// consistent snapshot of the system state. This is important to
	// prevent subtle bugs and TOC-TOU vulnerabilities.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// Root invocation name can be seen as a property of its descendants.
	// Therefore, the caller only need to have permission for the invocation in the request
	// to obtain root invocation names.
	if err := permissions.VerifyInvocationByName(ctx, in.Name, rdbperms.PermGetInvocation); err != nil {
		return nil, err
	}

	if err := validateQueryRootInvocationNamesRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	roots, err := graph.FindRoots(ctx, invocations.MustParseName(in.Name))
	if err != nil {
		return nil, errors.Fmt("find root: %w", err)
	}
	names := make([]string, 0, len(roots))
	for root := range roots {
		names = append(names, root.Name())
	}
	return &pb.QueryRootInvocationNamesResponse{
		RootInvocationNames: names,
	}, nil
}
