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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/workunits"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// GetRootInvocation implements pb.RecorderServer.
func (s *recorderServer) GetRootInvocation(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
	if err := verifyGetRootInvocationPermissions(ctx, in); err != nil {
		return nil, err
	}

	// Use one transaction for the entire RPC so that we work with a
	// consistent snapshot of the system state. This is important to
	// prevent subtle bugs and TOC-TOU vulnerabilities.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	row, err := rootinvocations.Read(ctx, rootinvocations.MustParseName(in.Name))
	if err != nil {
		// Error is already annotated with NotFound appstatus if record is not found.
		// Otherwise it is an internal error.
		return nil, err
	}

	return row.ToProto(), nil
}

func verifyGetRootInvocationPermissions(ctx context.Context, req *pb.GetRootInvocationRequest) error {
	name := req.GetName()
	rootInvocationID, err := rootinvocations.ParseName(name)
	if err != nil {
		return appstatus.BadRequest(errors.Fmt("name: %w", err))
	}

	token, err := extractUpdateToken(ctx)
	if err != nil {
		return err
	}

	// Root invocation expects the update token of the root work unit.
	rootWorkUnitID := workunits.ID{
		RootInvocationID: rootInvocationID,
		WorkUnitID:       workunits.RootWorkUnitID,
	}
	if err := validateWorkUnitUpdateToken(ctx, token, rootWorkUnitID); err != nil {
		return err // PermissionDenied appstatus error.
	}
	return nil
}
