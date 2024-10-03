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

package rules

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testutil"
)

func TestClearRuleUsers(t *testing.T) {
	ftt.Run(`With Spanner Test Database`, t, func(t *ftt.Test) {

		ctx := testutil.IntegrationTestContext(t)

		t.Run(`Rules older than 30 days should have their CreationUser cleared`, func(t *ftt.Test) {
			expectedRule := NewRule(101).
				WithCreateTime(time.Now().AddDate(0, 0, -31)).
				WithCreateUser("user@example.com").
				Build()
			err := SetForTesting(ctx, t, []*Entry{expectedRule})
			assert.Loosely(t, err, should.BeNil)

			rows, err := clearCreationUserColumn(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rows, should.Equal(1))

			rule, err := Read(span.Single(ctx), testProject, expectedRule.RuleID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rule.CreateUser, should.BeEmpty)
		})

		t.Run(`Rules created less than 30 days ago should not change`, func(t *ftt.Test) {
			expectedRule := NewRule(101).
				WithCreateTime(time.Now()).
				WithCreateUser("user@example.com").
				Build()
			err := SetForTesting(ctx, t, []*Entry{expectedRule})
			assert.Loosely(t, err, should.BeNil)

			rows, err := clearCreationUserColumn(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rows, should.BeZero)
		})

		t.Run(`Rules with auditable updates more than 30 days ago have their LastAuditableUpdateUser cleared`, func(t *ftt.Test) {
			expectedRule := NewRule(101).
				WithLastAuditableUpdateTime(time.Now().AddDate(0, 0, -31)).
				WithLastAuditableUpdateUser("user@example.com").
				Build()
			err := SetForTesting(ctx, t, []*Entry{expectedRule})
			assert.Loosely(t, err, should.BeNil)

			rows, err := clearLastUpdatedUserColumn(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rows, should.Equal(1))

			rule, err := Read(span.Single(ctx), testProject, expectedRule.RuleID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rule.LastAuditableUpdateUser, should.BeEmpty)
		})

		t.Run(`Rules with auditable updates less than 30 days ago should not change`, func(t *ftt.Test) {
			expectedRule := NewRule(101).
				WithLastAuditableUpdateTime(time.Now().AddDate(0, 0, -29)).
				WithLastAuditableUpdateUser("user@example.com").
				Build()
			err := SetForTesting(ctx, t, []*Entry{expectedRule})
			assert.Loosely(t, err, should.BeNil)

			rows, err := clearLastUpdatedUserColumn(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rows, should.BeZero)
		})

		t.Run(`ClearRulesUsers clears both LastAuditableUpdateUser and CreationUser`, func(t *ftt.Test) {
			expectedRule := NewRule(101).
				WithCreateTime(time.Now().AddDate(0, 0, -31)).
				WithCreateUser("user@example.com").
				WithLastAuditableUpdateTime(time.Now().AddDate(0, 0, -31)).
				WithLastAuditableUpdateUser("user@example.com").
				Build()
			err := SetForTesting(ctx, t, []*Entry{expectedRule})
			assert.Loosely(t, err, should.BeNil)

			err = ClearRulesUsers(ctx)
			assert.Loosely(t, err, should.BeNil)

			rule, err := Read(span.Single(ctx), testProject, expectedRule.RuleID)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, rule.CreateUser, should.BeEmpty)
			assert.Loosely(t, rule.LastAuditableUpdateUser, should.BeEmpty)
		})
	})
}
