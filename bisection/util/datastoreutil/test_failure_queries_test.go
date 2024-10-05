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

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/bisection/model"
)

func TestGetTestFailures(t *testing.T) {
	t.Parallel()

	ftt.Run("Get test failures by test id", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())

		t.Run("No test failure found", func(t *ftt.Test) {
			testFailures, err := GetTestFailures(c, "testproject", "testID", "testRefHash", "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, testFailures, should.HaveLength(0))
		})

		t.Run("Test failure found", func(t *ftt.Test) {
			// Prepare datastore
			testFailure := &model.TestFailure{
				ID:          101,
				Project:     "testproject",
				TestID:      "testID",
				VariantHash: "othertestVariantHash",
				RefHash:     "testRefHash",
			}
			assert.Loosely(t, datastore.Put(c, testFailure), should.BeNil)

			datastore.GetTestable(c).CatchupIndexes()

			testFailures, err := GetTestFailures(c, "testproject", "testID", "testRefHash", "")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, testFailures, should.HaveLength(1))
			assert.Loosely(t, testFailures[0].ID, should.Equal(101))
		})
	})

	ftt.Run("Get test failures by test variant", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())

		t.Run("No test failure found", func(t *ftt.Test) {
			testFailures, err := GetTestFailures(c, "testproject", "testID", "testRefHash", "testVariantHash")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, testFailures, should.HaveLength(0))
		})

		t.Run("Test failure found", func(t *ftt.Test) {
			// Prepare datastore
			testFailure := &model.TestFailure{
				ID:          102,
				Project:     "testproject",
				TestID:      "testID",
				VariantHash: "testVariantHash",
				RefHash:     "testRefHash",
			}
			assert.Loosely(t, datastore.Put(c, testFailure), should.BeNil)

			datastore.GetTestable(c).CatchupIndexes()

			testFailures, err := GetTestFailures(c, "testproject", "testID", "testRefHash", "testVariantHash")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, testFailures, should.HaveLength(1))
			assert.Loosely(t, testFailures[0].ID, should.Equal(102))
		})
	})
}
