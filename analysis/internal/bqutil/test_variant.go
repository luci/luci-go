// Copyright 2025 The LUCI Authors.
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

package bqutil

import (
	"go.chromium.org/luci/common/errors"
	rdbpbutil "go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	"go.chromium.org/luci/analysis/pbutil"
	bqpb "go.chromium.org/luci/analysis/proto/bq"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

// StructuredTestIdentifier constructs a BigQuery-format TestIdentifier from
// a flat ResultDB test ID and variant combination.
func StructuredTestIdentifier(testID string, variant *pb.Variant) (*bqpb.TestIdentifier, error) {
	return StructuredTestIdentifierRDB(testID, pbutil.VariantToResultDB(variant))
}

// StructuredTestIdentifierRDB constructs a BigQuery-format TestIdentifier from
// a flat ResultDB test ID and variant combination.
func StructuredTestIdentifierRDB(testID string, variant *rdbpb.Variant) (*bqpb.TestIdentifier, error) {
	test, err := rdbpbutil.ParseAndValidateTestID(testID)
	if err != nil {
		return nil, errors.Fmt("parse test ID: %w", err)
	}
	variantJSON, err := VariantJSON(variant)
	if err != nil {
		return nil, errors.Fmt("format variant: %w", err)
	}
	return &bqpb.TestIdentifier{
		ModuleName:        test.ModuleName,
		ModuleScheme:      test.ModuleScheme,
		ModuleVariant:     variantJSON,
		ModuleVariantHash: rdbpbutil.VariantHash(variant),
		CoarseName:        test.CoarseName,
		FineName:          test.FineName,
		CaseName:          test.CaseName,
	}, nil
}

// TestMetadata prepares a BQ TestMetadata proto corresponding to the
// given ResultDB test metadata proto.
func TestMetadata(rdbTmd *rdbpb.TestMetadata) (*bqpb.TestMetadata, error) {
	if rdbTmd == nil {
		return nil, nil
	}
	// StructPB type is not supported by the BigQuery Write API, so
	// convert it to a JSON string here.
	propertiesJSON, err := MarshalStructPB(rdbTmd.Properties)
	if err != nil {
		return nil, errors.Fmt("marshal properties: %w", err)
	}
	tmd := &bqpb.TestMetadata{
		Name:             rdbTmd.Name,
		Location:         rdbTmd.Location,
		BugComponent:     rdbTmd.BugComponent,
		PropertiesSchema: rdbTmd.PropertiesSchema,
		Properties:       propertiesJSON,
		PreviousTestId:   rdbTmd.PreviousTestId,
	}
	return tmd, nil
}
