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

package recorder

import (
	"context"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// validateUpdateInvocationRequest returns non-nil error if req is invalid.
func validateUpdateInvocationRequest(req *pb.UpdateInvocationRequest, now time.Time) error {
	if err := pbutil.ValidateInvocationName(req.Invocation.GetName()); err != nil {
		return errors.Annotate(err, "invocation: name").Err()
	}

	if len(req.UpdateMask.GetPaths()) == 0 {
		return errors.Reason("update_mask: paths is empty").Err()
	}

	for _, path := range req.UpdateMask.GetPaths() {
		switch path {
		// The cases in this switch statement must be synchronized with a
		// similar switch statement in UpdateInvocation implementation.

		case "deadline":
			if err := validateInvocationDeadline(req.Invocation.GetDeadline(), now); err != nil {
				return errors.Annotate(err, "invocation: deadline").Err()
			}

		case "bigquery_exports":
			for i, bqExport := range req.Invocation.GetBigqueryExports() {
				if err := pbutil.ValidateBigQueryExport(bqExport); err != nil {
					return errors.Annotate(err, "invocation: bigquery_exports[%d]", i).Err()
				}
			}

		case "properties":
			if err := pbutil.ValidateProperties(req.Invocation.Properties); err != nil {
				return errors.Annotate(err, "invocation: properties").Err()
			}

		default:
			return errors.Reason("update_mask: unsupported path %q", path).Err()
		}
	}

	return nil
}

// UpdateInvocation implements pb.RecorderServer.
func (s *recorderServer) UpdateInvocation(ctx context.Context, in *pb.UpdateInvocationRequest) (*pb.Invocation, error) {
	if err := validateUpdateInvocationRequest(in, clock.Now(ctx).UTC()); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	invID := invocations.MustParseName(in.Invocation.Name)

	var ret *pb.Invocation
	err := mutateInvocation(ctx, invID, func(ctx context.Context) error {
		var err error
		if ret, err = invocations.Read(ctx, invID); err != nil {
			return err
		}

		values := map[string]interface{}{
			"InvocationId": invID,
		}

		for _, path := range in.UpdateMask.Paths {
			switch path {
			// The cases in this switch statement must be synchronized with a
			// similar switch statement in validateUpdateInvocationRequest.

			case "deadline":
				deadlne := in.Invocation.Deadline
				values["Deadline"] = deadlne
				ret.Deadline = deadlne

			case "bigquery_exports":
				bqExports := in.Invocation.BigqueryExports
				values["BigQueryExports"] = bqExports
				ret.BigqueryExports = bqExports

			case "properties":
				values["Properties"] = spanutil.Compressed(pbutil.MustMarshal(in.Invocation.Properties))
				ret.Properties = in.Invocation.Properties

			default:
				panic("impossible")
			}
		}

		span.BufferWrite(ctx, spanutil.UpdateMap("Invocations", values))
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}
