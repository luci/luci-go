// Copyright 2019 The LUCI Authors.
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
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/validate"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

// ValidateBigQueryExport returns a non-nil error if bqExport is determined to
// be invalid.
func ValidateBigQueryExport(bqExport *pb.BigQueryExport) error {
	switch {
	case bqExport.Project == "":
		return errors.Fmt("project: %w", validate.Unspecified())
	case bqExport.Dataset == "":
		return errors.Fmt("dataset: %w", validate.Unspecified())
	case bqExport.Table == "":
		return errors.Fmt("table: %w", validate.Unspecified())
	}

	switch resultType := bqExport.ResultType.(type) {
	case *pb.BigQueryExport_TestResults_:
		return errors.WrapIf(ValidateTestResultPredicate(resultType.TestResults.GetPredicate()), "test_results: predicate")
	case *pb.BigQueryExport_TextArtifacts_:
		return errors.WrapIf(ValidateArtifactPredicate(resultType.TextArtifacts.GetPredicate(), false), "artifacts: predicate")
	case nil:
		return errors.Fmt("result_type: %w", validate.Unspecified())
	default:
		panic("impossible")
	}
}
