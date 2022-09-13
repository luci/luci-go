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
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"

	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestResultDB(t *testing.T) {
	Convey("FailureReasonFromResultDB", t, func() {
		rdbFailureReason := &rdbpb.FailureReason{
			PrimaryErrorMessage: "Some error message.",
		}
		fr := FailureReasonFromResultDB(rdbFailureReason)
		So(fr, ShouldResembleProto, &pb.FailureReason{
			PrimaryErrorMessage: "Some error message.",
		})
	})
	Convey("TestMetadataFromResultDB", t, func() {
		rdbTestMetadata := &rdbpb.TestMetadata{
			Name: "name",
			Location: &rdbpb.TestLocation{
				Repo:     "repo",
				FileName: "fileName",
				Line:     123,
			},
		}
		tmd := TestMetadataFromResultDB(rdbTestMetadata)
		So(tmd, ShouldResembleProto, &pb.TestMetadata{
			Name: "name",
			Location: &pb.TestLocation{
				Repo:     "repo",
				FileName: "fileName",
				Line:     123,
			}})
	})
	Convey("TestResultStatusFromResultDB", t, func() {
		// Confirm LUCI Analysis handles every test status defined by ResultDB.
		// This test is designed to break if ResultDB extends the set of
		// allowed values, without a corresponding update to LUCI Analysis.
		for _, v := range rdbpb.TestStatus_value {
			rdbStatus := rdbpb.TestStatus(v)
			if rdbStatus == rdbpb.TestStatus_STATUS_UNSPECIFIED {
				continue
			}

			status := TestResultStatusFromResultDB(rdbStatus)
			So(status, ShouldNotEqual, pb.TestResultStatus_TEST_RESULT_STATUS_UNSPECIFIED)
		}
	})
	Convey("ExonerationReasonFromResultDB", t, func() {
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
			So(reason, ShouldNotEqual, pb.ExonerationReason_EXONERATION_REASON_UNSPECIFIED)
		}
	})
}
