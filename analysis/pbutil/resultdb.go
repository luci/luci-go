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

// Package pbutil contains methods for manipulating LUCI Analysis protos.
package pbutil

import (
	pb "go.chromium.org/luci/analysis/proto/v1"
	"go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
)

// TestResultIDFromResultDB returns a LUCI Analysis TestResultId corresponding
// to the supplied ResultDB test result name.
// The format of name should be:
// "invocations/{INVOCATION_ID}/tests/{URL_ESCAPED_TEST_ID}/results/{RESULT_ID}".
func TestResultIDFromResultDB(name string) *pb.TestResultId {
	return &pb.TestResultId{System: "resultdb", Id: name}
}

// VariantFromResultDB returns a LUCI Analysis Variant corresponding to the
// supplied ResultDB Variant.
func VariantFromResultDB(v *rdbpb.Variant) *pb.Variant {
	if v == nil {
		// Variant is optional in ResultDB.
		return &pb.Variant{Def: make(map[string]string)}
	}
	return &pb.Variant{Def: v.Def}
}

// VariantToResultDB returns a ResultDB Variant corresponding to the
// supplied LUCI Analysis Variant.
func VariantToResultDB(v *pb.Variant) *rdbpb.Variant {
	if v == nil {
		return &rdbpb.Variant{Def: make(map[string]string)}
	}
	return &rdbpb.Variant{Def: v.Def}
}

// VariantHash returns a hash of the variant.
func VariantHash(v *pb.Variant) string {
	return pbutil.VariantHash(VariantToResultDB(v))
}

// StringPairFromResultDB returns a LUCI Analysis StringPair corresponding to
// the supplied ResultDB StringPair.
func StringPairFromResultDB(v []*rdbpb.StringPair) []*pb.StringPair {
	pairs := []*pb.StringPair{}
	for _, pair := range v {
		pairs = append(pairs, &pb.StringPair{Key: pair.Key, Value: pair.Value})
	}
	return pairs
}

// FailureReasonFromResultDB returns a LUCI Analysis FailureReason
// corresponding to the supplied ResultDB FailureReason.
func FailureReasonFromResultDB(fr *rdbpb.FailureReason) *pb.FailureReason {
	if fr == nil {
		return nil
	}
	return &pb.FailureReason{
		PrimaryErrorMessage: fr.PrimaryErrorMessage,
	}
}

// LegacyTestStatusFromResultDB returns the LUCI Analysis test result status
// corresponding to the given ResultDB test result status.
func LegacyTestStatusFromResultDB(s rdbpb.TestStatus) pb.TestResultStatus {
	switch s {
	case rdbpb.TestStatus_ABORT:
		return pb.TestResultStatus_ABORT
	case rdbpb.TestStatus_CRASH:
		return pb.TestResultStatus_CRASH
	case rdbpb.TestStatus_FAIL:
		return pb.TestResultStatus_FAIL
	case rdbpb.TestStatus_PASS:
		return pb.TestResultStatus_PASS
	case rdbpb.TestStatus_SKIP:
		return pb.TestResultStatus_SKIP
	default:
		return pb.TestResultStatus_TEST_RESULT_STATUS_UNSPECIFIED
	}
}

// TestStatusV2FromResultDB returns the LUCI Analysis test result status
// corresponding to the ResultDB test status.
func TestStatusV2FromResultDB(s rdbpb.TestResult_Status) pb.TestResult_Status {
	switch s {
	case rdbpb.TestResult_PASSED:
		return pb.TestResult_PASSED
	case rdbpb.TestResult_FAILED:
		return pb.TestResult_FAILED
	case rdbpb.TestResult_SKIPPED:
		return pb.TestResult_SKIPPED
	case rdbpb.TestResult_EXECUTION_ERRORED:
		return pb.TestResult_EXECUTION_ERRORED
	case rdbpb.TestResult_PRECLUDED:
		return pb.TestResult_PRECLUDED
	default:
		return pb.TestResult_STATUS_UNSPECIFIED
	}
}

