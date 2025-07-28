// Copyright 2024 The LUCI Authors.
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

package status

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/tree_status/internal/testutil"
)

func TestClearStatusUsers(t *testing.T) {
	ftt.Run(`With Spanner Test Database`, t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)

		t.Run(`Status older than 30 days should have their CreateUser cleared`, func(t *ftt.Test) {
			NewStatusBuilder().
				WithCreateTime(time.Now().AddDate(0, 0, -31)).
				WithCreateUser("user@example.com").
				CreateInDB(ctx)

			rows, err := clearCreateUserColumn(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rows, should.Equal(1))

			status, err := ReadLatest(span.Single(ctx), "chromium")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, status.CreateUser, should.BeEmpty)
		})

		t.Run(`Status created less than 30 days ago should not change`, func(t *ftt.Test) {
			NewStatusBuilder().
				WithCreateTime(time.Now()).
				WithCreateUser("user@example.com").
				CreateInDB(ctx)

			rows, err := clearCreateUserColumn(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rows, should.BeZero)

			status, err := ReadLatest(span.Single(ctx), "chromium")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, status.CreateUser, should.Equal("user@example.com"))
		})

		t.Run(`ClearStatusUsers clears CreateUser`, func(t *ftt.Test) {
			NewStatusBuilder().
				WithCreateTime(time.Now().AddDate(0, 0, -31)).
				WithCreateUser("user@example.com").
				CreateInDB(ctx)

			err := ClearStatusUsers(ctx)
			assert.Loosely(t, err, should.BeNil)

			status, err := ReadLatest(span.Single(ctx), "chromium")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, status.CreateUser, should.BeEmpty)
		})
	})
}
