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
	"go.chromium.org/luci/resultdb/pbutil"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	pb "go.chromium.org/luci/analysis/proto/v1"
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

// TestMetadataFromResultDB converts a ResultDB TestMetadata to a LUCI Analysis
// TestMetadata.
func TestMetadataFromResultDB(rdbTmd *rdbpb.TestMetadata) *pb.TestMetadata {
	if rdbTmd == nil {
		return nil
	}

	tmd := &pb.TestMetadata{
		Name: rdbTmd.Name,
	}
	loc := rdbTmd.GetLocation()
	if loc != nil {
		tmd.Location = &pb.TestLocation{
			Repo:     loc.Repo,
			FileName: loc.FileName,
			Line:     loc.Line,
		}
	}

	return tmd
}

// TestResultStatusFromResultDB returns the LUCI Analysis test result status
// corresponding to the given ResultDB test result status.
func TestResultStatusFromResultDB(s rdbpb.TestStatus) pb.TestResultStatus {
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
