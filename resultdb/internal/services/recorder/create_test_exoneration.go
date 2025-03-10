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
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// validateCreateTestExonerationRequest returns a non-nil error if req is invalid.
func validateCreateTestExonerationRequest(req *pb.CreateTestExonerationRequest, cfg *config.CompiledServiceConfig, requireInvocation bool) error {
	if requireInvocation || req.Invocation != "" {
		if err := pbutil.ValidateInvocationName(req.Invocation); err != nil {
			return errors.Annotate(err, "invocation").Err()
		}
	}
	if err := validateTestExoneration(req.TestExoneration, cfg); err != nil {
		return errors.Annotate(err, "test_exoneration").Err()
	}
	if err := pbutil.ValidateRequestID(req.RequestId); err != nil {
		return errors.Annotate(err, "request_id").Err()
	}
	return nil
}

func validateTestExoneration(ex *pb.TestExoneration, cfg *config.CompiledServiceConfig) error {
	if ex == nil {
		return errors.Reason("unspecified").Err()
	}
	if ex.TestIdStructured == nil && ex.TestId != "" {
		// For backwards compatibility, we still accept legacy uploaders setting
		// the test_id and variant or variant_hash fields (even though they are
		// officially OUTPUT_ONLY now).
		testID, err := pbutil.ParseAndValidateTestID(ex.TestId)
		if err != nil {
			return errors.Annotate(err, "test_id").Err()
		}
		if err := pbutil.ValidateVariant(ex.GetVariant()); err != nil {
			return errors.Annotate(err, "variant").Err()
		}

		// Some legacy clients do not set variant and only set variant_hash.
		// Some set both. If both are set, check they are consistent.
		hasVariant := len(ex.Variant.GetDef()) != 0
		hasVariantHash := ex.VariantHash != ""
		if hasVariant && hasVariantHash {
			computedHash := pbutil.VariantHash(ex.GetVariant())
			if computedHash != ex.VariantHash {
				return errors.Reason("computed and supplied variant hash don't match").Err()
			}
		}
		if hasVariantHash {
			if err := pbutil.ValidateVariantHash(ex.VariantHash); err != nil {
				return errors.Annotate(err, "variant_hash").Err()
			}
		}

		// Validate the test identifier meets the requirements of the scheme.
		// This is enforced only at upload time.
		if err := validateTestIDToScheme(cfg, testID); err != nil {
			return errors.Annotate(err, "test_id").Err()
		}
	} else {
		// Not a legacy uploader.
		// The TestId, Variant, VariantHash fields are treated as output only as per
		// the API spec and should be ignored. Instead read from the TestIdStructured field.
		// Note that TestIdStructured.ModuleVariantHash is also output only and should
		// also be ignored.

		if err := pbutil.ValidateStructuredTestIdentifier(ex.TestIdStructured); err != nil {
			return errors.Annotate(err, "test_id_structured").Err()
		}
		// Validate the test identifier meets the requirements of the scheme.
		// This is enforced only at upload time.
		if err := validateTestIDToScheme(cfg, pbutil.ExtractBaseTestIdentifier(ex.TestIdStructured)); err != nil {
			return errors.Annotate(err, "test_id_structured").Err()
		}
	}

	if ex.ExplanationHtml == "" {
		return errors.Reason("explanation_html: unspecified").Err()
	}
	if ex.Reason == pb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED {
		return errors.Reason("reason: unspecified").Err()
	}
	return nil
}

// CreateTestExoneration implements pb.RecorderServer.
func (s *recorderServer) CreateTestExoneration(ctx context.Context, in *pb.CreateTestExonerationRequest) (*pb.TestExoneration, error) {
	cfg, err := config.Service(ctx)
	if err != nil {
		return nil, err
	}
	if err := validateCreateTestExonerationRequest(in, cfg, true); err != nil {
		return nil, appstatus.BadRequest(err)
	}
	invID := invocations.MustParseName(in.Invocation)

	ret, mutation := insertTestExoneration(ctx, invID, in.RequestId, 0, in.TestExoneration)
	err = mutateInvocation(ctx, invID, func(ctx context.Context) error {
		span.BufferWrite(ctx, mutation)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return ret, nil
}

func insertTestExoneration(ctx context.Context, invID invocations.ID, requestID string, ordinal int, body *pb.TestExoneration) (*pb.TestExoneration, *spanner.Mutation) {
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

	var testID string
	var variant *pb.Variant
	var variantHash string
	var structuredTestIdentifier *pb.TestIdentifier

	if body.TestIdStructured != nil {
		// Not a legacy uploader.
		// Populate TestId, Variant, VariantHash in the result.
		testID = pbutil.TestIDFromStructuredTestIdentifier(body.TestIdStructured)
		variant = pbutil.VariantFromStructuredTestIdentifier(body.TestIdStructured)
		variantHash = pbutil.VariantHash(variant)

		// Populate the output only fields in StructuredTestIdentifier.
		structuredTestIdentifier = proto.Clone(body.TestIdStructured).(*pb.TestIdentifier)
		pbutil.PopulateStructuredTestIdentifierHashes(structuredTestIdentifier)
	} else {
		// Legacy uploader.
		testID = body.TestId

		// Note this is not always available. This is a data integrity problem where
		// Hash(Variant) != VariantHash and we have missing data in the database, but not
		// much we can do about it until we fix clients away from the hash-only requests.
		variant = body.Variant

		// Use the given variant hash, or the hash of the given variant, whichever
		// is present. If both are present then validation guarantees they'll
		// match, so we can just use whichever.
		variantHash = body.VariantHash
		if variantHash == "" {
			variantHash = pbutil.VariantHash(body.Variant)
		}

		// Do not set StructuredTestIdentifier in the response to legacy requests
		// to minimise changes for these clients.
	}

	exonerationID := fmt.Sprintf("%s:%s", variantHash, exonerationIDSuffix)
	ret := &pb.TestExoneration{
		Name:             pbutil.TestExonerationName(string(invID), testID, exonerationID),
		TestIdStructured: structuredTestIdentifier,
		TestId:           testID,
		Variant:          variant,
		VariantHash:      variantHash,
		ExonerationId:    exonerationID,
		ExplanationHtml:  body.ExplanationHtml,
		Reason:           body.Reason,
	}

	mutation := mutFn("TestExonerations", spanutil.ToSpannerMap(map[string]any{
		"InvocationId":    invID,
		"TestId":          ret.TestId,
		"ExonerationId":   exonerationID,
		"Variant":         ret.Variant,
		"VariantHash":     ret.VariantHash,
		"ExplanationHTML": spanutil.Compressed(ret.ExplanationHtml),
		"Reason":          ret.Reason,
	}))
	return ret, mutation
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
