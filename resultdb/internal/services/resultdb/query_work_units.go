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
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/masking"
	"go.chromium.org/luci/resultdb/internal/pagination"
	"go.chromium.org/luci/resultdb/internal/permissions"
	"go.chromium.org/luci/resultdb/internal/rootinvocations"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// ResponseLimitBytes is the soft limit on the response size.
// It is a rough estimate of the pRPC response size, assuming the response is protoJSON encoded.
// ProtoJSON encoding will always use more bytes than protocol buffer wire format,
// so this is also a limit on the protocol buffer wire encoded form of the work units.
// A page will be truncated if the response exceeds this value, and a page_token
// will be returned to query subsequent work units.
const ResponseLimitBytes = 20 * 1000 * 1000 // 20 MB

func validateQueryWorkUnitsRequest(req *pb.QueryWorkUnitsRequest) error {
	if err := pbutil.ValidateRootInvocationName(req.Parent); err != nil {
		return errors.Fmt("parent: %w", err)
	}
	if err := pbutil.ValidateWorkUnitView(req.View); err != nil {
		return errors.Fmt("view: %w", err)
	}

	if req.Predicate != nil {
		if err := pbutil.ValidateWorkUnitPredicate(req.Predicate); err != nil {
			return errors.Fmt("predicate: %w", err)
		}
		// Validate that ancestors_of refers to the same root invocation as the request.
		// Since ValidateWorkUnitPredicate passed, req.Predicate is non-nil and AncestorsOf is non-empty.
		if req.Predicate.AncestorsOf != "" {
			rootInvID := rootinvocations.MustParseName(req.Parent)
			ancestorID, err := workunits.ParseName(req.Predicate.AncestorsOf)
			if err != nil {
				return errors.Fmt("predicate: ancestors_of: %w", err)
			}
			if ancestorID.RootInvocationID != rootInvID {
				return errors.Fmt("predicate: ancestors_of: work unit %q does not belong to the parent root invocation %q", req.Predicate.AncestorsOf, rootInvID.Name())
			}
		}
	}

	if err := pagination.ValidatePageSize(req.PageSize); err != nil {
		return errors.Fmt("page_size: %w", err)
	}
	return nil
}

func (s *resultDBServer) QueryWorkUnits(ctx context.Context, in *pb.QueryWorkUnitsRequest) (*pb.QueryWorkUnitsResponse, error) {
	// Use one transaction for the entire RPC so that we work with a
	// consistent snapshot of the system state. This is important to
	// prevent subtle bugs and TOC-TOU vulnerabilities.
	ctx, cancel := span.ReadOnlyTransaction(ctx)
	defer cancel()

	// While google.aip.dev/211 prescribes request validation should occur after
	// authorisation, the root invocation ID is necessary to perform this authorisation.
	rootInvID, err := rootinvocations.ParseName(in.Parent)
	if err != nil {
		return nil, appstatus.BadRequest(errors.Fmt("parent: %w", err))
	}

	// Create the accessChecker and verifies the caller has the basic permission to list work units
	// in the specified root invocation.
	accessChecker, err := permissions.NewWorkUnitAccessChecker(ctx, rootInvID, permissions.ListWorkUnitsAccessModel)
	if err != nil {
		return nil, err
	}
	if accessChecker.RootInvovcationAccess == permissions.NoAccess {
		return nil, permissions.NoRootInvocationAccessError(rootInvID, permissions.ListWorkUnitsAccessModel)
	}

	if err := validateQueryWorkUnitsRequest(in); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}

	mask := workunits.ExcludeExtendedProperties
	if in.View == pb.WorkUnitView_WORK_UNIT_VIEW_FULL {
		mask = workunits.AllFields
	}
	q := workunits.Query{
		RootInvocationID: rootInvID,
		Predicate:        in.Predicate,
		Mask:             mask,
		PageSize:         pagination.AdjustPageSize(in.PageSize),
		PageToken:        in.PageToken,
	}
	wus := make([]*pb.WorkUnit, 0, q.PageSize)
	responseSize := 0

	nextPageToken, err := q.Query(ctx, func(wur *workunits.WorkUnitRow) error {
		accessLevel, err := accessChecker.Check(ctx, wur.Realm)
		if err != nil {
			return err
		}

		wuProto := masking.WorkUnit(wur, accessLevel, in.View, cfg)
		wus = append(wus, wuProto)

		// Apply soft response size limit.
		responseSize += workUnitRowSize(wuProto)
		if responseSize > ResponseLimitBytes {
			return workunits.ResponseLimitReachedErr
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if len(wus) == 0 {
		return &pb.QueryWorkUnitsResponse{}, nil
	}

	return &pb.QueryWorkUnitsResponse{
		WorkUnits:     wus,
		NextPageToken: nextPageToken,
	}, nil
}

// workUnitRowSize return the size of a work unit in
// a pRPC response (pRPC responses use JSON serialisation).
func workUnitRowSize(wu *pb.WorkUnit) int {
	// Estimate the size of a JSON-serialised work unit,
	// as the sum of the sizes of its fields, plus
	// an overhead (for JSON grammar and the field names).
	return 1000 + proto.Size(wu)
}
