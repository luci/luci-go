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
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/golang/protobuf/ptypes"
	"github.com/google/uuid"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/resultdb/internal/appstatus"
	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

func validateCreateTestResultRequest(msg *pb.CreateTestResultRequest, now time.Time) error {
	if err := pbutil.ValidateInvocationName(msg.Invocation); err != nil {
		return errors.Annotate(err, "invocation").Err()
	}
	if err := pbutil.ValidateTestResult(now, msg.TestResult); err != nil {
		return errors.Annotate(err, "test_result").Err()
	}
	if err := pbutil.ValidateRequestID(msg.RequestId); err != nil {
		return errors.Annotate(err, "request_id").Err()
	}
	return nil
}

// CreateTestResult implements pb.RecorderServer.
func (s *recorderServer) CreateTestResult(ctx context.Context, in *pb.CreateTestResultRequest) (*pb.TestResult, error) {
	now := clock.Now(ctx).UTC()
	if err := validateCreateTestResultRequest(in, now); err != nil {
		return nil, appstatus.BadRequest(err)
	}

	invID := span.MustParseInvocationName(in.Invocation)
	ret, mutation := insertTestResult(ctx, invID, in.RequestId, 0, in.TestResult)
	err := mutateInvocation(ctx, invID, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		return txn.BufferWrite([]*spanner.Mutation{mutation})
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func insertTestResult(ctx context.Context, invID span.InvocationID, requestID string, ordinal int, body *pb.TestResult) (*pb.TestResult, *spanner.Mutation) {
	// create a shallow copy of the input message with the OUTPUT_ONLY fields to be used in
	// the response
	ret := *body
	vHash := pbutil.VariantHash(ret.Variant)
	ret.ResultId = genTestResultID(ctx, requestID, vHash, ordinal)
	ret.Name = pbutil.TestResultName(string(invID), ret.TestId, ret.ResultId)

	// handle values for nullable columns
	isUnexpected := spanner.NullBool{Bool: true, Valid: !body.Expected}
	runDuration := spanner.NullInt64{Int64: 0, Valid: false}
	if ret.Duration != nil {
		d, _ := ptypes.Duration(ret.Duration)
		runDuration.Int64 = d.Microseconds()
		runDuration.Valid = true
	}

	mutFn := spanner.InsertMap
	if requestID == "" {
		mutFn = spanner.InsertOrUpdateMap
	}
	mutation := mutFn(
		"TestResults",
		span.ToSpannerMap(map[string]interface{}{
			"InvocationId":    invID,
			"TestId":          ret.TestId,
			"ResultId":        ret.ResultId,
			"Variant":         ret.Variant,
			"VariantHash":     vHash,
			"CommitTimestamp": spanner.CommitTimestamp,
			"IsUnexpected":    isUnexpected,
			"Status":          ret.Status,
			"SummaryHTML":     span.Compressed(ret.SummaryHtml),
			"StartTime":       ret.StartTime,
			"RunDurationUsec": runDuration,
			"Tags":            ret.Tags,
			"InputArtifacts":  ret.InputArtifacts,
			"OutputArtifacts": ret.OutputArtifacts,
		}),
	)
	return &ret, mutation
}

func genTestResultID(ctx context.Context, reqID string, vHash string, ordinal int) string {
	if reqID == "" {
		return fmt.Sprintf("%s:r:%s", vHash, uuid.New().String())
	}

	h := sha512.New()
	// include the current identity to distinguish requests with the same request ID, but
	// from different clients.
	fmt.Fprintln(h, auth.CurrentIdentity(ctx))
	fmt.Fprintln(h, reqID)
	fmt.Fprintln(h, ordinal)
	suffix := hex.EncodeToString(h.Sum(nil))
	return fmt.Sprintf("%s:d:%s", vHash, suffix)
}
