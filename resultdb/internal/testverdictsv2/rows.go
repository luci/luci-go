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
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/resultdb/internal/testexonerationsv2"
	"go.chromium.org/luci/resultdb/internal/testresultsv2"
	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

const (
	// StandardVerdictSizeLimit is the recommended size limit, in bytes, of each test verdict.
	//
	// A limit must be enforced to avoid verdicts hitting limits such as:
	// - BigQuery's 10 MiB per row limit
	// - Spanner's 10 MiB per request limit
	// - Pubsub's 10 MiB message size limit
	StandardVerdictSizeLimit = 2 * 1024 * 1024

	// StandardVerdictResultLimit is the standard limit on the number of results and
	// exonerations returned in a verdict.
	//
	// This avoids clients running into issues, e.g. Spanner's 80,000 mutation per
	// commit limit.
	StandardVerdictResultLimit = 500
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
	// The number of test results in the verdict.
	ResultCount int64
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
//
// If an attempt was made to retrieve verdicts by ID, and the verdict was not
// found, an empty TestVerdict with only Ordinal set may be retruend.
type TestVerdict struct {
	// The identifier of the test verdict.
	ID testresultsv2.VerdictID
	// The test results in the verdict.
	Results []*testresultsv2.TestResultRow
	// The test exonerations that make up the verdict.
	Exonerations []*testexonerationsv2.TestExonerationRow
	// The one-based index into Query.VerdictIDs this result relates to. Only set
	// if the result is retrieved using a query for nominated verdict IDs. Output only.
	RequestOrdinal int
}

// ToProto converts the given TestVerdict into its proto representation, obeying
// dual (result count and size) limits.
func (v *TestVerdict) ToProto(resultLimit int, verdictSizeLimit int) *pb.TestVerdict {
	if len(v.Results) == 0 {
		// This is an empty verdict, e.g. placeholder for a requested verdict that was
		// not found.
		return nil
	}

	tv := &pb.TestVerdict{}

	// Take the first non-nil test metadata and module variant.
	var moduleVariant *pb.Variant
	var testMetadata *pb.TestMetadata
	for _, result := range v.Results {
		if testMetadata == nil && result.TestMetadata != nil {
			testMetadata = result.TestMetadata
		}
		if moduleVariant == nil && result.ModuleVariant != nil {
			moduleVariant = result.ModuleVariant
		}
	}

	tv.TestIdStructured = &pb.TestIdentifier{
		ModuleName:        v.Results[0].ID.ModuleName,
		ModuleScheme:      v.Results[0].ID.ModuleScheme,
		ModuleVariant:     moduleVariant,
		ModuleVariantHash: v.Results[0].ID.ModuleVariantHash,
		CoarseName:        v.Results[0].ID.CoarseName,
		FineName:          v.Results[0].ID.FineName,
		CaseName:          v.Results[0].ID.CaseName,
	}
	if moduleVariant == nil {
		tv.IsMasked = true
	}
	tv.TestId = pbutil.EncodeTestID(pbutil.ExtractBaseTestIdentifier(tv.TestIdStructured))
	tv.TestMetadata = testMetadata

	// To avoid inaccurate statuses, the status should be computed from all results before
	// truncation.
	tv.Status = statusV2FromResults(v.Results)
	isExonerable := (tv.Status == pb.TestVerdict_FAILED || tv.Status == pb.TestVerdict_EXECUTION_ERRORED || tv.Status == pb.TestVerdict_PRECLUDED || tv.Status == pb.TestVerdict_FLAKY)
	if len(v.Exonerations) > 0 && isExonerable {
		tv.StatusOverride = pb.TestVerdict_EXONERATED
	} else {
		tv.StatusOverride = pb.TestVerdict_NOT_OVERRIDDEN
	}

	totalSize := protoJSONOverheadBytes + proto.Size(tv)

	// Alternate between adding results and exonerations to ensure fairness
	// in how the available bytes are used.
	for i := range max(len(v.Results), len(v.Exonerations)) {
		if i >= resultLimit {
			break
		}
		if i < len(v.Results) {
			resultProto := v.Results[i].ToProto()
			// Unset fields that are lifted up to the verdict level to reduce
			// response size.
			resultProto.TestId = ""
			resultProto.TestIdStructured = nil
			resultProto.Variant = nil
			resultProto.VariantHash = ""
			resultProto.TestMetadata = nil

			// Add a few extra bytes to capture the cost of embedding
			// the result inside the parent verdict. Five should be conservative.
			// https://protobuf.dev/programming-guides/encoding/#embedded
			size := proto.Size(resultProto) + 5
			if (totalSize + size) > verdictSizeLimit {
				// We are full.
				break
			}
			tv.Results = append(tv.Results, resultProto)
			totalSize += size
		}
		if i < len(v.Exonerations) {
			exonerationProto := v.Exonerations[i].ToProto()
			// Unset fields that are lifted up to the verdict level to reduce
			// response size.
			exonerationProto.TestIdStructured = nil
			exonerationProto.TestId = ""
			exonerationProto.Variant = nil
			exonerationProto.VariantHash = ""

			// Add a few extra bytes to capture the cost of embedding
			// the exoneration inside the parent verdict. Five should be conservative.
			// https://protobuf.dev/programming-guides/encoding/#embedded
			size := proto.Size(exonerationProto) + 5
			if (totalSize + size) > verdictSizeLimit {
				// We are full.
				break
			}
			tv.Exonerations = append(tv.Exonerations, exonerationProto)
			totalSize += size
		}
	}
	return tv
}
