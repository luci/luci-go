// Copyright 2023 The LUCI Authors.
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

package datastoreutil

import (
	"context"
	"testing"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/model"

	. "github.com/smartystreets/goconvey/convey"
)

func TestGetTestFailures(t *testing.T) {
	t.Parallel()

	Convey("Get test failures by test id", t, func() {
		c := memory.Use(context.Background())

		Convey("No test failure found", func() {
			testFailures, err := GetTestFailures(c, "testproject", "testID", "testRefHash", "")
			So(err, ShouldBeNil)
			So(testFailures, ShouldHaveLength, 0)
		})

		Convey("Test failure found", func() {
			// Prepare datastore
			testFailure := &model.TestFailure{
				ID:          101,
				Project:     "testproject",
				TestID:      "testID",
				VariantHash: "othertestVariantHash",
				RefHash:     "testRefHash",
			}
			So(datastore.Put(c, testFailure), ShouldBeNil)

			datastore.GetTestable(c).CatchupIndexes()

			testFailures, err := GetTestFailures(c, "testproject", "testID", "testRefHash", "")
			So(err, ShouldBeNil)
			So(testFailures, ShouldHaveLength, 1)
			So(testFailures[0].ID, ShouldEqual, 101)
		})
	})

	Convey("Get test failures by test variant", t, func() {
		c := memory.Use(context.Background())

		Convey("No test failure found", func() {
			testFailures, err := GetTestFailures(c, "testproject", "testID", "testRefHash", "testVariantHash")
			So(err, ShouldBeNil)
			So(testFailures, ShouldHaveLength, 0)
		})

		Convey("Test failure found", func() {
			// Prepare datastore
			testFailure := &model.TestFailure{
				ID:          102,
				Project:     "testproject",
				TestID:      "testID",
				VariantHash: "testVariantHash",
				RefHash:     "testRefHash",
			}
			So(datastore.Put(c, testFailure), ShouldBeNil)

			datastore.GetTestable(c).CatchupIndexes()

			testFailures, err := GetTestFailures(c, "testproject", "testID", "testRefHash", "testVariantHash")
			So(err, ShouldBeNil)
			So(testFailures, ShouldHaveLength, 1)
			So(testFailures[0].ID, ShouldEqual, 102)
		})
	})
}
