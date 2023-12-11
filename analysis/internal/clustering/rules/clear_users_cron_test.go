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

	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/testutil"

	. "github.com/smartystreets/goconvey/convey"
)

func TestClearRuleUsers(t *testing.T) {
	Convey(`With Spanner Test Database`, t, func() {

		ctx := testutil.IntegrationTestContext(t)

		Convey(`Rules older than 30 days should have their CreationUser cleared`, func() {
			expectedRule := NewRule(101).
				WithCreateTime(time.Now().AddDate(0, 0, -31)).
				WithCreateUser("user@example.com").
				Build()
			err := SetForTesting(ctx, []*Entry{expectedRule})
			So(err, ShouldBeNil)

			rows, err := clearCreationUserColumn(ctx)
			So(err, ShouldBeNil)
			So(rows, ShouldEqual, 1)

			rule, err := Read(span.Single(ctx), testProject, expectedRule.RuleID)
			So(err, ShouldBeNil)
			So(rule.CreateUser, ShouldEqual, "")
		})

		Convey(`Rules created less than 30 days ago should not change`, func() {
			expectedRule := NewRule(101).
				WithCreateTime(time.Now()).
				WithCreateUser("user@example.com").
				Build()
			err := SetForTesting(ctx, []*Entry{expectedRule})
			So(err, ShouldBeNil)

			rows, err := clearCreationUserColumn(ctx)
			So(err, ShouldBeNil)
			So(rows, ShouldEqual, 0)
		})

		Convey(`Rules with auditable updates more than 30 days ago have their LastAuditableUpdateUser cleared`, func() {
			expectedRule := NewRule(101).
				WithLastAuditableUpdateTime(time.Now().AddDate(0, 0, -31)).
				WithLastAuditableUpdateUser("user@example.com").
				Build()
			err := SetForTesting(ctx, []*Entry{expectedRule})
			So(err, ShouldBeNil)

			rows, err := clearLastUpdatedUserColumn(ctx)
			So(err, ShouldBeNil)
			So(rows, ShouldEqual, 1)

			rule, err := Read(span.Single(ctx), testProject, expectedRule.RuleID)
			So(err, ShouldBeNil)
			So(rule.LastAuditableUpdateUser, ShouldEqual, "")
		})

		Convey(`Rules with auditable updates less than 30 days ago should not change`, func() {
			expectedRule := NewRule(101).
				WithLastAuditableUpdateTime(time.Now().AddDate(0, 0, -29)).
				WithLastAuditableUpdateUser("user@example.com").
				Build()
			err := SetForTesting(ctx, []*Entry{expectedRule})
			So(err, ShouldBeNil)

			rows, err := clearLastUpdatedUserColumn(ctx)
			So(err, ShouldBeNil)
			So(rows, ShouldEqual, 0)
		})

		Convey(`ClearRulesUsers clears both LastAuditableUpdateUser and CreationUser`, func() {
			expectedRule := NewRule(101).
				WithCreateTime(time.Now().AddDate(0, 0, -31)).
				WithCreateUser("user@example.com").
				WithLastAuditableUpdateTime(time.Now().AddDate(0, 0, -31)).
				WithLastAuditableUpdateUser("user@example.com").
				Build()
			err := SetForTesting(ctx, []*Entry{expectedRule})
			So(err, ShouldBeNil)

			err = ClearRulesUsers(ctx)
			So(err, ShouldBeNil)

			rule, err := Read(span.Single(ctx), testProject, expectedRule.RuleID)
			So(err, ShouldBeNil)
			So(rule.CreateUser, ShouldEqual, "")
			So(rule.LastAuditableUpdateUser, ShouldEqual, "")
		})
	})
}
