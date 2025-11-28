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
	"go.chromium.org/luci/server/auth"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/internal/workunits"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func validateTestExoneration(ex *pb.TestExoneration, cfg *config.CompiledServiceConfig, strictValidation bool) error {
	if ex == nil {
		return errors.New("unspecified")
	}
	if ex.TestIdStructured == nil && ex.TestId != "" {
		// For backwards compatibility, we still accept legacy uploaders setting
		// the test_id and variant or variant_hash fields (even though they are
		// officially OUTPUT_ONLY now).
		testID, err := pbutil.ParseAndValidateTestID(ex.TestId)
		if err != nil {
			return errors.Fmt("test_id: %w", err)
		}
		// Legacy clients may not set Variant, or use nil to represent
		// the empty variant. As this is ambiguous, we rely on the absence
		// of VariantHash being set to determine if Variant was deliberately
		// set to nil or if the Variant is simply not set.
		//
		// In strict validation: the variant is always required.
		if ex.Variant != nil || strictValidation {
			if err := pbutil.ValidateVariant(ex.GetVariant()); err != nil {
				return errors.Fmt("variant: %w", err)
			}
		}

		// Some legacy clients do not set variant and only set variant_hash.
		// Some set both. If both are set, check they are consistent.
		hasVariant := len(ex.Variant.GetDef()) != 0 || strictValidation
		hasVariantHash := ex.VariantHash != ""
		if hasVariant && hasVariantHash {
			computedHash := pbutil.VariantHash(ex.GetVariant())
			if computedHash != ex.VariantHash {
				return errors.New("computed and supplied variant hash don't match")
			}
		}
		if hasVariantHash {
			if err := pbutil.ValidateVariantHash(ex.VariantHash); err != nil {
				return errors.Fmt("variant_hash: %w", err)
			}
		}

		// Validate the test identifier meets the requirements of the scheme.
		// This is enforced only at upload time.
		if err := validateTestIDToScheme(cfg, testID); err != nil {
			return errors.Fmt("test_id: %w", err)
		}
	} else {
		// Not a legacy uploader.
		// The TestId, Variant, VariantHash fields are treated as output only as per
		// the API spec and should be ignored. Instead read from the TestIdStructured field.
		// Note that TestIdStructured.ModuleVariantHash is also output only and should
		// also be ignored.

		if err := pbutil.ValidateStructuredTestIdentifierForStorage(ex.TestIdStructured); err != nil {
			return errors.Fmt("test_id_structured: %w", err)
		}
		// Validate the test identifier meets the requirements of the scheme.
		// This is enforced only at upload time.
		if err := validateTestIDToScheme(cfg, pbutil.ExtractBaseTestIdentifier(ex.TestIdStructured)); err != nil {
			return errors.Fmt("test_id_structured: %w", err)
		}
	}

	if ex.ExplanationHtml == "" {
		return errors.New("explanation_html: unspecified")
	}
	if ex.Reason == pb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED {
		return errors.New("reason: unspecified")
	}
	return nil
}

// CreateTestExoneration implements pb.RecorderServer.
func (s *recorderServer) CreateTestExoneration(ctx context.Context, in *pb.CreateTestExonerationRequest) (*pb.TestExoneration, error) {
	// Piggy back on BatchCreateTestExonerations.
	res, err := s.BatchCreateTestExonerations(ctx, &pb.BatchCreateTestExonerationsRequest{
		Parent:     in.Parent,
		Invocation: in.Invocation,
		Requests:   []*pb.CreateTestExonerationRequest{in},
		RequestId:  in.RequestId,
	})
	if err != nil {
		// Remove any references to "requests[0]: ", this is a single create RPC not a batch RPC.
		return nil, removeRequestNumberFromAppStatusError(err)
	}
	return res.TestExonerations[0], nil
}