// TestVerdictStatusFromResultDB returns the LUCI Analysis test verdict status
// corresponding to the given ResultDB test variant status.
func TestVerdictStatusFromResultDB(s rdbpb.TestVariantStatus) pb.TestVerdictStatus {
	switch s {
	case rdbpb.TestVariantStatus_EXONERATED:
		return pb.TestVerdictStatus_EXONERATED
	case rdbpb.TestVariantStatus_EXPECTED:
		return pb.TestVerdictStatus_EXPECTED
	case rdbpb.TestVariantStatus_FLAKY:
		return pb.TestVerdictStatus_FLAKY
	case rdbpb.TestVariantStatus_UNEXPECTED:
		return pb.TestVerdictStatus_UNEXPECTED
	case rdbpb.TestVariantStatus_UNEXPECTEDLY_SKIPPED:
		return pb.TestVerdictStatus_UNEXPECTEDLY_SKIPPED
	default:
		return pb.TestVerdictStatus_TEST_VERDICT_STATUS_UNSPECIFIED
	}
}

// TestVerdictStatusV2FromResultDB returns the LUCI Analysis test verdict status
// corresponding to the given ResultDB test verdict status.
func TestVerdictStatusV2FromResultDB(s rdbpb.TestVerdict_Status) pb.TestVerdict_Status {
	switch s {
	case rdbpb.TestVerdict_FAILED:
		return pb.TestVerdict_FAILED
	case rdbpb.TestVerdict_EXECUTION_ERRORED:
		return pb.TestVerdict_EXECUTION_ERRORED
	case rdbpb.TestVerdict_PRECLUDED:
		return pb.TestVerdict_PRECLUDED
	case rdbpb.TestVerdict_FLAKY:
		return pb.TestVerdict_FLAKY
	case rdbpb.TestVerdict_SKIPPED:
		return pb.TestVerdict_SKIPPED
	case rdbpb.TestVerdict_PASSED:
		return pb.TestVerdict_PASSED
	default:
		return pb.TestVerdict_STATUS_UNSPECIFIED
	}
}

// TestVerdictStatusV2FromResultDB returns the LUCI Analysis test verdict status
// corresponding to the given ResultDB test verdict status.
func TestVerdictStatusOverrideFromResultDB(s rdbpb.TestVerdict_StatusOverride) pb.TestVerdict_StatusOverride {
	switch s {
	case rdbpb.TestVerdict_EXONERATED:
		return pb.TestVerdict_EXONERATED
	case rdbpb.TestVerdict_NOT_OVERRIDDEN:
		return pb.TestVerdict_NOT_OVERRIDDEN
	default:
		return pb.TestVerdict_STATUS_OVERRIDE_UNSPECIFIED
	}
}

// ExonerationReasonFromResultDB converts a ResultDB ExonerationReason to a
// LUCI Analysis ExonerationReason.
func ExonerationReasonFromResultDB(s rdbpb.ExonerationReason) pb.ExonerationReason {
	switch s {
	case rdbpb.ExonerationReason_NOT_CRITICAL:
		return pb.ExonerationReason_NOT_CRITICAL
	case rdbpb.ExonerationReason_OCCURS_ON_MAINLINE:
		return pb.ExonerationReason_OCCURS_ON_MAINLINE
	case rdbpb.ExonerationReason_OCCURS_ON_OTHER_CLS:
		return pb.ExonerationReason_OCCURS_ON_OTHER_CLS
	case rdbpb.ExonerationReason_UNEXPECTED_PASS:
		return pb.ExonerationReason_UNEXPECTED_PASS
	default:
		return pb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED
	}
}

// GitilesCommitFromResultDB returns the LUCI Analysis gitiles commit
// corresponding to a ResultDB gitiles commit.
func GitilesCommitFromResultDB(c *rdbpb.GitilesCommit) *pb.GitilesCommit {
	return &pb.GitilesCommit{
		Host:       c.Host,
		Project:    c.Project,
		Ref:        c.Ref,
		CommitHash: c.CommitHash,
		Position:   c.Position,
	}
}

