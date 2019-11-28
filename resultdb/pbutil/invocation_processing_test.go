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

	pb "go.chromium.org/luci/resultdb/proto/rpc/v1"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateBigQueryExport(t *testing.T) {
	Convey(`ValidateBigQueryExport`, t, func() {
		Convey(`Valid, Enpty TestResults`, func() {
			err := validateBigQueryExport(&pb.BigQueryExport{
				Project:     "project",
				Dataset:     "dataset",
				Table:       "table",
				TestResults: &pb.BigQueryExport_TestResults{},
			})
			So(err, ShouldBeNil)
		})

		Convey(`Missing project`, func() {
			err := validateBigQueryExport(&pb.BigQueryExport{
				Dataset: "dataset",
				Table:   "table",
			})
			So(err, ShouldErrLike, `unspecified: project`)
		})

		Convey(`Missing dataset`, func() {
			err := validateBigQueryExport(&pb.BigQueryExport{
				Project: "project",
				Table:   "table",
			})
			So(err, ShouldErrLike, `unspecified: dataset`)
		})

		Convey(`Missing table`, func() {
			err := validateBigQueryExport(&pb.BigQueryExport{
				Project: "project",
				Dataset: "dataset",
			})
			So(err, ShouldErrLike, `unspecified: table`)
		})

		Convey(`Missing TestResults`, func() {
			err := validateBigQueryExport(&pb.BigQueryExport{
				Project: "project",
				Dataset: "dataset",
				Table:   "table",
			})
			So(err, ShouldErrLike, `unspecified: test_results`)
		})

		Convey(`invalid test result predicate`, func() {
			err := validateBigQueryExport(&pb.BigQueryExport{
				Project: "project",
				Dataset: "dataset",
				Table:   "table",
				TestResults: &pb.BigQueryExport_TestResults{
					Predicate: &pb.TestResultPredicate{
						TestPathRegexp: "(",
					},
				},
			})
			So(err, ShouldErrLike, `predicate: test_path_regexp: error parsing regexp`)
		})
	})
}
