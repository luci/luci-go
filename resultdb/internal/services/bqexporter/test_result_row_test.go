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

package bqexporter

import (
	"testing"

	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/resultdb/pbutil"
	pb "go.chromium.org/luci/resultdb/proto/v1"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGenerateBQRow(t *testing.T) {
	t.Parallel()

	Convey("GenerateBQRow", t, func() {
		input := &rowInput{
			exported: &pb.Invocation{
				Name:       "invocations/exported",
				CreateTime: pbutil.MustTimestampProto(testclock.TestRecentTimeUTC),
				Realm: "testproject:testrealm",
			},
			parent: &pb.Invocation{
				Name: "invocations/parent",
			},
			tr: &pb.TestResult{
				Name: "invocations/parent/tests/t/results/r",
			},
		}
		Convey("TestLocation", func() {
			input.tr.TestLocation = &pb.TestLocation{
				FileName: "//a_test.go",
				Line:     54,
			}
			actual := input.row()
			So(actual.TestLocation, ShouldResemble, &TestLocation{
				FileName: "//a_test.go",
				Line:     54,
			})
		})
	})
}