// normalizeTestExoneration converts a TestExoneration provided in a request to its
// normal proto form, as should be returned in the response. This includes:
//   - assigning an exoenration ID
//   - populating the name field
//   - populating output only fields
//   - compatibility logic for legacy clients which set TestId and Variant instead of
//     TestIdStructured
//
// Either workUnitID or invID should be specified, not both.
func normalizeTestExoneration(ctx context.Context, workUnitID workunits.ID, invID invocations.ID, requestID string, ordinal int, body *pb.TestExoneration) *pb.TestExoneration {
	if workUnitID == (workunits.ID{}) && invID == "" {
		panic("either work unit ID or invocation ID must be set")
	}

	// Compute exoneration ID and choose Insert vs InsertOrUpdate.
	var exonerationIDSuffix string
	if requestID == "" {
		// Use a random id.
		exonerationIDSuffix = "r:" + uuid.New().String()
	} else {
		// Use a deterministic id.
		exonerationIDSuffix = "d:" + deterministicExonerationIDSuffix(ctx, requestID, ordinal)
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

		// Unfortunately some legacy clients can provide the VariantHash, but not the Variant
		// and this is accepted as valid input for legacy reasons.

		// Note this is not always available. This is a data integrity problem where
		// Hash(Variant) != VariantHash and we have missing data in the database, but not
		// much we can do about it until we fix clients away from the hash-only requests.
		variant = body.Variant
		if variant == nil {
			// Prefer to return empty variant instead of nil variant to avoid ambiguities
			// between setting an empty variant vs no variant.
			variant = &pb.Variant{}
		}

		// Use the given variant hash, or the hash of the given variant, whichever
		// is present. If both are present then validation guarantees they'll
		// match, so we can just use whichever.
		variantHash = body.VariantHash
		if variantHash == "" {
			variantHash = pbutil.VariantHash(body.Variant)
		}

		// Legacy test uploader. Populate TestIdStructured from TestId and Variant
		// so that this RPC returns the same response as GetTestExoneration.
		var err error
		structuredTestIdentifier, err = pbutil.ParseStructuredTestIdentifierForOutput(testID, variant)
		if err != nil {
			// This should not happen, the test identifier should already have been validated.
			panic(fmt.Errorf("parse test identifier: %w", err))
		}
		// Override the computed variant hash with the user-supplied variant hash,
		// see point about being able to set VariantHash without setting the variant.
		structuredTestIdentifier.ModuleVariantHash = variantHash
	}

	exonerationID := fmt.Sprintf("%s:%s", variantHash, exonerationIDSuffix)
	if err := pbutil.ValidateExonerationID(exonerationID); err != nil {
		panic(fmt.Sprintf("server-generated exoneration ID is not valid: %q: %s", exonerationID, err))
	}

	var name string
	if invID != "" {
		name = pbutil.LegacyTestExonerationName(string(invID), testID, exonerationID)
	} else {
		name = pbutil.TestExonerationName(string(workUnitID.RootInvocationID), workUnitID.WorkUnitID, testID, exonerationID)
	}

	ret := &pb.TestExoneration{
		Name:             name,
		TestIdStructured: structuredTestIdentifier,
		TestId:           testID,
		Variant:          variant,
		VariantHash:      variantHash,
		ExonerationId:    exonerationID,
		ExplanationHtml:  body.ExplanationHtml,
		Reason:           body.Reason,
		IsMasked:         false,
	}
	return ret
}

// insertTestExoneration creates a test exoneration insertion mutation with the given properties.
func insertTestExoneration(invID invocations.ID, body *pb.TestExoneration) *spanner.Mutation {
	return spanner.InsertOrUpdateMap("TestExonerations", spanutil.ToSpannerMap(map[string]any{
		"InvocationId":    invID,
		"TestId":          body.TestId,
		"ExonerationId":   body.ExonerationId,
		"Variant":         body.Variant,
		"VariantHash":     body.VariantHash,
		"ExplanationHTML": spanutil.Compressed(body.ExplanationHtml),
		"Reason":          body.Reason,
	}))
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
