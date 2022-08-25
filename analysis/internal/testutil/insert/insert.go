// Copyright 2022 The LUCI Authors.
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

// Package insert implements functions to insert rows for testing purposes.
package insert

import (
	"time"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/analysis/internal"
	"go.chromium.org/luci/analysis/internal/span"
	atvpb "go.chromium.org/luci/analysis/proto/analyzedtestvariant"
)

func updateDict(dest, source map[string]interface{}) {
	for k, v := range source {
		dest[k] = v
	}
}

// AnalyzedTestVariant returns a spanner mutation that inserts an analyzed test variant.
func AnalyzedTestVariant(realm, tId, vHash string, status atvpb.Status, extraValues map[string]interface{}) *spanner.Mutation {
	values := map[string]interface{}{
		"Realm":            realm,
		"TestId":           tId,
		"VariantHash":      vHash,
		"Status":           status,
		"CreateTime":       spanner.CommitTimestamp,
		"StatusUpdateTime": spanner.CommitTimestamp,
	}
	updateDict(values, extraValues)
	return span.InsertMap("AnalyzedTestVariants", values)
}

// Verdict returns a spanner mutation that inserts a Verdicts row.
func Verdict(realm, tId, vHash, invID string, status internal.VerdictStatus, invTime time.Time, extraValues map[string]interface{}) *spanner.Mutation {
	values := map[string]interface{}{
		"Realm":                  realm,
		"TestId":                 tId,
		"VariantHash":            vHash,
		"InvocationId":           invID,
		"Status":                 status,
		"InvocationCreationTime": invTime,
		"IngestionTime":          invTime.Add(time.Hour),
	}
	updateDict(values, extraValues)
	return span.InsertMap("Verdicts", values)
}
