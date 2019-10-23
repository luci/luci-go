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
	"crypto/sha512"
	"encoding/hex"
	"io"

	"cloud.google.com/go/spanner"
	"github.com/google/uuid"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// validateCreateTestExonerationRequest returns a non-nil error if req is invalid.
func validateCreateTestExonerationRequest(req *pb.CreateTestExonerationRequest) error {
	if err := pbutil.ValidateInvocationName(req.Invocation); err != nil {
		return errors.Annotate(err, "invocation").Err()
	}
	if err := pbutil.ValidateTestVariant(req.TestExoneration.GetTestVariant()); err != nil {
		return errors.Annotate(err, "test_exoneration: test_variant").Err()
	}
	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Annotate(err, "request_id").Err()
	}
	return nil
}

// CreateTestExoneration implements pb.RecorderServer.
func (s *recorderServer) CreateTestExoneration(ctx context.Context, in *pb.CreateTestExonerationRequest) (*pb.TestExoneration, error) {
	if err := validateCreateTestExonerationRequest(in); err != nil {
		return nil, errors.Annotate(err, "bad request").Tag(grpcutil.InvalidArgumentTag).Err()
	}
	invID := pbutil.MustParseInvocationName(in.Invocation)

	// Compute exoneration ID and choose Insert vs InsertOrUpdate.
	var exonerationID string
	mutFn := spanner.InsertMap
	if in.RequestId == "" {
		// Use a random id.
		exonerationID = "r:" + uuid.New().String()
	} else {
		// Use a deterministic id.
		exonerationID = "d:" + deterministicExonerationID(ctx, in.RequestId)
		mutFn = spanner.InsertOrUpdateMap
	}

	ret := &pb.TestExoneration{
		Name:                pbutil.TestExonerationName(invID, exonerationID),
		TestVariant:         in.TestExoneration.TestVariant,
		ExplanationMarkdown: in.TestExoneration.ExplanationMarkdown,
	}

	return ret, mutateInvocation(ctx, invID, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		return txn.BufferWrite([]*spanner.Mutation{
			mutFn("TestExonerations", span.ToSpannerMap(map[string]interface{}{
				"InvocationId":        invID,
				"ExonerationId":       exonerationID,
				"TestPath":            in.TestExoneration.TestVariant.TestPath,
				"VariantDef":          in.TestExoneration.TestVariant.Variant,
				"ExplanationMarkdown": in.TestExoneration.ExplanationMarkdown,
			})),
		})
	})
}

func deterministicExonerationID(ctx context.Context, requestID string) string {
	h := sha512.New()
	// Include current identity, so that two separate clients
	// do not override each other's test exonerations even if
	// they happened to produce identical request ids.
	// The alternative is to use remote IP address, but it is not
	// implemented in pRPC.
	io.WriteString(h, string(auth.CurrentIdentity(ctx)))
	io.WriteString(h, "\n")
	io.WriteString(h, requestID)
	return hex.EncodeToString(h.Sum(nil))
}
