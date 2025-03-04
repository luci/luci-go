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
	"go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	bqpb "go.chromium.org/luci/analysis/proto/bq"
)

// TestVariantIdentifier constructs a BigQuery-format TestVariantIdentifier from
// a flat ResultDB test ID and variant combination.
func TestVariantIdentifier(testID string, variant *rdbpb.Variant) (*bqpb.TestVariantIdentifier, error) {
	test, err := pbutil.ParseAndValidateTestID(testID)
	if err != nil {
		return nil, errors.Annotate(err, "parse test ID").Err()
	}
	variantJSON, err := VariantJSON(variant)
	if err != nil {
		return nil, errors.Annotate(err, "format variant").Err()
	}
	return &bqpb.TestVariantIdentifier{
		ModuleName:        test.ModuleName,
		ModuleScheme:      test.ModuleScheme,
		ModuleVariant:     variantJSON,
		ModuleVariantHash: pbutil.VariantHash(variant),
		CoarseName:        test.CoarseName,
		FineName:          test.FineName,
		CaseName:          test.CaseName,
	}, nil
}
