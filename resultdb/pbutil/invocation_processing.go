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
		return errors.Annotate(validate.Unspecified(), "project").Err()
	case bqExport.Dataset == "":
		return errors.Annotate(validate.Unspecified(), "dataset").Err()
	case bqExport.Table == "":
		return errors.Annotate(validate.Unspecified(), "table").Err()
	}

	switch resultType := bqExport.ResultType.(type) {
	case *pb.BigQueryExport_TestResults_:
		return errors.Annotate(ValidateTestResultPredicate(resultType.TestResults.GetPredicate()), "test_results: predicate").Err()
	case *pb.BigQueryExport_TextArtifacts_:
		return errors.Annotate(ValidateArtifactPredicate(resultType.TextArtifacts.GetPredicate()), "artifacts: predicate").Err()
	case nil:
		return errors.Annotate(validate.Unspecified(), "result_type").Err()
	default:
		panic("impossible")
	}
}
