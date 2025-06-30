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

package pbutil

import (
	"encoding/hex"
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestResultDB(t *testing.T) {
	ftt.Run("FailureReasonFromResultDB", t, func(t *ftt.Test) {
		rdbFailureReason := &rdbpb.FailureReason{
			PrimaryErrorMessage: "Some error message.",
		}
		fr := FailureReasonFromResultDB(rdbFailureReason)
		assert.Loosely(t, fr, should.Match(&pb.FailureReason{
			PrimaryErrorMessage: "Some error message.",
		}))
	})
	ftt.Run("LegacyTestStatusFromResultDB", t, func(t *ftt.Test) {
		// Confirm LUCI Analysis handles every test status defined by ResultDB.
		// This test is designed to break if ResultDB extends the set of
		// allowed values, without a corresponding update to LUCI Analysis.
		for _, v := range rdbpb.TestStatus_value {
			rdbStatus := rdbpb.TestStatus(v)
			if rdbStatus == rdbpb.TestStatus_STATUS_UNSPECIFIED {
				continue
			}

			status := LegacyTestStatusFromResultDB(rdbStatus)
			assert.Loosely(t, status, should.NotEqual(pb.TestResultStatus_TEST_RESULT_STATUS_UNSPECIFIED))
		}
	})
	ftt.Run("TestStatusV2FromResultDB", t, func(t *ftt.Test) {
		// Confirm LUCI Analysis handles every test status defined by ResultDB.
		// This test is designed to break if ResultDB extends the set of
		// allowed values, without a corresponding update to LUCI Analysis.
		for _, v := range rdbpb.TestResult_Status_value {
			rdbStatus := rdbpb.TestResult_Status(v)
			if rdbStatus == rdbpb.TestResult_STATUS_UNSPECIFIED {
				continue
			}

			status := TestStatusV2FromResultDB(rdbStatus)
			assert.Loosely(t, status, should.NotEqual(pb.TestResult_STATUS_UNSPECIFIED))
		}
	})
	ftt.Run("TestVerdictStatusFromResultDB", t, func(t *ftt.Test) {
		// Confirm LUCI Analysis handles every test variant status defined by ResultDB.
		// This test is designed to break if ResultDB extends the set of
		// allowed values, without a corresponding update to LUCI Analysis.
		for _, v := range rdbpb.TestVariantStatus_value {
			rdbStatus := rdbpb.TestVariantStatus(v)
			if rdbStatus == rdbpb.TestVariantStatus_TEST_VARIANT_STATUS_UNSPECIFIED ||
				rdbStatus == rdbpb.TestVariantStatus_UNEXPECTED_MASK {
				continue
			}

			status := TestVerdictStatusFromResultDB(rdbStatus)
			assert.Loosely(t, status, should.NotEqual(pb.TestVerdictStatus_TEST_VERDICT_STATUS_UNSPECIFIED))
		}
	})
	ftt.Run("TestVerdictStatusV2FromResultDB", t, func(t *ftt.Test) {
		// Confirm LUCI Analysis handles every test verdict status defined by ResultDB.
		// This test is designed to break if ResultDB extends the set of
		// allowed values, without a corresponding update to LUCI Analysis.
		for _, v := range rdbpb.TestVerdict_Status_value {
			rdbStatus := rdbpb.TestVerdict_Status(v)
			if rdbStatus == rdbpb.TestVerdict_STATUS_UNSPECIFIED {
				continue
			}

			status := TestVerdictStatusV2FromResultDB(rdbStatus)
			assert.Loosely(t, status, should.NotEqual(pb.TestVerdict_STATUS_UNSPECIFIED))
		}
	})
	ftt.Run("TestVerdictStatusOverrideFromResultDB", t, func(t *ftt.Test) {
		// Confirm LUCI Analysis handles every test verdict status defined by ResultDB.
		// This test is designed to break if ResultDB extends the set of
		// allowed values, without a corresponding update to LUCI Analysis.
		for _, v := range rdbpb.TestVerdict_StatusOverride_value {
			rdbStatusOverride := rdbpb.TestVerdict_StatusOverride(v)
			if rdbStatusOverride == rdbpb.TestVerdict_STATUS_OVERRIDE_UNSPECIFIED {
				continue
			}

			status := TestVerdictStatusOverrideFromResultDB(rdbStatusOverride)
			assert.Loosely(t, status, should.NotEqual(pb.TestVerdict_STATUS_OVERRIDE_UNSPECIFIED))
		}
	})
	ftt.Run("ExonerationReasonFromResultDB", t, func(t *ftt.Test) {
		// Confirm LUCI Analysis handles every exoneration reason defined by
		// ResultDB.
		// This test is designed to break if ResultDB extends the set of
		// allowed values, without a corresponding update to LUCI Analysis.
		for _, v := range rdbpb.ExonerationReason_value {
			rdbReason := rdbpb.ExonerationReason(v)
			if rdbReason == rdbpb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED {
				continue
			}

			reason := ExonerationReasonFromResultDB(rdbReason)
			assert.Loosely(t, reason, should.NotEqual(pb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED))
		}
	})
	ftt.Run("Sources to/from ResultDB", t, func(t *ftt.Test) {
		rdbSources := &rdbpb.Sources{
			GitilesCommit: &rdbpb.GitilesCommit{
				Host:       "project.googlesource.com",
				Project:    "myproject/src",
				Ref:        "refs/heads/main",
				CommitHash: "abcdefabcd1234567890abcdefabcd1234567890",
				Position:   16801,
			},
			Changelists: []*rdbpb.GerritChange{
				{
					Host:     "project-review.googlesource.com",
					Project:  "myproject/src2",
					Change:   9991,
					Patchset: 82,
				},
			},
			IsDirty: true,
		}
		analysisSources := &pb.Sources{
			GitilesCommit: &pb.GitilesCommit{
				Host:       "project.googlesource.com",
				Project:    "myproject/src",
				Ref:        "refs/heads/main",
				CommitHash: "abcdefabcd1234567890abcdefabcd1234567890",
				Position:   16801,
			},
			Changelists: []*pb.GerritChange{
				{
					Host:     "project-review.googlesource.com",
					Project:  "myproject/src2",
					Change:   9991,
					Patchset: 82,
				},
			},
			IsDirty: true,
		}
		assert.Loosely(t, SourcesFromResultDB(rdbSources), should.Match(analysisSources))
		assert.Loosely(t, SourcesToResultDB(analysisSources), should.Match(rdbSources))
	})
	ftt.Run("SourceRef to resultdb", t, func(t *ftt.Test) {
		sourceRef := &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "host",
					Project: "proj",
					Ref:     "ref",
				},
			},
		}
		sourceRef1 := SourceRefToResultDB(sourceRef)
		assert.Loosely(t, sourceRef1, should.Match(&rdbpb.SourceRef{
			System: &rdbpb.SourceRef_Gitiles{
				Gitiles: &rdbpb.GitilesRef{
					Host:    "host",
					Project: "proj",
					Ref:     "ref",
				},
			},
		}))
	})
	ftt.Run("RefFromSources", t, func(t *ftt.Test) {
		sources := &pb.Sources{
			GitilesCommit: &pb.GitilesCommit{
				Host:       "project.googlesource.com",
				Project:    "myproject/src",
				Ref:        "refs/heads/main",
				CommitHash: "abcdefabcd1234567890abcdefabcd1234567890",
				Position:   16801,
			},
		}
		ref := SourceRefFromSources(sources)
		assert.Loosely(t, ref, should.Match(&pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "project.googlesource.com",
					Project: "myproject/src",
					Ref:     "refs/heads/main",
				},
			},
		}))
	})
	ftt.Run("RefHash", t, func(t *ftt.Test) {
		ref := &pb.SourceRef{
			System: &pb.SourceRef_Gitiles{
				Gitiles: &pb.GitilesRef{
					Host:    "project.googlesource.com",
					Project: "myproject/src",
					Ref:     "refs/heads/main",
				},
			},
		}
		hash := SourceRefHash(ref)
		assert.Loosely(t, hex.EncodeToString(hash), should.Equal(`5d47c679cf080cb5`))
	})
}