// ChangelistFromResultDB returns the LUCI Analysis gerrit changelist
// corresponding to a ResultDB gerrit changelist.
func ChangelistFromResultDB(cl *rdbpb.GerritChange) *pb.GerritChange {
	return &pb.GerritChange{
		Host:     cl.Host,
		Project:  cl.Project,
		Change:   cl.Change,
		Patchset: cl.Patchset,
	}
}

// SourcesFromResultDB returns the LUCI Analysis source description
// corresponding to a ResultDB source description.
func SourcesFromResultDB(s *rdbpb.Sources) *pb.Sources {
	result := &pb.Sources{
		GitilesCommit: GitilesCommitFromResultDB(s.GitilesCommit),
		IsDirty:       s.IsDirty,
	}
	for _, cl := range s.Changelists {
		result.Changelists = append(result.Changelists, ChangelistFromResultDB(cl))
	}
	return result
}

// GitilesCommitToResultDB returns the ResultDB gitiles commit
// corresponding to a LUCI Analysis gitiles commit.
func GitilesCommitToResultDB(c *pb.GitilesCommit) *rdbpb.GitilesCommit {
	if c == nil {
		return nil
	}
	return &rdbpb.GitilesCommit{
		Host:       c.Host,
		Project:    c.Project,
		Ref:        c.Ref,
		CommitHash: c.CommitHash,
		Position:   c.Position,
	}
}

// ChangelistToResultDB returns the ResultDB gerrit changelist
// corresponding to a LUCI Analysis gerrit changelist.
func ChangelistToResultDB(cl *pb.GerritChange) *rdbpb.GerritChange {
	if cl == nil {
		return nil
	}
	return &rdbpb.GerritChange{
		Host:     cl.Host,
		Project:  cl.Project,
		Change:   cl.Change,
		Patchset: cl.Patchset,
	}
}

// SourcesToResultDB returns the ResultDB source description
// corresponding to a LUCI Analysis source description.
func SourcesToResultDB(s *pb.Sources) *rdbpb.Sources {
	if s == nil {
		return nil
	}
	result := &rdbpb.Sources{
		GitilesCommit: GitilesCommitToResultDB(s.GitilesCommit),
		IsDirty:       s.IsDirty,
	}
	for _, cl := range s.Changelists {
		result.Changelists = append(result.Changelists, ChangelistToResultDB(cl))
	}
	return result
}

// SourceRefToResultDB returns a ResultDB SourceRef corresponding to the
// supplied LUCI Analysis SourceRef.
func SourceRefToResultDB(v *pb.SourceRef) *rdbpb.SourceRef {
	if v == nil {
		return nil
	}
	// Assert system should be Gitiles
	if _, ok := v.System.(*pb.SourceRef_Gitiles); !ok {
		panic("system should be gitiles")
	}
	return &rdbpb.SourceRef{
		System: &rdbpb.SourceRef_Gitiles{
			Gitiles: &rdbpb.GitilesRef{
				Host:    v.GetGitiles().Host,
				Project: v.GetGitiles().Project,
				Ref:     v.GetGitiles().Ref,
			},
		},
	}
}

// SourceRefFromSources extracts a SourceRef from given sources.
//
// panics if the sources object is not valid.
func SourceRefFromSources(sources *pb.Sources) *pb.SourceRef {
	return &pb.SourceRef{
		System: &pb.SourceRef_Gitiles{
			Gitiles: &pb.GitilesRef{
				Host:    sources.GitilesCommit.Host,
				Project: sources.GitilesCommit.Project,
				Ref:     sources.GitilesCommit.Ref,
			},
		},
	}
}

// SourceRefHash returns a short hash of the source ref.
func SourceRefHash(sourceRef *pb.SourceRef) []byte {
	return pbutil.SourceRefHash(SourceRefToResultDB(sourceRef))
}

// SourcePosition returns the position along the source
// ref tested by the given sources.
//
// panics if the sources object is not valid.
func SourcePosition(sources *pb.Sources) int64 {
	return sources.GitilesCommit.Position
}
