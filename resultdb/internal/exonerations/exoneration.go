// Copyright 2020 The LUCI Authors.
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

package exonerations

import (
	"context"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/grpc/appstatus"

	"go.chromium.org/luci/resultdb/internal/invocations"
	"go.chromium.org/luci/resultdb/internal/spanutil"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// MustParseName extracts invocation, test id and exoneration
// IDs from the name.
// Panics on failure.
func MustParseName(name string) (invID invocations.ID, testID, exonerationID string) {
	invIDStr, testID, exonerationID, err := pbutil.ParseTestExonerationName(name)
	if err != nil {
		panic(err)
	}
	invID = invocations.ID(invIDStr)
	return
}

// Read reads a test exoneration from Spanner.
// If it does not exist, the returned error is annotated with NotFound GRPC
// code.
func Read(ctx context.Context, name string) (*pb.TestExoneration, error) {
	invIDStr, testID, exonerationID, err := pbutil.ParseTestExonerationName(name)
	if err != nil {
		return nil, err
	}
	invID := invocations.ID(invIDStr)

	ret := &pb.TestExoneration{
		Name:          name,
		TestId:        testID,
		ExonerationId: exonerationID,
	}

	// Populate fields from TestExonerations table.
	var explanationHTML spanutil.Compressed
	err = spanutil.ReadRow(ctx, "TestExonerations", invID.Key(testID, exonerationID), map[string]any{
		"Variant":         &ret.Variant,
		"VariantHash":     &ret.VariantHash,
		"ExplanationHTML": &explanationHTML,
		"Reason":          &ret.Reason,
	})
	switch {
	case spanner.ErrCode(err) == codes.NotFound:
		return nil, appstatus.Attachf(err, codes.NotFound, "%s not found", ret.Name)

	case err != nil:
		return nil, errors.Annotate(err, "fetch %q", ret.Name).Err()

	default:
	}

	ret.ExplanationHtml = string(explanationHTML)
	ret.TestVariantIdentifier, err = pbutil.ParseTestVariantIdentifier(ret.TestId, ret.Variant)
	if err != nil {
		return nil, errors.Annotate(err, "parse test variant identifier").Err()
	}
	// Clients uploading data using the legacy API (test_id + variant/variant_hash) were
	// erroneously allowed to set variant_hash only and not the variant. This means the
	// hash of the variant is not always the variant_hash.
	// Set ModuleVariantHash directly to the stored variant hash as a work around,
	// do not compute it.
	ret.TestVariantIdentifier.ModuleVariantHash = ret.VariantHash
	return ret, nil
}
