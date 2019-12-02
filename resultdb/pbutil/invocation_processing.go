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

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"
)

// ValidateBigQueryExport returns a non-nil error if bq_export is determined to
// be invalid.
func ValidateBigQueryExport(bq_export *pb.BigQueryExport) error {
	switch {
	case bq_export.Project == "":
		return errors.Annotate(unspecified(), "project").Err()
	case bq_export.Dataset == "":
		return errors.Annotate(unspecified(), "dataset").Err()
	case bq_export.Table == "":
		return errors.Annotate(unspecified(), "table").Err()
	case bq_export.GetTestResults() == nil:
		return errors.Annotate(unspecified(), "test_results").Err()
	}

	if bq_export.TestResults.GetPredicate() == nil {
		return nil
	}

	if err := ValidateTestResultPredicate(bq_export.TestResults.Predicate); err != nil {
		return errors.Annotate(err, "test_results: predicate").Err()
	}

	return nil
}
