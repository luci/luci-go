// Copyright 2026 The LUCI Authors.
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

package testverdictsv2

import (
	"go.chromium.org/luci/resultdb/internal/testexonerationsv2"
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// TestVerdictSummary represents the summary of a test verdict. It corresponds
// to the fields present on the BASIC test verdict view.
type TestVerdictSummary struct {
	// The identifier of the test verdict.
	ID testresultsv2.VerdictID
	// The module variant.
	ModuleVariant *pb.Variant
	// The status of the test verdict.
	Status pb.TestVerdict_Status
	// The status override of the test verdict.
	StatusOverride pb.TestVerdict_StatusOverride
	// Whether the variant of the verdict has been masked, because the
	// user only has limited access.
	IsMasked bool
	// The UI Priority of the verdict.
	UIPriority int64
}

func (v *TestVerdictSummary) ToProto() *pb.TestVerdict {
	tv := &pb.TestVerdict{
		TestIdStructured: &pb.TestIdentifier{
			ModuleName:        v.ID.ModuleName,
			ModuleScheme:      v.ID.ModuleScheme,
			ModuleVariantHash: v.ID.ModuleVariantHash,
			CoarseName:        v.ID.CoarseName,
			FineName:          v.ID.FineName,
			CaseName:          v.ID.CaseName,
			ModuleVariant:     v.ModuleVariant,
		},
		Status:         v.Status,
		StatusOverride: v.StatusOverride,
		IsMasked:       v.IsMasked,
	}
	tv.TestId = pbutil.EncodeTestID(pbutil.ExtractBaseTestIdentifier(tv.TestIdStructured))
	return tv
}

// TestVerdict represents a test verdict. It corresponds to the fields
// present on the FULL test verdict view.
type TestVerdict struct {
	// The identifier of the test verdict.
	ID testresultsv2.VerdictID
	// The test results in the verdict.
	Results []*testresultsv2.TestResultRow
	// The test exonerations that make up the verdict.
	Exonerations []*testexonerationsv2.TestExonerationRow
	// The test metadata of the verdict. This is picked from one of the results.
	TestMetadata *pb.TestMetadata
}

func (v *TestVerdict) ToProto(resultLimit int) *pb.TestVerdict {
	tv := &pb.TestVerdict{}

	// To avoid inaccurate statuses, the status should be computed from all results before
	// truncation.
	tv.Status = statusV2FromResults(v.Results)
	isExonerable := (tv.Status == pb.TestVerdict_FAILED || tv.Status == pb.TestVerdict_EXECUTION_ERRORED || tv.Status == pb.TestVerdict_PRECLUDED || tv.Status == pb.TestVerdict_FLAKY)
	if len(v.Exonerations) > 0 && isExonerable {
		tv.StatusOverride = pb.TestVerdict_EXONERATED
	} else {
		tv.StatusOverride = pb.TestVerdict_NOT_OVERRIDDEN
	}

	for i, result := range v.Results {
		resultProto := result.ToProto()
		if i == 0 {
			// Lift test ID and metadata up to the verdict level.
			tv.TestId = resultProto.TestId
			tv.TestIdStructured = resultProto.TestIdStructured
		}
		if tv.TestMetadata == nil && result.TestMetadata != nil {
			// Take the first non-nil test metadata.
			tv.TestMetadata = result.TestMetadata
		}

		if len(tv.Results) < resultLimit {
			// Unset fields that are lifted up to the verdict level to reduce
			// response size.
			resultProto.TestId = ""
			resultProto.TestIdStructured = nil
			resultProto.Variant = nil
			resultProto.VariantHash = ""
			resultProto.TestMetadata = nil
			tv.Results = append(tv.Results, resultProto)
		}
	}

	for _, exoneration := range v.Exonerations {
		if len(tv.Exonerations) < resultLimit {
			exonerationProto := exoneration.ToProto()
			// Unset fields that are lifted up to the verdict level to reduce
			// response size.
			exonerationProto.TestIdStructured = nil
			exonerationProto.TestId = ""
			exonerationProto.Variant = nil
			exonerationProto.VariantHash = ""
			tv.Exonerations = append(tv.Exonerations, exonerationProto)
		}
	}
	return tv
}
