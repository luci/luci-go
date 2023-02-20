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

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/analysis/internal/testutil"
	"go.chromium.org/luci/server/span"
)

func TestClearRuleUsers(t *testing.T) {
	Convey(`With Spanner Test Database`, t, func() {

		ctx := testutil.IntegrationTestContext(t)

		Convey(`Rules older than 30 days should have their CreationUser cleared`, func() {
			expectedRule := NewRule(101).
				WithCreationTime(time.Now().AddDate(0, 0, -30)).
				WithCreationUser("user@example.com").
				Build()
			err := SetRulesForTesting(ctx, []*FailureAssociationRule{expectedRule})
			So(err, ShouldBeNil)

			rows, err := clearCreationUserColumn(ctx)
			So(err, ShouldBeNil)
			So(rows, ShouldEqual, 1)

			rule, err := Read(span.Single(ctx), testProject, expectedRule.RuleID)
			So(err, ShouldBeNil)
			So(rule.CreationUser, ShouldEqual, "")
		})

		Convey(`Rules created less than 30 days ago should not change`, func() {
			expectedRule := NewRule(101).
				WithCreationTime(time.Now()).
				WithCreationUser("user@example.com").
				Build()
			err := SetRulesForTesting(ctx, []*FailureAssociationRule{expectedRule})
			So(err, ShouldBeNil)

			rows, err := clearCreationUserColumn(ctx)
			So(err, ShouldBeNil)
			So(rows, ShouldEqual, 0)
		})

		Convey(`Rules updated more than 30 days ago have their LastUpdateUser cleared`, func() {
			expectedRule := NewRule(101).
				WithLastUpdated(time.Now().AddDate(0, 0, -30)).
				WithLastUpdatedUser("user@example.com").
				Build()
			err := SetRulesForTesting(ctx, []*FailureAssociationRule{expectedRule})
			So(err, ShouldBeNil)

			rows, err := clearLastUpdatedUserColumn(ctx)
			So(err, ShouldBeNil)
			So(rows, ShouldEqual, 1)

			rule, err := Read(span.Single(ctx), testProject, expectedRule.RuleID)
			So(err, ShouldBeNil)
			So(rule.LastUpdatedUser, ShouldEqual, "")
		})

		Convey(`Rules updated less than 30 days ago should not change`, func() {
			expectedRule := NewRule(101).
				WithLastUpdated(time.Now()).
				WithLastUpdatedUser("user@example.com").
				Build()
			err := SetRulesForTesting(ctx, []*FailureAssociationRule{expectedRule})
			So(err, ShouldBeNil)

			rows, err := clearLastUpdatedUserColumn(ctx)
			So(err, ShouldBeNil)
			So(rows, ShouldEqual, 0)
		})

		Convey(`ClearRulesUsers clears both LastUpdatedUser and CreationUser`, func() {
			expectedRule := NewRule(101).
				WithCreationTime(time.Now().AddDate(0, 0, -30)).
				WithCreationUser("user@example.com").
				WithLastUpdated(time.Now().AddDate(0, 0, -30)).
				WithLastUpdatedUser("user@example.com").
				Build()
			err := SetRulesForTesting(ctx, []*FailureAssociationRule{expectedRule})
			So(err, ShouldBeNil)

			err = ClearRulesUsers(ctx)
			So(err, ShouldBeNil)

			rule, err := Read(span.Single(ctx), testProject, expectedRule.RuleID)
			So(err, ShouldBeNil)
			So(rule.CreationUser, ShouldEqual, "")
			So(rule.LastUpdatedUser, ShouldEqual, "")
		})
	})
}
