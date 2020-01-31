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
	"crypto/rand"
	"encoding/hex"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/ptypes"
	tspb "github.com/golang/protobuf/ptypes/timestamp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/prpc"

	"go.chromium.org/luci/resultdb/internal"
	"go.chromium.org/luci/resultdb/internal/appstatus"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// validateInvocationDeadline returns a non-nil error if deadline is invalid.
func validateInvocationDeadline(deadline *tspb.Timestamp, now time.Time) error {
	internal.AssertUTC(now)
	switch deadline, err := ptypes.Timestamp(deadline); {
	case err != nil:
		return err

	case deadline.Sub(now) < 10*time.Second:
		return errors.Reason("must be at least 10 seconds in the future").Err()

	case deadline.Sub(now) > 2*24*time.Hour:
		return errors.Reason("must be before 48h in the future").Err()

	default:
		return nil
	}
}

// validateCreateInvocationRequest returns an error if req is determined to be
// invalid.
func validateCreateInvocationRequest(req *pb.CreateInvocationRequest, now time.Time) error {
	if err := pbutil.ValidateInvocationID(req.InvocationId); err != nil {
		return errors.Annotate(err, "invocation_id").Err()
	}

	// TODO(nodir): whitelist trusted LUCI service accounts that are allowed to
	// create invocations with any ids.
	if !strings.HasPrefix(req.InvocationId, "u:") {
		return errors.Reason(`invocation_id: an invocation created by a non-LUCI system must have id starting with "u:"; please generate "u:{GUID}"`).Err()
	}

	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Annotate(err, "request_id").Err()
	}

	inv := req.Invocation
	if inv == nil {
		return nil
	}

	if err := pbutil.ValidateStringPairs(inv.GetTags()); err != nil {
		return errors.Annotate(err, "invocation.tags").Err()
	}

	if inv.GetDeadline() != nil {
		if err := validateInvocationDeadline(inv.Deadline, now); err != nil {
			return errors.Annotate(err, "invocation: deadline").Err()
		}
	}

	for i, bqExport := range inv.GetBigqueryExports() {
		if err := pbutil.ValidateBigQueryExport(bqExport); err != nil {
			return errors.Annotate(err, "bigquery_export[%d]", i).Err()
		}
	}

	return nil
}

// CreateInvocation implements pb.RecorderServer.
func (s *recorderServer) CreateInvocation(ctx context.Context, in *pb.CreateInvocationRequest) (*pb.Invocation, error) {
	now := clock.Now(ctx).UTC()

	if err := validateCreateInvocationRequest(in, now); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	invID := span.InvocationID(in.InvocationId)

	// Return update token to the client.
	updateToken, err := generateUpdateToken()
	if err != nil {
		return nil, err
	}
	prpc.SetHeader(ctx, metadata.Pairs(UpdateTokenMetadataKey, updateToken))

	// Prepare the invocation we will return.
	inv := &pb.Invocation{
		Name:            invID.Name(),
		State:           pb.Invocation_ACTIVE,
		Deadline:        in.Invocation.GetDeadline(),
		Tags:            in.Invocation.GetTags(),
		BigqueryExports: in.Invocation.GetBigqueryExports(),
	}

	// Ensure the invocation has a deadline.
	if inv.Deadline == nil {
		inv.Deadline = pbutil.MustTimestampProto(now.Add(defaultInvocationDeadlineDuration))
	}

	pbutil.NormalizeInvocation(inv)

	commitTimestamp, err := span.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		// Dedupe the request if possible.
		if in.RequestId != "" {
			var curRequestID spanner.NullString
			err := span.ReadInvocation(ctx, txn, invID, map[string]interface{}{
				"CreateRequestId": &curRequestID,
			})
			st, _ := appstatus.Get(err)
			switch {
			case st.Code() == codes.NotFound:
				// Continue to creation.

			case err != nil:
				return err

			case curRequestID.Valid && curRequestID.StringVal == in.RequestId:
				// Dedupe the request.
				inv, err = span.ReadInvocationFull(ctx, txn, invID)
				return err

			default:
				return invocationAlreadyExists(invID)
			}
		}

		return txn.BufferWrite([]*spanner.Mutation{
			span.InsertMap("Invocations", s.rowOfInvocation(ctx, inv, updateToken, in.RequestId)),
		})
	})
	switch {
	case spanner.ErrCode(err) == codes.AlreadyExists:
		return nil, invocationAlreadyExists(invID)

	case err != nil:
		return nil, err
	}

	if inv.CreateTime == nil {
		// The request was not deduped.
		inv.CreateTime = pbutil.MustTimestampProto(commitTimestamp)
	}
	return inv, nil
}

func invocationAlreadyExists(id span.InvocationID) error {
	return appstatus.Errorf(codes.AlreadyExists, "%s already exsts", id.Name())
}

func generateUpdateToken() (string, error) {
	buf := make([]byte, 32)
	_, err := rand.Read(buf)
	return hex.EncodeToString(buf), err
}
