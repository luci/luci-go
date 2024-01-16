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

package migration

import (
	"sort"
	"testing"
	"time"

	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
	"go.chromium.org/luci/server/span"

	"go.chromium.org/luci/analysis/internal/bugs"
	"go.chromium.org/luci/analysis/internal/bugs/monorail"
	"go.chromium.org/luci/analysis/internal/bugs/monorail/api_proto"
	migrationpb "go.chromium.org/luci/analysis/internal/bugs/monorail/migration/proto"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/testutil"
	configpb "go.chromium.org/luci/analysis/proto/config"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestMigration(t *testing.T) {
	Convey("With Server", t, func() {
		ctx := testutil.IntegrationTestContext(t)

		// For user identification.
		ctx = authtest.MockAuthConfig(ctx)
		authState := &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{luciAnalysisAdminGroup},
		}
		ctx = auth.WithState(ctx, authState)
		ctx = secrets.Use(ctx, &testsecrets.Store{})

		// Service config required by implementation.
		ctx = memory.Use(ctx)
		err := config.SetTestConfig(ctx, &configpb.Config{})
		So(err, ShouldBeNil)

		// Set up some rules for testing.
		ruleMonorail := rules.NewRule(0).
			WithProject("testproject").
			WithBug(bugs.BugID{System: "monorail", ID: "monorailproject/111"}).Build()
		ruleMonorail2 := rules.NewRule(1).
			WithProject("testproject").
			WithBug(bugs.BugID{System: "monorail", ID: "monorailproject/222"}).Build()
		ruleBuganizer := rules.NewRule(2).
			WithProject("testproject").
			WithBug(bugs.BugID{System: "buganizer", ID: "666"}).
			Build()
		ruleOtherProject := rules.NewRule(3).
			WithProject("anotherproject").
			WithBug(bugs.BugID{System: "monorail", ID: "aproject/111"}).
			Build()

		err = rules.SetForTesting(ctx, []*rules.Entry{
			ruleMonorail,
			ruleMonorail2,
			ruleBuganizer,
			ruleOtherProject,
		})
		So(err, ShouldBeNil)

		f := &monorail.FakeIssuesStore{
			NextID:            100,
			PriorityFieldName: "projects/chromium/fieldDefs/11",
			ComponentNames: []string{
				"projects/chromium/componentDefs/Blink",
			},
			Issues: []*monorail.IssueData{
				{
					Issue: &api_proto.Issue{
						Name:       "projects/monorailproject/issues/111",
						MigratedId: "8111",
					},
				},
				{
					Issue: &api_proto.Issue{
						Name:       "projects/monorailproject/issues/222",
						MigratedId: "8222",
					},
				},
			},
		}

		ctx = monorail.UseFakeIssuesClient(ctx, f, "service-user")

		srv := NewMonorailMigrationServer()

		Convey("Unauthorised requests are rejected", func() {
			// Ensure no access to luci-analysis-access.
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				// Not a member of service-luci-analysis-admins.
				IdentityGroups: []string{"other-group"},
			})

			// Make some request (the request should not matter, as
			// a common decorator is used for all requests.)
			request := &migrationpb.MigrateProjectRequest{
				Project:  "testproject",
				MaxRules: 1,
			}

			rsp, err := srv.MigrateProject(ctx, request)
			So(err, ShouldBeRPCPermissionDenied, "not a member of service-luci-analysis-admins")
			So(rsp, ShouldBeNil)
		})
		Convey("MigrateProject", func() {
			request := &migrationpb.MigrateProjectRequest{
				Project:  "testproject",
				MaxRules: 1,
			}
			expectedRules := []*rules.Entry{
				ruleMonorail,
				ruleMonorail2,
				ruleBuganizer,
				ruleOtherProject,
			}

			Convey("Migrated-to bug mapping available", func() {
				rsp, err := srv.MigrateProject(ctx, request)
				So(err, ShouldBeNil)
				So(rsp, ShouldResembleProto, &migrationpb.MigrateProjectResponse{
					RulesNotStarted: 0,
					RuleResults: []*migrationpb.MigrateProjectResponse_RuleResult{
						{
							RuleId:         ruleMonorail.RuleID,
							MonorailBugId:  "monorailproject/111",
							BuganizerBugId: "8111",
						},
					},
				})

				// Rule should be updated to use buganizer bug.
				expectedRules[0].BugID = bugs.BugID{
					System: "buganizer",
					ID:     "8111",
				}
				expectedRules[0].LastAuditableUpdateUser = "system"

				rs, err := rules.ReadAllForTesting(span.Single(ctx))
				So(err, ShouldBeNil)
				So(sortRulesAndClearTimestamps(rs), ShouldResembleProto, sortRulesAndClearTimestamps(expectedRules))
			})
			Convey("Migrated to bug mapping unavailable", func() {
				for _, issue := range f.Issues {
					issue.Issue.MigratedId = ""
				}

				rsp, err := srv.MigrateProject(ctx, request)
				So(err, ShouldBeNil)
				So(rsp, ShouldResembleProto, &migrationpb.MigrateProjectResponse{
					RulesNotStarted: 0,
					RuleResults: []*migrationpb.MigrateProjectResponse_RuleResult{
						{
							RuleId:         ruleMonorail.RuleID,
							MonorailBugId:  "monorailproject/111",
							BuganizerBugId: "",
							Error:          "monorail did not return the Buganizer bug this bug was migrated to",
						},
					},
				})

				rs, err := rules.ReadAllForTesting(span.Single(ctx))
				So(err, ShouldBeNil)
				So(sortRulesAndClearTimestamps(rs), ShouldResembleProto, sortRulesAndClearTimestamps(expectedRules))
			})
		})
	})
}

func sortRulesAndClearTimestamps(rs []*rules.Entry) []*rules.Entry {
	rsCopy := make([]*rules.Entry, len(rs))
	copy(rsCopy, rs)

	for _, r := range rsCopy {
		r.LastUpdateTime = time.Time{}
		r.LastAuditableUpdateTime = time.Time{}
	}

	sort.Slice(rsCopy, func(i, j int) bool {
		return rsCopy[i].RuleID < rsCopy[j].RuleID
	})
	return rsCopy
}
