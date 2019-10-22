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

	"go.chromium.org/luci/resultdb/internal/span"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/auth"
)

func validateCreateTestExonerationRequest(req *pb.CreateTestExonerationRequest) error {
	if err := pbutil.ValidateInvocationName(req.Invocation); err != nil {
		return errors.Annotate(err, "invocation").Err()
	}
	if err := pbutil.ValidateTestVariant(req.TestExoneration.GetTestVariant()); err != nil {
		return errors.Annotate(err, "test_exoneration: test_variant").Err()
	}
	return nil
}

// CreateTestExoneration implements pb.RecorderServer.
func (s *recorderServer) CreateTestExoneration(ctx context.Context, in *pb.CreateTestExonerationRequest) (*pb.TestExoneration, error) {
	if err := validateCreateTestExonerationRequest(in); err != nil {
		return nil, errors.Annotate(err, "bad request").Tag(grpcutil.InvalidArgumentTag).Err()
	}

	invID := pbutil.MustParseInvocationName(in.Invocation)
	ret := &pb.TestExoneration{
		TestVariant:         in.TestExoneration.TestVariant,
		ExplanationMarkdown: in.TestExoneration.ExplanationMarkdown,
	}

	// Prepare common column values for the new row.
	const exonerationIDColumn = "ExonerationId"
	columnValues := map[string]interface{}{
		"InvocationId": invID,
		// ExonerationID is set below.
		"TestPath":            in.TestExoneration.TestVariant.TestPath,
		"VariantDef":          in.TestExoneration.TestVariant.Variant,
		"ExplanationMarkdown": in.TestExoneration.ExplanationMarkdown,
		// Do not set CreateRequestId unless it is not empty.
	}

	// Compute exoneration ID and choose Insert vs InsertOrUpdate.
	var exonerationID string
	mutFn := spanner.InsertMap
	if in.RequestId != "" {
		exonerationID = "r:" + exonerationIDFromRequestID(ctx, in.RequestId)
		mutFn = spanner.InsertOrUpdateMap
	} else {
		exonerationID = "u:" + uuid.New().String()
	}
	ret.Name = pbutil.TestExonerationName(invID, exonerationID)

	columnValues["ExonerationId"] = exonerationID
	return ret, mutateInvocation(ctx, invID, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		return txn.BufferWrite([]*spanner.Mutation{
			mutFn("TestExonerations", span.ToSpannerMap(columnValues)),
		})
	})
}

func exonerationIDFromRequestID(ctx context.Context, requestID string) string {
	h := sha512.New()
	io.WriteString(h, string(auth.CurrentIdentity(ctx)))
	io.WriteString(h, "\n")
	io.WriteString(h, requestID)
	return hex.EncodeToString(h.Sum(nil))
}
