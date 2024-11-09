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
	"testing"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestValidateBigQueryExport(t *testing.T) {
	ftt.Run(`ValidateBigQueryExport`, t, func(t *ftt.Test) {
		t.Run(`Valid, Empty TestResults`, func(t *ftt.Test) {
			err := ValidateBigQueryExport(&pb.BigQueryExport{
				Project: "project",
				Dataset: "dataset",
				Table:   "table",
				ResultType: &pb.BigQueryExport_TestResults_{
					TestResults: &pb.BigQueryExport_TestResults{},
				},
			})
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run(`Missing project`, func(t *ftt.Test) {
			err := ValidateBigQueryExport(&pb.BigQueryExport{
				Dataset: "dataset",
				Table:   "table",
			})
			assert.Loosely(t, err, should.ErrLike(`project: unspecified`))
		})

		t.Run(`Missing dataset`, func(t *ftt.Test) {
			err := ValidateBigQueryExport(&pb.BigQueryExport{
				Project: "project",
				Table:   "table",
			})
			assert.Loosely(t, err, should.ErrLike(`dataset: unspecified`))
		})

		t.Run(`Missing table`, func(t *ftt.Test) {
			err := ValidateBigQueryExport(&pb.BigQueryExport{
				Project: "project",
				Dataset: "dataset",
			})
			assert.Loosely(t, err, should.ErrLike(`table: unspecified`))
		})

		t.Run(`Missing ResultType`, func(t *ftt.Test) {
			err := ValidateBigQueryExport(&pb.BigQueryExport{
				Project: "project",
				Dataset: "dataset",
				Table:   "table",
			})
			assert.Loosely(t, err, should.ErrLike(`result_type: unspecified`))
		})

		t.Run(`invalid test result predicate`, func(t *ftt.Test) {
			err := ValidateBigQueryExport(&pb.BigQueryExport{
				Project: "project",
				Dataset: "dataset",
				Table:   "table",
				ResultType: &pb.BigQueryExport_TestResults_{
					TestResults: &pb.BigQueryExport_TestResults{
						Predicate: &pb.TestResultPredicate{
							TestIdRegexp: "(",
						},
					},
				},
			})
			assert.Loosely(t, err, should.ErrLike(`test_results: predicate`))
		})

		t.Run(`invalid artifact predicate`, func(t *ftt.Test) {
			err := ValidateBigQueryExport(&pb.BigQueryExport{
				Project: "project",
				Dataset: "dataset",
				Table:   "table",
				ResultType: &pb.BigQueryExport_TextArtifacts_{
					TextArtifacts: &pb.BigQueryExport_TextArtifacts{
						Predicate: &pb.ArtifactPredicate{
							TestResultPredicate: &pb.TestResultPredicate{
								TestIdRegexp: "(",
							},
						},
					},
				},
			})
			assert.Loosely(t, err, should.ErrLike(`artifacts: predicate`))
		})
	})
}
