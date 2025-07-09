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

package resultdb

import (
	"context"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/resultdb/rdbperms"
)

func (s *resultDBServer) GetRootInvocation(ctx context.Context, in *pb.GetRootInvocationRequest) (*pb.RootInvocation, error) {
	// Use one transaction for the entire RPC so that we work with a
	// consistent snapshot of the system state. This is important to
	// prevent subtle bugs and TOC-TOU vulnerabilities.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	if err := verifyGetRootInvocationPermissions(ctx, in); err != nil {
		return nil, err
	}

	row, err := rootinvocations.Read(ctx, rootinvocations.MustParseName(in.Name))
	if err != nil {
		// Error is already annotated with NotFound appstatus if record is not found.
		// Otherwise it is an internal error.
		return nil, err
	}

	return toRootInvocationProto(row), nil
}

// verifyGetRootInvocationPermissions verifies the user has access to the
// root invocation specified in the request.
func verifyGetRootInvocationPermissions(ctx context.Context, req *pb.GetRootInvocationRequest) error {
	id, err := pbutil.ParseRootInvocationName(req.Name)
	if err != nil {
		return appstatus.BadRequest(errors.Fmt("name: %w", err))
	}
	// Lookup the root invocation and check permissions on its realm.
	return permissions.VerifyRootInvocation(ctx, rootinvocations.ID(id), rdbperms.PermGetRootInvocation)
}

func toRootInvocationProto(in *rootinvocations.RootInvocationRow) *pb.RootInvocation {
	result := &pb.RootInvocation{
		Name:             in.RootInvocationID.Name(),
		RootInvocationId: string(in.RootInvocationID),
		State:            in.State,
		Realm:            in.Realm,
		CreateTime:       pbutil.MustTimestampProto(in.CreateTime),
		Creator:          in.CreatedBy,
		Deadline:         pbutil.MustTimestampProto(in.Deadline),
		ProducerResource: in.ProducerResource,
		Sources:          in.Sources,
		SourcesFinal:     in.IsSourcesFinal,
		Tags:             in.Tags,
		Properties:       in.Properties,
		BaselineId:       in.BaselineID,
	}
	if in.FinalizeStartTime.Valid {
		result.FinalizeStartTime = pbutil.MustTimestampProto(in.FinalizeStartTime.Time)
	}
	if in.FinalizeTime.Valid {
		result.FinalizeTime = pbutil.MustTimestampProto(in.FinalizeTime.Time)
	}
	return result
}
