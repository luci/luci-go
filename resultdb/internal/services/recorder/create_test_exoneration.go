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

	"cloud.google.com/go/spanner"
	"github.com/google/uuid"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// validateCreateTestExonerationRequest returns a non-nil error if req is invalid.
func validateCreateTestExonerationRequest(req *pb.CreateTestExonerationRequest, requireInvocation bool) error {
	if requireInvocation || req.Invocation != "" {
		if err := pbutil.ValidateInvocationName(req.Invocation); err != nil {
			return errors.Annotate(err, "invocation").Err()
		}
	}

	ex := req.GetTestExoneration()
	if err := pbutil.ValidateTestID(ex.GetTestId()); err != nil {
		return errors.Annotate(err, "test_exoneration: test_id").Err()
	}
	if err := pbutil.ValidateVariant(ex.GetVariant()); err != nil {
		return errors.Annotate(err, "test_exoneration: variant").Err()
	}

	hasVariant := len(ex.GetVariant().GetDef()) != 0
	hasVariantHash := ex.VariantHash != ""
	if hasVariant && hasVariantHash {
		computedHash := pbutil.VariantHash(ex.GetVariant())
		if computedHash != ex.VariantHash {
			return errors.Reason("computed and supplied variant hash don't match").Err()
		}
	}

	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Annotate(err, "request_id").Err()
	}

	if ex.ExplanationHtml == "" {
		return errors.Reason("test_exoneration: explanation_html: unspecified").Err()
	}
	if ex.Reason == pb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED {
		return errors.Reason("test_exoneration: reason: unspecified").Err()
	}
	return nil
}

// CreateTestExoneration implements pb.RecorderServer.
func (s *recorderServer) CreateTestExoneration(ctx context.Context, in *pb.CreateTestExonerationRequest) (*pb.TestExoneration, error) {
	if err := validateCreateTestExonerationRequest(in, true); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	invID := invocations.MustParseName(in.Invocation)

	ret, mutation := insertTestExoneration(ctx, invID, in.RequestId, 0, in.TestExoneration)
	err := mutateInvocation(ctx, invID, func(ctx context.Context) error {
		span.BufferWrite(ctx, mutation)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func insertTestExoneration(ctx context.Context, invID invocations.ID, requestID string, ordinal int, body *pb.TestExoneration) (ret *pb.TestExoneration, mutation *spanner.Mutation) {
	// Compute exoneration ID and choose Insert vs InsertOrUpdate.
	var exonerationIDSuffix string
	mutFn := spanner.InsertMap
	if requestID == "" {
		// Use a random id.
		exonerationIDSuffix = "r:" + uuid.New().String()
	} else {
		// Use a deterministic id.
		exonerationIDSuffix = "d:" + deterministicExonerationIDSuffix(ctx, requestID, ordinal)
		mutFn = spanner.InsertOrUpdateMap
	}

	// Use the given variant hash, or the hash of the given variant, whichever
	// is present. If both are present then validation guarantees they'll
	// match, so we can just use whichever.
	variantHash := body.VariantHash
	if variantHash == "" {
		variantHash = pbutil.VariantHash(body.Variant)
	}

	exonerationID := fmt.Sprintf("%s:%s", variantHash, exonerationIDSuffix)
	ret = &pb.TestExoneration{
		Name:            pbutil.TestExonerationName(string(invID), body.TestId, exonerationID),
		TestId:          body.TestId,
		Variant:         body.Variant,
		VariantHash:     variantHash,
		ExonerationId:   exonerationID,
		ExplanationHtml: body.ExplanationHtml,
		Reason:          body.Reason,
	}

	mutation = mutFn("TestExonerations", spanutil.ToSpannerMap(map[string]any{
		"InvocationId":    invID,
		"TestId":          ret.TestId,
		"ExonerationId":   exonerationID,
		"Variant":         ret.Variant,
		"VariantHash":     ret.VariantHash,
		"ExplanationHTML": spanutil.Compressed(ret.ExplanationHtml),
		"Reason":          ret.Reason,
	}))
	return
}

func deterministicExonerationIDSuffix(ctx context.Context, requestID string, ordinal int) string {
	h := sha512.New()
	// Include current identity, so that two separate clients
	// do not override each other's test exonerations even if
	// they happened to produce identical request ids.
	// The alternative is to use remote IP address, but it is not
	// implemented in pRPC.
	fmt.Fprintln(h, auth.CurrentIdentity(ctx))
	fmt.Fprintln(h, requestID)
	fmt.Fprintln(h, ordinal)
	return hex.EncodeToString(h.Sum(nil))
}
