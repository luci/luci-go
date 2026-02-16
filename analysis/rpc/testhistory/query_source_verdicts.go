// Copyright 2026 The LUCI Authors.
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

package testhistory

import (
	"context"
	"encoding/hex"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	"go.chromium.org/luci/resultdb/rdbperms"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/internal/testresults"
	"go.chromium.org/luci/analysis/internal/testresults/lowlatency"
	"go.chromium.org/luci/analysis/pbutil"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// maxSourceVerdictsPageSize is the maximum page size for QuerySourceVerdicts.
const maxSourceVerdictsPageSize = 1_000

// QuerySourceVerdicts queries source verdicts for a given test and source ref.
func (s *testHistoryServer) QuerySourceVerdicts(ctx context.Context, req *pb.QuerySourceVerdictsV2Request) (*pb.QuerySourceVerdictsV2Response, error) {
	if err := validateQuerySourceVerdictsRequest(req); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	q := lowlatency.SourceVerdictQuery{
		Project:  req.Project,
		Filter:   req.Filter,
		PageSize: adjustPageSize(req.PageSize),
	}

	var err error
	q.OrderBy, err = lowlatency.ParseOrderBy(req.OrderBy)
	if err != nil {
		// Should have been picked up in request validation already.
		return nil, errors.Fmt("order_by: %w", err)
	}

	if req.GetTestIdStructured() != nil {
		q.TestID, q.VariantHash = pbutil.FlatTestIDFromStructured(req.GetTestIdStructured())
	} else if req.GetTestIdFlat() != nil {
		q.TestID = req.GetTestIdFlat().TestId
		q.VariantHash = req.GetTestIdFlat().VariantHash
	} else {
		// Should have been picked up in request validation already.
		return nil, errors.New("either test_id_structured or test_id_flat must be specified")
	}

	if req.SourceRef != nil {
		q.RefHash = pbutil.SourceRefHash(req.SourceRef)
	} else {
		var err error
		q.RefHash, err = hex.DecodeString(req.SourceRefHash)
		if err != nil {
			// Should have been picked up in request validation already.
			return nil, errors.Fmt("decode source ref hash: %w", err)
		}
	}

	pageToken, err := lowlatency.ParsePageToken(req.PageToken)
	if err != nil {
		// Should have been picked up in request validation already.
		return nil, errors.Fmt("page token: %w", err)
	}

	// Get all realms the user has access to in the project.
	allowedSubRealms, err := perms.QuerySubRealmsNonEmpty(ctx, req.Project, "", nil, rdbperms.PermListTestResults)
	if err != nil {
		// Internal error using authdb, or PermissionDenied appstatus error because
		// user has access to no realms.
		return nil, err
	}
	q.AllowedSubrealms = allowedSubRealms

	// Execute query.
	var results []lowlatency.SourceVerdictV2
	var nextPageToken lowlatency.PageToken
	results, nextPageToken, err = q.Fetch(span.Single(ctx), pageToken)
	if err != nil {
		return nil, errors.Fmt("query source verdicts: %w", err)
	}

	resp := &pb.QuerySourceVerdictsV2Response{}
	for _, r := range results {
		sv := toSourceVerdictProto(r)
		resp.SourceVerdicts = append(resp.SourceVerdicts, sv)
	}
	if nextPageToken != (lowlatency.PageToken{}) {
		resp.NextPageToken = nextPageToken.Serialize()
	}
	return resp, nil
}

func validateQuerySourceVerdictsRequest(req *pb.QuerySourceVerdictsV2Request) error {
	if err := pbutil.ValidateProject(req.Project); err != nil {
		return errors.Fmt("project: %w", err)
	}

	if id := req.GetTestIdStructured(); id != nil {
		if err := pbutil.ValidateStructuredTestIdentifierForQuery(id); err != nil {
			return errors.Fmt("test_id_structured: %w", err)
		}
		// Structured test ID validation if we supported it.
	} else if id := req.GetTestIdFlat(); id != nil {
		if err := pbutil.ValidateFlatTestIdentifierForQuery(id); err != nil {
			return errors.Fmt("test_id_flat: %w", err)
		}
	} else {
		return errors.New("either test_id_structured or test_id_flat must be specified")
	}

	if req.SourceRef != nil {
		if err := pbutil.ValidateSourceRef(req.SourceRef); err != nil {
			return errors.Fmt("source_ref: %w", err)
		}
		if req.SourceRefHash != "" {
			return errors.New("only one of source_ref and source_ref_hash can be specified")
		}
	} else if req.SourceRefHash != "" {
		if err := pbutil.ValidateSourceRefHash(req.SourceRefHash); err != nil {
			return errors.Fmt("source_ref_hash: %w", err)
		}
	} else {
		return errors.New("either source_ref or source_ref_hash must be specified")
	}

	if req.PageSize < 0 {
		return errors.New("page_size: must be non-negative")
	}

	if _, err := lowlatency.ParsePageToken(req.PageToken); err != nil {
		return errors.New("page_token: invalid page token")
	}

	if err := lowlatency.ValidateSourceVerdictsFilter(req.Filter); err != nil {
		return errors.Fmt("filter: %w", err)
	}

	if _, err := lowlatency.ParseOrderBy(req.OrderBy); err != nil {
		return errors.Fmt("order_by: %w", err)
	}

	return nil
}

func adjustPageSize(pageSize int32) int {
	if pageSize == 0 {
		// Default to max size.
		return maxSourceVerdictsPageSize
	} else if pageSize > maxSourceVerdictsPageSize {
		return maxSourceVerdictsPageSize
	}
	return int(pageSize)
}

func toSourceVerdictProto(r lowlatency.SourceVerdictV2) *pb.SourceVerdict {
	sv := &pb.SourceVerdict{
		Position:          r.Position,
		Status:            r.Status,
		ApproximateStatus: r.ApproximateStatus,
	}
	for _, iv := range r.InvocationVerdicts {
		item := &pb.SourceVerdict_InvocationTestVerdict{
			PartitionTime:  timestamppb.New(iv.PartitionTime),
			Status:         iv.Status,
			Changelists:    toChangelistProto(iv.Changelists),
			IsSourcesDirty: iv.HasDirtySources,
		}
		if iv.RootInvocationID != "" {
			item.RootInvocation = rdbpbutil.RootInvocationName(iv.RootInvocationID)
		}
		if iv.InvocationID != "" {
			item.Invocation = rdbpbutil.InvocationName(iv.InvocationID)
		}
		sv.InvocationVerdicts = append(sv.InvocationVerdicts, item)
	}
	return sv
}

func toChangelistProto(cls []testresults.Changelist) []*pb.Changelist {
	if len(cls) == 0 {
		return nil
	}
	res := make([]*pb.Changelist, len(cls))
	for i, cl := range cls {
		res[i] = &pb.Changelist{
			Host:      cl.Host,
			Change:    cl.Change,
			Patchset:  int32(cl.Patchset),
			OwnerKind: cl.OwnerKind,
		}
	}
	return res
}
