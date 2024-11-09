// Copyright 2023 The LUCI Authors.
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

// Package baselinetestvariant provides crud operations for baseline test variants.
package baselinetestvariant

import (
	"context"
	"time"

	"cloud.google.com/go/spanner"
	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/errors"

	"go.chromium.org/luci/resultdb/internal/spanutil"
)

// BaselineTestVariant captures a test variant mapped to a baseline identifier.
// BaselineTestVariants are used to when determining new tests.
type BaselineTestVariant struct {
	Project     string
	BaselineID  string
	TestID      string
	VariantHash string
	LastUpdated time.Time
}

var NotFound = errors.New("baseline test variant not found")

// Read reads one baseline test variant from Spanner.
// If the invocation does not exist, NotFound error is returned.
func Read(ctx context.Context, project, baselineID, testID, variantHash string) (*BaselineTestVariant, error) {
	key := spanner.Key{
		project,
		baselineID,
		testID,
		variantHash,
	}

	var proj, bID, tID, vHash string
	var lastUpdated time.Time

	err := spanutil.ReadRow(ctx, "BaselineTestVariants", key, map[string]any{
		"Project":     &proj,
		"BaselineId":  &bID,
		"TestId":      &tID,
		"VariantHash": &vHash,
		"LastUpdated": &lastUpdated,
	})
	if err != nil {
		if spanner.ErrCode(err) == codes.NotFound {
			err = NotFound
		}
		return &BaselineTestVariant{}, err
	}

	res := &BaselineTestVariant{
		Project:     proj,
		BaselineID:  bID,
		TestID:      tID,
		VariantHash: vHash,
		LastUpdated: lastUpdated,
	}
	return res, nil
}

// Create returns a Spanner mutation that creates the Baseline Test Variant, setting LastUpdated to spanner.CommitTimestamp
func Create(project, baselineID, testID, variantHash string) *spanner.Mutation {
	row := map[string]any{
		"Project":     project,
		"BaselineId":  baselineID,
		"TestId":      testID,
		"VariantHash": variantHash,
		"LastUpdated": spanner.CommitTimestamp,
	}
	return spanutil.InsertMap("BaselineTestVariants", row)
}

var baselineTestVariantCols = []string{
	"Project", "BaselineId", "TestId", "VariantHash", "LastUpdated",
}

// InsertOrUpdate returns a Spanner mutation for inserting/updating a baseline test variant
func InsertOrUpdate(project, baselineID, testID, variantHash string) *spanner.Mutation {
	// Specifying values in a slice directly instead of creating maps to save
	// wasted CPU cycles of generating maps and converting back to slice for
	// large data sets such as test results. See
	// go/src/go.chromium.org/luci/analysis/internal/testresults/span.go
	vals := []any{
		project, baselineID, testID, variantHash, spanner.CommitTimestamp,
	}
	return spanner.InsertOrUpdate("BaselineTestVariants", baselineTestVariantCols, vals)
}
