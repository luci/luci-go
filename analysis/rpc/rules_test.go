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

package rpc

import (
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
	"go.chromium.org/luci/server/span"
	"go.chromium.org/luci/third_party/google.golang.org/genproto/googleapis/devtools/issuetracker/v1"

	"go.chromium.org/luci/analysis/internal/bugs"
	"go.chromium.org/luci/analysis/internal/bugs/buganizer"
	bugspb "go.chromium.org/luci/analysis/internal/bugs/proto"
	"go.chromium.org/luci/analysis/internal/clustering"
	"go.chromium.org/luci/analysis/internal/clustering/algorithms/testname"
	"go.chromium.org/luci/analysis/internal/clustering/rules"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/perms"
	"go.chromium.org/luci/analysis/internal/testutil"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"
)

func TestRules(t *testing.T) {
	ftt.Run("With Server", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)

		// For user identification.
		ctx = authtest.MockAuthConfig(ctx)
		authState := &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{luciAnalysisAccessGroup, auditUsersAccessGroup},
		}
		ctx = auth.WithState(ctx, authState)
		ctx = secrets.Use(ctx, &testsecrets.Store{})

		// Provides datastore implementation needed for project config.
		ctx = memory.Use(ctx)

		uiBaseURL := "https://analysis.luci.app"
		buganizerClient := buganizer.NewFakeClient()
		srv := NewRulesServer(uiBaseURL, buganizerClient, "service@project.iam.gserviceaccount.com")

		ruleManagedBuilder := rules.NewRule(0).
			WithProject(testProject).
			WithBug(bugs.BugID{System: "buganizer", ID: "111"})
		ruleManaged := ruleManagedBuilder.Build()
		ruleTwoProject := rules.NewRule(1).
			WithProject(testProject).
			WithBug(bugs.BugID{System: "buganizer", ID: "222"}).
			WithBugManaged(false).
			Build()
		ruleTwoProjectOther := rules.NewRule(2).
			WithProject("otherproject").
			WithBug(bugs.BugID{System: "buganizer", ID: "222"}).
			Build()
		ruleUnmanagedOther := rules.NewRule(3).
			WithProject("otherproject").
			WithBug(bugs.BugID{System: "buganizer", ID: "444"}).
			WithBugManaged(false).
			Build()
		ruleManagedOther := rules.NewRule(4).
			WithProject("otherproject").
			WithBug(bugs.BugID{System: "buganizer", ID: "555"}).
			WithBugManaged(true).
			Build()
		ruleBuganizer := rules.NewRule(5).
			WithProject(testProject).
			WithBug(bugs.BugID{System: "buganizer", ID: "666"}).
			Build()

		err := rules.SetForTesting(ctx, t, []*rules.Entry{
			ruleManaged,
			ruleTwoProject,
			ruleTwoProjectOther,
			ruleUnmanagedOther,
			ruleManagedOther,
			ruleBuganizer,
		})
		assert.Loosely(t, err, should.BeNil)

		cfg := &configpb.ProjectConfig{}
		err = config.SetTestProjectConfig(ctx, map[string]*configpb.ProjectConfig{
			"testproject": cfg,
		})
		assert.Loosely(t, err, should.BeNil)

		t.Run("Unauthorised requests are rejected", func(t *ftt.Test) {
			// Ensure no access to luci-analysis-access.
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				// Not a member of luci-analysis-access.
				IdentityGroups: []string{"other-group"},
			})

			// Make some request (the request should not matter, as
			// a common decorator is used for all requests.)
			request := &pb.GetRuleRequest{
				Name: fmt.Sprintf("projects/%s/rules/%s", ruleManaged.Project, ruleManaged.RuleID),
			}

			rule, err := srv.Get(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
			assert.Loosely(t, err, should.ErrLike("not a member of luci-analysis-access"))
			assert.Loosely(t, rule, should.BeNil)
		})
		t.Run("Get", func(t *ftt.Test) {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "testproject:@project",
					Permission: perms.PermGetRule,
				},
				{
					Realm:      "testproject:@project",
					Permission: perms.PermGetRuleDefinition,
				},
			}

			t.Run("No get rule permission", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetRule)

				request := &pb.GetRuleRequest{
					Name: fmt.Sprintf("projects/%s/rules/%s", ruleManaged.Project, ruleManaged.RuleID),
				}

				rule, err := srv.Get(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("caller does not have permission analysis.rules.get"))
				assert.Loosely(t, rule, should.BeNil)
			})
			t.Run("Rule exists", func(t *ftt.Test) {
				mask := ruleMask{
					IncludeDefinition: true,
					IncludeAuditUsers: true,
				}
				t.Run("Baseline", func(t *ftt.Test) {
					request := &pb.GetRuleRequest{
						Name: fmt.Sprintf("projects/%s/rules/%s", ruleManaged.Project, ruleManaged.RuleID),
					}

					rule, err := srv.Get(ctx, request)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, rule, should.Match(createRulePB(ruleManaged, cfg, mask)))
				})
				t.Run("Without get rule definition permission", func(t *ftt.Test) {
					authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetRuleDefinition)

					request := &pb.GetRuleRequest{
						Name: fmt.Sprintf("projects/%s/rules/%s", ruleManaged.Project, ruleManaged.RuleID),
					}

					rule, err := srv.Get(ctx, request)
					assert.Loosely(t, err, should.BeNil)
					mask.IncludeDefinition = false
					assert.Loosely(t, rule, should.Match(createRulePB(ruleManaged, cfg, mask)))
				})
				t.Run("Without get rule audit users permission", func(t *ftt.Test) {
					authState.IdentityGroups = removeGroup(authState.IdentityGroups, auditUsersAccessGroup)

					request := &pb.GetRuleRequest{
						Name: fmt.Sprintf("projects/%s/rules/%s", ruleManaged.Project, ruleManaged.RuleID),
					}

					rule, err := srv.Get(ctx, request)
					assert.Loosely(t, err, should.BeNil)
					mask.IncludeAuditUsers = false
					assert.Loosely(t, rule, should.Match(createRulePB(ruleManaged, cfg, mask)))
				})
			})
			t.Run("Rule does not exist", func(t *ftt.Test) {
				ruleID := strings.Repeat("00", 16)
				request := &pb.GetRuleRequest{
					Name: fmt.Sprintf("projects/%s/rules/%s", ruleManaged.Project, ruleID),
				}

				rule, err := srv.Get(ctx, request)
				assert.Loosely(t, rule, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			})
		})
		t.Run("List", func(t *ftt.Test) {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "testproject:@project",
					Permission: perms.PermListRules,
				},
				{
					Realm:      "testproject:@project",
					Permission: perms.PermGetRuleDefinition,
				},
			}

			request := &pb.ListRulesRequest{
				Parent: fmt.Sprintf("projects/%s", testProject),
			}
			t.Run("No list rules permission", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermListRules)

				response, err := srv.List(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("caller does not have permission analysis.rules.list"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("Non-Empty", func(t *ftt.Test) {
				test := func(mask ruleMask, cfg *configpb.ProjectConfig) {
					rs := []*rules.Entry{
						ruleManaged,
						ruleBuganizer,
						rules.NewRule(2).WithProject(testProject).Build(),
						rules.NewRule(3).WithProject(testProject).Build(),
						rules.NewRule(4).WithProject(testProject).Build(),
						// In other project.
						ruleManagedOther,
					}
					err := rules.SetForTesting(ctx, t, rs)
					assert.Loosely(t, err, should.BeNil)

					response, err := srv.List(ctx, request)
					assert.Loosely(t, err, should.BeNil)

					expected := &pb.ListRulesResponse{
						Rules: []*pb.Rule{
							createRulePB(rs[0], cfg, mask),
							createRulePB(rs[1], cfg, mask),
							createRulePB(rs[2], cfg, mask),
							createRulePB(rs[3], cfg, mask),
							createRulePB(rs[4], cfg, mask),
						},
					}
					sort.Slice(expected.Rules, func(i, j int) bool {
						return expected.Rules[i].RuleId < expected.Rules[j].RuleId
					})
					sort.Slice(response.Rules, func(i, j int) bool {
						return response.Rules[i].RuleId < response.Rules[j].RuleId
					})
					assert.Loosely(t, response, should.Match(expected))
				}
				mask := ruleMask{
					IncludeDefinition: true,
					IncludeAuditUsers: true,
				}
				t.Run("Baseline", func(t *ftt.Test) {
					test(mask, cfg)
				})
				t.Run("Without get rule definition permission", func(t *ftt.Test) {
					authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetRuleDefinition)

					mask.IncludeDefinition = false
					test(mask, cfg)
				})
				t.Run("Without get rule audit users permission", func(t *ftt.Test) {
					authState.IdentityGroups = removeGroup(authState.IdentityGroups, auditUsersAccessGroup)

					mask.IncludeAuditUsers = false
					test(mask, cfg)
				})
				t.Run("With project not configured", func(t *ftt.Test) {
					err = config.SetTestProjectConfig(ctx, map[string]*configpb.ProjectConfig{})

					test(mask, config.NewEmptyProject())
				})
			})
			t.Run("Empty", func(t *ftt.Test) {
				err := rules.SetForTesting(ctx, t, nil)
				assert.Loosely(t, err, should.BeNil)

				response, err := srv.List(ctx, request)
				assert.Loosely(t, err, should.BeNil)

				expected := &pb.ListRulesResponse{}
				assert.Loosely(t, response, should.Match(expected))
			})
		})
		t.Run("Update", func(t *ftt.Test) {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "testproject:@project",
					Permission: perms.PermUpdateRule,
				},
				{
					Realm:      "testproject:@project",
					Permission: perms.PermGetRuleDefinition,
				},
			}
			request := &pb.UpdateRuleRequest{
				Rule: &pb.Rule{
					Name:           fmt.Sprintf("projects/%s/rules/%s", ruleManaged.Project, ruleManaged.RuleID),
					RuleDefinition: `test = "updated"`,
					Bug: &pb.AssociatedBug{
						System: "buganizer",
						Id:     "2",
					},
					IsManagingBug:         false,
					IsManagingBugPriority: false,
					IsActive:              false,
				},
				UpdateMask: &fieldmaskpb.FieldMask{
					// On the client side, we use JSON equivalents, i.e. ruleDefinition,
					// bug, isActive, isManagingBug. The pRPC layer handles the
					// translation before the request hits our service.
					Paths: []string{"rule_definition", "bug", "is_active", "is_managing_bug", "is_managing_bug_priority"},
				},
				Etag: ruleETag(ruleManaged, ruleMask{IncludeDefinition: true, IncludeAuditUsers: false}),
			}

			t.Run("No update rules permission", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermUpdateRule)

				rule, err := srv.Update(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("caller does not have permission analysis.rules.update"))
				assert.Loosely(t, rule, should.BeNil)
			})
			t.Run("No rule definition permission", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetRuleDefinition)

				rule, err := srv.Update(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("caller does not have permission analysis.rules.getDefinition"))
				assert.Loosely(t, rule, should.BeNil)
			})
			t.Run("Validation error", func(t *ftt.Test) {
				t.Run("Rule unspecified", func(t *ftt.Test) {
					request.Rule = nil

					_, err := srv.Update(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("rule: name: invalid rule name"))
				})
				t.Run("Empty bug", func(t *ftt.Test) {
					request.Rule.Bug = nil

					_, err := srv.Update(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("rule: bug: unspecified"))
				})
				t.Run("Invalid bug system", func(t *ftt.Test) {
					request.Rule.Bug.System = "other"

					_, err := srv.Update(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("rule: bug: invalid bug tracking system \"other\""))
				})
				t.Run("Invalid bug system - monorail", func(t *ftt.Test) {
					request.Rule.Bug.System = "monorail"
					request.Rule.Bug.Id = "project/2"

					_, err := srv.Update(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("rule: bug: system: monorail bug system is no longer supported"))
				})
				t.Run("Invalid buganizer bug id", func(t *ftt.Test) {
					request.Rule.Bug.System = "buganizer"
					request.Rule.Bug.Id = "-12345"

					_, err := srv.Update(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("rule: bug: invalid buganizer bug ID \"-12345\""))
				})
				t.Run("Re-use of same bug in same project", func(t *ftt.Test) {
					// Use the same bug as another rule.
					request.Rule.Bug = &pb.AssociatedBug{
						System: ruleTwoProject.BugID.System,
						Id:     ruleTwoProject.BugID.ID,
					}

					_, err := srv.Update(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
					assert.Loosely(t, err, should.ErrLike(fmt.Sprintf("bug already used by a rule in the same project (%s/%s)", ruleTwoProject.Project, ruleTwoProject.RuleID)))
				})
				t.Run("Bug managed by another rule", func(t *ftt.Test) {
					// Select a bug already managed by another rule.
					request.Rule.Bug = &pb.AssociatedBug{
						System: ruleManagedOther.BugID.System,
						Id:     ruleManagedOther.BugID.ID,
					}
					// Request we manage this bug.
					request.Rule.IsManagingBug = true
					request.UpdateMask.Paths = []string{"bug", "is_managing_bug"}

					_, err := srv.Update(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
					assert.Loosely(t, err, should.ErrLike(fmt.Sprintf("rule: bug: bug already managed by a rule in another project (%s/%s)", ruleManagedOther.Project, ruleManagedOther.RuleID)))
				})
				t.Run("Empty rule definition", func(t *ftt.Test) {
					// Use an invalid failure association rule.
					request.Rule.RuleDefinition = ""

					_, err := srv.Update(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`rule: rule_definition: unspecified`))
				})
				t.Run("Invalid rule definition", func(t *ftt.Test) {
					// Use an invalid failure association rule.
					request.Rule.RuleDefinition = "<"

					_, err := srv.Update(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`rule: rule_definition: parse: syntax error: 1:1: invalid input text "<"`))
				})
			})
			t.Run("Success", func(t *ftt.Test) {
				t.Run("Predicate updated", func(t *ftt.Test) {
					// The requested updates should be applied.
					expectedRule := ruleManagedBuilder.Build().Clone()
					expectedRule.RuleDefinition = `test = "updated"`
					expectedRule.BugID = bugs.BugID{System: "buganizer", ID: "2"}
					expectedRule.IsActive = false
					expectedRule.IsManagingBug = false
					expectedRule.IsManagingBugPriority = false

					// The notification flags should be reset as the bug was changed.
					expectedRule.BugManagementState.RuleAssociationNotified = false
					for _, policyState := range expectedRule.BugManagementState.PolicyState {
						policyState.ActivationNotified = false
					}

					t.Run("baseline", func(t *ftt.Test) {
						// Act
						rule, err := srv.Update(ctx, request)

						// Verify
						assert.Loosely(t, err, should.BeNil)

						storedRule, err := rules.Read(span.Single(ctx), testProject, ruleManaged.RuleID)
						assert.Loosely(t, err, should.BeNil)
						assert.That(t, storedRule.LastUpdateTime, should.NotMatch(ruleManaged.LastUpdateTime))

						// Accept the new last update time (this value is set non-deterministically).
						expectedRule.LastUpdateTime = storedRule.LastUpdateTime
						expectedRule.LastAuditableUpdateTime = storedRule.LastUpdateTime
						expectedRule.IsManagingBugPriorityLastUpdateTime = storedRule.LastUpdateTime
						expectedRule.PredicateLastUpdateTime = storedRule.LastUpdateTime
						expectedRule.LastAuditableUpdateUser = "someone@example.com"

						// Verify the rule was updated as expected.
						assert.Loosely(t, storedRule, should.Match(expectedRule))

						// Verify the returned rule matches what was expected.
						mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: true}
						assert.Loosely(t, rule, should.Match(createRulePB(expectedRule, cfg, mask)))
					})
					t.Run("No audit users permission", func(t *ftt.Test) {
						authState.IdentityGroups = removeGroup(authState.IdentityGroups, auditUsersAccessGroup)

						// Act
						rule, err := srv.Update(ctx, request)

						// Verify
						assert.Loosely(t, err, should.BeNil)

						storedRule, err := rules.Read(span.Single(ctx), testProject, ruleManaged.RuleID)
						assert.Loosely(t, err, should.BeNil)
						assert.That(t, storedRule.LastUpdateTime, should.NotMatch(ruleManaged.LastUpdateTime))

						// Accept the new last update time (this value is set non-deterministically).
						expectedRule.LastUpdateTime = storedRule.LastUpdateTime
						expectedRule.LastAuditableUpdateTime = storedRule.LastUpdateTime
						expectedRule.PredicateLastUpdateTime = storedRule.LastUpdateTime
						expectedRule.IsManagingBugPriorityLastUpdateTime = storedRule.LastUpdateTime
						expectedRule.LastAuditableUpdateUser = "someone@example.com"

						assert.Loosely(t, storedRule, should.Match(expectedRule))

						// Verify audit users are omitted from the result.
						mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: false}
						assert.Loosely(t, rule, should.Match(createRulePB(expectedRule, cfg, mask)))
					})
				})
				t.Run("Predicate not updated", func(t *ftt.Test) {
					request.UpdateMask.Paths = []string{"bug"}
					request.Rule.Bug = &pb.AssociatedBug{
						System: "buganizer",
						Id:     "99999999",
					}

					rule, err := srv.Update(ctx, request)
					assert.Loosely(t, err, should.BeNil)

					storedRule, err := rules.Read(span.Single(ctx), testProject, ruleManaged.RuleID)
					assert.Loosely(t, err, should.BeNil)

					// Check the rule was updated, but that predicate last
					// updated time was NOT updated.
					assert.That(t, storedRule.LastUpdateTime, should.NotMatch(ruleManaged.LastUpdateTime))

					// Verify the rule was correctly updated in the database:
					// - The requested updates should be applied.
					expectedRule := ruleManagedBuilder.Build()
					expectedRule.BugID = bugs.BugID{System: "buganizer", ID: "99999999"}

					// - The notification flags should be reset as the bug was changed.
					expectedRule.BugManagementState.RuleAssociationNotified = false
					for _, policyState := range expectedRule.BugManagementState.PolicyState {
						policyState.ActivationNotified = false
					}

					// - Accept the new last update time (this value is set non-deterministically).
					expectedRule.LastUpdateTime = storedRule.LastUpdateTime
					expectedRule.LastAuditableUpdateTime = storedRule.LastUpdateTime
					expectedRule.LastAuditableUpdateUser = "someone@example.com"

					assert.Loosely(t, storedRule, should.Match(expectedRule))

					// Verify the returned rule matches what was expected.
					mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: true}
					assert.Loosely(t, rule, should.Match(createRulePB(expectedRule, cfg, mask)))
				})
				t.Run("Managing bug priority updated", func(t *ftt.Test) {
					request.UpdateMask.Paths = []string{"is_managing_bug_priority"}
					request.Rule.IsManagingBugPriority = false

					rule, err := srv.Update(ctx, request)
					assert.Loosely(t, err, should.BeNil)

					storedRule, err := rules.Read(span.Single(ctx), testProject, ruleManaged.RuleID)
					assert.Loosely(t, err, should.BeNil)

					// Check the rule was updated.
					assert.That(t, storedRule.LastUpdateTime, should.NotMatch(ruleManaged.LastUpdateTime))

					// Verify the rule was correctly updated in the database:
					// - The requested update should be applied.
					expectedRule := ruleManagedBuilder.Build()
					expectedRule.IsManagingBugPriority = false

					// - Accept the new last update time (this value is set non-deterministically).
					expectedRule.IsManagingBugPriorityLastUpdateTime = storedRule.LastUpdateTime
					expectedRule.LastUpdateTime = storedRule.LastUpdateTime
					expectedRule.LastAuditableUpdateTime = storedRule.LastUpdateTime
					expectedRule.LastAuditableUpdateUser = "someone@example.com"

					assert.Loosely(t, storedRule, should.Match(expectedRule))

					// Verify the returned rule matches what was expected.
					mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: true}
					assert.Loosely(t, rule, should.Match(createRulePB(expectedRule, cfg, mask)))
				})
				t.Run("Re-use of bug managed by another project", func(t *ftt.Test) {
					request.UpdateMask.Paths = []string{"bug"}
					request.Rule.Bug = &pb.AssociatedBug{
						System: ruleManagedOther.BugID.System,
						Id:     ruleManagedOther.BugID.ID,
					}

					rule, err := srv.Update(ctx, request)
					assert.Loosely(t, err, should.BeNil)

					storedRule, err := rules.Read(span.Single(ctx), testProject, ruleManaged.RuleID)
					assert.Loosely(t, err, should.BeNil)

					// Check the rule was updated.
					assert.That(t, storedRule.LastUpdateTime, should.NotMatch(ruleManaged.LastUpdateTime))

					// Verify the rule was correctly updated in the database:
					// - The requested update should be applied.
					expectedRule := ruleManagedBuilder.Build()
					expectedRule.BugID = ruleManagedOther.BugID

					// - The notification flags should be reset as the bug was changed.
					expectedRule.BugManagementState.RuleAssociationNotified = false
					for _, policyState := range expectedRule.BugManagementState.PolicyState {
						policyState.ActivationNotified = false
					}

					// - IsManagingBug should be silently set to false, because ruleManagedOther
					//  already controls the bug.
					expectedRule.IsManagingBug = false

					// - Accept the new last update time (this value is set non-deterministically).
					expectedRule.LastUpdateTime = storedRule.LastUpdateTime
					expectedRule.LastAuditableUpdateTime = storedRule.LastUpdateTime
					expectedRule.LastAuditableUpdateUser = "someone@example.com"

					assert.Loosely(t, storedRule, should.Match(expectedRule))

					// Verify the returned rule matches what was expected.
					mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: true}
					assert.Loosely(t, rule, should.Match(createRulePB(expectedRule, cfg, mask)))
				})
			})
			t.Run("Concurrent Modification", func(t *ftt.Test) {
				_, err := srv.Update(ctx, request)
				assert.Loosely(t, err, should.BeNil)

				// Attempt the same modification again without
				// requerying.
				rule, err := srv.Update(ctx, request)
				assert.Loosely(t, rule, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.Aborted))
			})
			t.Run("Rule does not exist", func(t *ftt.Test) {
				ruleID := strings.Repeat("00", 16)
				request.Rule.Name = fmt.Sprintf("projects/%s/rules/%s", testProject, ruleID)

				rule, err := srv.Update(ctx, request)
				assert.Loosely(t, rule, should.BeNil)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			})
		})
		t.Run("Create", func(t *ftt.Test) {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "testproject:@project",
					Permission: perms.PermCreateRule,
				},
				{
					Realm:      "testproject:@project",
					Permission: perms.PermGetRuleDefinition,
				},
			}
			request := &pb.CreateRuleRequest{
				Parent: fmt.Sprintf("projects/%s", testProject),
				Rule: &pb.Rule{
					RuleDefinition: `test = "create"`,
					Bug: &pb.AssociatedBug{
						System: "buganizer",
						Id:     "2",
					},
					IsActive:              false,
					IsManagingBug:         true,
					IsManagingBugPriority: true,
					SourceCluster: &pb.ClusterId{
						Algorithm: testname.AlgorithmName,
						Id:        strings.Repeat("aa", 16),
					},
				},
			}

			t.Run("No create rule permission", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermCreateRule)

				rule, err := srv.Create(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("caller does not have permission analysis.rules.create"))
				assert.Loosely(t, rule, should.BeNil)
			})
			t.Run("No create rule definition permission", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetRuleDefinition)

				rule, err := srv.Create(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("caller does not have permission analysis.rules.getDefinition"))
				assert.Loosely(t, rule, should.BeNil)
			})
			t.Run("Validation error", func(t *ftt.Test) {
				t.Run("Rule unspecified", func(t *ftt.Test) {
					request.Rule = nil

					_, err := srv.Create(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("rule: unspecified"))
				})
				t.Run("Bug unspecified", func(t *ftt.Test) {
					request.Rule.Bug = nil

					_, err := srv.Create(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("rule: bug: unspecified"))
				})
				t.Run("Invalid buganizer bug", func(t *ftt.Test) {
					request.Rule.Bug.System = "buganizer"
					request.Rule.Bug.Id = "-2"

					_, err := srv.Create(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("rule: bug: invalid buganizer bug ID \"-2\""))
				})
				t.Run("Invalid bug system", func(t *ftt.Test) {
					request.Rule.Bug.System = "other"

					_, err := srv.Create(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("rule: bug: invalid bug tracking system \"other\""))
				})
				t.Run("Invalid bug system - monorail", func(t *ftt.Test) {
					request.Rule.Bug.System = "monorail"
					request.Rule.Bug.Id = "otherproject/2"

					_, err := srv.Create(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("rule: bug: system: monorail bug system is no longer supported"))
				})
				t.Run("Invalid source cluster", func(t *ftt.Test) {
					request.Rule.SourceCluster.Algorithm = "*invalid*"

					_, err := srv.Create(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("rule: source_cluster: algorithm not valid"))
				})
				t.Run("Re-use of same bug in same project", func(t *ftt.Test) {
					// Use the same bug as another rule, in the same project.
					request.Rule.Bug = &pb.AssociatedBug{
						System: ruleTwoProject.BugID.System,
						Id:     ruleTwoProject.BugID.ID,
					}

					_, err := srv.Create(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.FailedPrecondition))
					assert.Loosely(t, err, should.ErrLike(fmt.Sprintf("rule: bug: bug already used by a rule in the same project (%s/%s)", ruleTwoProject.Project, ruleTwoProject.RuleID)))
				})
				t.Run("Empty rule definition", func(t *ftt.Test) {
					request.Rule.RuleDefinition = ""

					_, err := srv.Create(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`rule: rule_definition: unspecified`))
				})
				t.Run("Invalid rule definition", func(t *ftt.Test) {
					// Use an invalid failure association rule.
					request.Rule.RuleDefinition = "<"

					_, err := srv.Create(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`rule: rule_definition: parse: syntax error: 1:1: invalid input text "<"`))
				})
			})
			t.Run("Success", func(t *ftt.Test) {
				expectedRuleBuilder := rules.NewRule(0).
					WithProject(testProject).
					WithRuleDefinition(`test = "create"`).
					WithActive(false).
					WithBug(bugs.BugID{System: "buganizer", ID: "2"}).
					WithBugManaged(true).
					WithBugPriorityManaged(true).
					WithCreateUser("someone@example.com").
					WithLastAuditableUpdateUser("someone@example.com").
					WithSourceCluster(clustering.ClusterID{
						Algorithm: testname.AlgorithmName,
						ID:        strings.Repeat("aa", 16),
					}).
					WithBugManagementState(&bugspb.BugManagementState{})

				acceptStoredRuleTimestamps := func(b *rules.RuleBuilder, storedRule *rules.Entry) *rules.RuleBuilder {
					return b.
						// Accept whatever CreationTime was assigned, as it
						// is determined by Spanner commit time.
						// Rule spanner data access code tests already validate
						// this is populated correctly.
						WithCreateTime(storedRule.CreateTime).
						WithLastAuditableUpdateTime(storedRule.CreateTime).
						WithLastUpdateTime(storedRule.CreateTime).
						WithPredicateLastUpdateTime(storedRule.CreateTime).
						WithBugPriorityManagedLastUpdateTime(storedRule.CreateTime)
				}

				t.Run("Bug not managed by another rule", func(t *ftt.Test) {
					// Re-use the same bug as a rule in another project,
					// where the other rule is not managing the bug.
					request.Rule.Bug = &pb.AssociatedBug{
						System: ruleUnmanagedOther.BugID.System,
						Id:     ruleUnmanagedOther.BugID.ID,
					}

					rule, err := srv.Create(ctx, request)
					assert.Loosely(t, err, should.BeNil)

					storedRule, err := rules.Read(span.Single(ctx), testProject, rule.RuleId)
					assert.Loosely(t, err, should.BeNil)

					expectedRule := acceptStoredRuleTimestamps(expectedRuleBuilder, storedRule).
						// Accept the randomly generated rule ID.
						WithRuleID(rule.RuleId).
						WithBug(ruleUnmanagedOther.BugID).
						Build()

					// Verify the rule was correctly created in the database.
					assert.Loosely(t, storedRule, should.Match(expectedRuleBuilder.Build()))

					// Verify the returned rule matches our expectations.
					mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: true}
					assert.Loosely(t, rule, should.Match(createRulePB(expectedRule, cfg, mask)))
				})
				t.Run("Bug managed by another rule", func(t *ftt.Test) {
					// Re-use the same bug as a rule in another project,
					// where that rule is managing the bug.
					request.Rule.Bug = &pb.AssociatedBug{
						System: ruleManagedOther.BugID.System,
						Id:     ruleManagedOther.BugID.ID,
					}

					rule, err := srv.Create(ctx, request)
					assert.Loosely(t, err, should.BeNil)

					storedRule, err := rules.Read(span.Single(ctx), testProject, rule.RuleId)
					assert.Loosely(t, err, should.BeNil)

					expectedRule := acceptStoredRuleTimestamps(expectedRuleBuilder, storedRule).
						// Accept the randomly generated rule ID.
						WithRuleID(rule.RuleId).
						WithBug(ruleManagedOther.BugID).
						// Because another rule is managing the bug, this rule
						// should be silenlty stopped from managing the bug.
						WithBugManaged(false).
						Build()

					// Verify the rule was correctly created in the database.
					assert.Loosely(t, storedRule, should.Match(expectedRuleBuilder.Build()))

					// Verify the returned rule matches our expectations.
					mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: true}
					assert.Loosely(t, rule, should.Match(createRulePB(expectedRule, cfg, mask)))
				})
				t.Run("Buganizer", func(t *ftt.Test) {
					request.Rule.Bug = &pb.AssociatedBug{
						System: "buganizer",
						Id:     "1111111111",
					}

					rule, err := srv.Create(ctx, request)
					assert.Loosely(t, err, should.BeNil)

					storedRule, err := rules.Read(span.Single(ctx), testProject, rule.RuleId)
					assert.Loosely(t, err, should.BeNil)

					expectedRule := acceptStoredRuleTimestamps(expectedRuleBuilder, storedRule).
						// Accept the randomly generated rule ID.
						WithRuleID(rule.RuleId).
						WithBug(bugs.BugID{System: "buganizer", ID: "1111111111"}).
						Build()

					// Verify the rule was correctly created in the database.
					assert.Loosely(t, storedRule, should.Match(expectedRuleBuilder.Build()))

					// Verify the returned rule matches our expectations.
					mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: true}
					assert.Loosely(t, rule, should.Match(createRulePB(expectedRule, cfg, mask)))
				})
				t.Run("No audit users permission", func(t *ftt.Test) {
					authState.IdentityGroups = removeGroup(authState.IdentityGroups, auditUsersAccessGroup)

					rule, err := srv.Create(ctx, request)
					assert.Loosely(t, err, should.BeNil)

					storedRule, err := rules.Read(span.Single(ctx), testProject, rule.RuleId)
					assert.Loosely(t, err, should.BeNil)

					expectedRule := acceptStoredRuleTimestamps(expectedRuleBuilder, storedRule).
						// Accept the randomly generated rule ID.
						WithRuleID(rule.RuleId).
						Build()

					// Verify the rule was correctly created in the database.
					assert.Loosely(t, storedRule, should.Match(expectedRuleBuilder.Build()))

					// Verify the returned rule matches our expectations.
					mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: false}
					assert.Loosely(t, rule, should.Match(createRulePB(expectedRule, cfg, mask)))
				})
				t.Run("Without source cluster", func(t *ftt.Test) {
					request.Rule.SourceCluster = &pb.ClusterId{}

					// Rule creation should succeed.
					rule, err := srv.Create(ctx, request)
					assert.Loosely(t, err, should.BeNil)

					storedRule, err := rules.Read(span.Single(ctx), testProject, rule.RuleId)
					assert.Loosely(t, err, should.BeNil)

					expectedRule := acceptStoredRuleTimestamps(expectedRuleBuilder, storedRule).
						// Accept the randomly generated rule ID.
						WithRuleID(rule.RuleId).
						WithSourceCluster(clustering.ClusterID{}).
						Build()

					// Verify the rule was correctly created in the database.
					assert.Loosely(t, storedRule, should.Match(expectedRuleBuilder.Build()))

					// Verify the returned rule matches our expectations.
					mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: true}
					assert.Loosely(t, rule, should.Match(createRulePB(expectedRule, cfg, mask)))
				})
			})
		})
		t.Run("CreateWithNewIssue", func(t *ftt.Test) {
			authState.IdentityGroups = []string{luciAnalysisAccessGroup, buganizerAccessGroup, auditUsersAccessGroup}
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "testproject:@project",
					Permission: perms.PermCreateRule,
				},
				{
					Realm:      "testproject:@project",
					Permission: perms.PermGetRuleDefinition,
				},
			}
			request := &pb.CreateRuleWithNewIssueRequest{
				Parent: fmt.Sprintf("projects/%s", testProject),
				Rule: &pb.Rule{
					RuleDefinition:        `test = "createWithIssue"`,
					IsActive:              true,
					IsManagingBug:         true,
					IsManagingBugPriority: true,
					SourceCluster: &pb.ClusterId{
						Algorithm: testname.AlgorithmName,
						Id:        strings.Repeat("aa", 16),
					},
				},
				Issue: &pb.CreateRuleWithNewIssueRequest_Issue{
					Component: &pb.BugComponent{
						System: &pb.BugComponent_IssueTracker{
							IssueTracker: &pb.IssueTrackerComponent{
								ComponentId: 123456,
							},
						},
					},
					Title:       "Issue title.",
					Comment:     "Description.",
					Priority:    pb.BuganizerPriority_P1,
					AccessLimit: pb.CreateRuleWithNewIssueRequest_Issue_Trusted,
				},
			}
			expectedRuleBuilder := rules.NewRule(0).
				WithProject(testProject).
				WithRuleDefinition(`test = "createWithIssue"`).
				WithActive(true).
				WithBugManaged(true).
				WithBugPriorityManaged(true).
				WithSourceCluster(clustering.ClusterID{
					Algorithm: testname.AlgorithmName,
					ID:        strings.Repeat("aa", 16),
				}).
				WithBugManagementState(&bugspb.BugManagementState{})

			t.Run("Not a member of luci-analysis-buganizer-access", func(t *ftt.Test) {
				authState.IdentityGroups = removeGroup(authState.IdentityGroups, "luci-analysis-buganizer-access")

				rule, err := srv.CreateWithNewIssue(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("not a member of luci-analysis-buganizer-access"))
				assert.Loosely(t, rule, should.BeNil)
			})
			t.Run("No create rule permission", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermCreateRule)

				rule, err := srv.CreateWithNewIssue(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("caller does not have permission analysis.rules.create"))
				assert.Loosely(t, rule, should.BeNil)
			})
			t.Run("No create rule definition permission", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetRuleDefinition)

				rule, err := srv.CreateWithNewIssue(ctx, request)
				assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
				assert.Loosely(t, err, should.ErrLike("caller does not have permission analysis.rules.getDefinition"))
				assert.Loosely(t, rule, should.BeNil)
			})
			t.Run("Validation error", func(t *ftt.Test) {
				t.Run("Rule", func(t *ftt.Test) {
					t.Run("Unspecified", func(t *ftt.Test) {
						request.Rule = nil

						_, err := srv.CreateWithNewIssue(ctx, request)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("rule: unspecified"))
					})
					t.Run("Bug specified", func(t *ftt.Test) {
						// This RPC creates a bug, so the rule should not already have a
						// bug.
						request.Rule.Bug = &pb.AssociatedBug{
							System: "buganizer",
							Id:     "12345",
						}

						_, err := srv.CreateWithNewIssue(ctx, request)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike("rule: bug: must not be specified, as a new bug will be created by this RPC"))
					})
					t.Run("Empty rule definition", func(t *ftt.Test) {
						request.Rule.RuleDefinition = ""

						_, err := srv.CreateWithNewIssue(ctx, request)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`rule: rule_definition: unspecified`))
					})
					t.Run("Invalid rule definition", func(t *ftt.Test) {
						// Use an invalid failure association rule.
						request.Rule.RuleDefinition = "<"

						_, err := srv.CreateWithNewIssue(ctx, request)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`rule: rule_definition: parse: syntax error: 1:1: invalid input text "<"`))
					})
				})
				t.Run("Issue", func(t *ftt.Test) {
					t.Run("unspecified", func(t *ftt.Test) {
						request.Issue = nil

						_, err := srv.CreateWithNewIssue(ctx, request)
						assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						assert.Loosely(t, err, should.ErrLike(`issue: unspecified`))
					})
					t.Run("title", func(t *ftt.Test) {
						t.Run("unspecified", func(t *ftt.Test) {
							request.Issue.Title = ""

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: title: unspecified`))
						})
						t.Run("too long", func(t *ftt.Test) {
							request.Issue.Title = strings.Repeat("a", 251)

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: title: longer than 250 bytes`))
						})
						t.Run("invalid - non-printables", func(t *ftt.Test) {
							request.Issue.Title = "hello\x00"

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: title: non-printable rune '\x00' at byte index 5`))
						})
						t.Run("invalid - invalid UTF-8", func(t *ftt.Test) {
							request.Issue.Title = "\xFF"

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: title: not a valid utf8 string`))
						})
						t.Run("invalid - not in normal form C", func(t *ftt.Test) {
							// U+0041 (LATIN CAPITAL LETTER A) followed by U+030A (COMBINING ACUTE ACCENT) is Normal Form D.
							// It should be written as U+00C5 (LATIN CAPITAL LETTER A WITH RING ABOVE) in Normal Form C.
							request.Issue.Title = "\u0041\u030a"

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: title: not in unicode normalized form C`))
						})
					})
					t.Run("comment", func(t *ftt.Test) {
						t.Run("unspecified", func(t *ftt.Test) {
							request.Issue.Comment = ""

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: comment: unspecified`))
						})
						t.Run("too long", func(t *ftt.Test) {
							request.Issue.Comment = strings.Repeat("a", 100_001)

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: comment: longer than 100000 bytes`))
						})
						t.Run("invalid - non-printables", func(t *ftt.Test) {
							request.Issue.Comment = "hello\x00"

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: comment: non-printable rune '\x00' at byte index 5`))
						})
						t.Run("invalid - invalid UTF-8", func(t *ftt.Test) {
							request.Issue.Comment = "\xFF"

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: comment: not a valid utf8 string`))
						})
						t.Run("invalid - not in normal form C", func(t *ftt.Test) {
							// U+0041 (LATIN CAPITAL LETTER A) followed by U+030A (COMBINING ACUTE ACCENT) is Normal Form D.
							// It should be written as U+00C5 (LATIN CAPITAL LETTER A WITH RING ABOVE) in Normal Form C.
							request.Issue.Comment = "\u0041\u030a"

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: comment: not in unicode normalized form C`))
						})
					})
					t.Run("access limit", func(t *ftt.Test) {
						t.Run("unspecified", func(t *ftt.Test) {
							request.Issue.AccessLimit = pb.CreateRuleWithNewIssueRequest_Issue_ISSUE_ACCESS_LIMIT_UNSPECIFIED

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: access_limit: unspecified`))
						})
						t.Run("invalid", func(t *ftt.Test) {
							request.Issue.AccessLimit = pb.CreateRuleWithNewIssueRequest_Issue_IssueAccessLimit(100)

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: access_limit: invalid value, must be a valid IssueAccessLimit`))
						})
					})
					t.Run("priority", func(t *ftt.Test) {
						t.Run("unspecified", func(t *ftt.Test) {
							request.Issue.Priority = pb.BuganizerPriority_BUGANIZER_PRIORITY_UNSPECIFIED

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: priority: unspecified`))
						})
						t.Run("invalid", func(t *ftt.Test) {
							request.Issue.Priority = pb.BuganizerPriority(100)

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: priority: invalid value, must be a valid BuganizerPriority`))
						})
					})
					t.Run("component", func(t *ftt.Test) {
						t.Run("unspecified", func(t *ftt.Test) {
							request.Issue.Component = nil

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: component: unspecified`))
						})
						t.Run("system unspecified", func(t *ftt.Test) {
							request.Issue.Component.System = nil

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: component: system: unspecified`))
						})
						t.Run("invalid - monorail system", func(t *ftt.Test) {
							request.Issue.Component.System = &pb.BugComponent_Monorail{
								Monorail: &pb.MonorailComponent{
									Project: "testproject",
									Value:   "MyComponent",
								},
							}

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: component: monorail: filing bugs into monorail is not supported by this RPC`))
						})
						t.Run("invalid - issue tracker unspecified", func(t *ftt.Test) {
							request.Issue.Component.System = &pb.BugComponent_IssueTracker{
								IssueTracker: nil,
							}

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: component: issue_tracker: unspecified`))
						})
						t.Run("component ID unspecified", func(t *ftt.Test) {
							request.Issue.Component.GetIssueTracker().ComponentId = 0

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: component: issue_tracker: component_id: unspecified`))
						})
						t.Run("component ID invalid", func(t *ftt.Test) {
							request.Issue.Component.GetIssueTracker().ComponentId = -1

							_, err := srv.CreateWithNewIssue(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike(`issue: component: issue_tracker: component_id: must be positive`))
						})
					})
				})
			})
			t.Run("Success", func(t *ftt.Test) {
				t.Run("With audit access", func(t *ftt.Test) {
					rule, err := srv.CreateWithNewIssue(ctx, request)
					assert.Loosely(t, err, should.BeNil)

					// Verify the issue that was filed.
					assert.Loosely(t, buganizerClient.FakeStore.Issues, should.HaveLength(1))
					issue := buganizerClient.FakeStore.Issues[1]
					assert.Loosely(t, issue, should.NotBeNil)
					assert.Loosely(t, issue.Issue.IssueState.ComponentId, should.Equal(123456))
					assert.Loosely(t, issue.Issue.IssueState.Title, should.Equal("Issue title."))
					assert.Loosely(t, issue.Comments[0].Comment, should.Equal("Description.\n\n"+
						"View example failures and modify the failures associated with this bug in LUCI Analysis: https://analysis.luci.app/p/testproject/rules/"+rule.RuleId+". "+
						"Filed on behalf of someone@example.com."))
					assert.Loosely(t, issue.Issue.IssueState.AccessLimit.AccessLevel, should.Equal(issuetracker.IssueAccessLimit_LIMIT_VIEW_TRUSTED))
					assert.Loosely(t, issue.Issue.IssueState.Priority, should.Equal(pb.BuganizerPriority_P1))

					storedRule, err := rules.Read(span.Single(ctx), testProject, rule.RuleId)
					assert.Loosely(t, err, should.BeNil)

					expectedRule := expectedRuleBuilder.
						// Accept the randomly generated rule ID.
						WithRuleID(rule.RuleId).
						// Accept the ID of the filed bug.
						WithBug(bugs.BugID{System: "buganizer", ID: "1"}).
						WithCreateUser("someone@example.com").
						WithLastAuditableUpdateUser("someone@example.com").
						// Accept whatever CreationTime was assigned, as it
						// is determined by Spanner commit time.
						// Rule spanner data access code tests already validate
						// this is populated correctly.
						WithCreateTime(storedRule.CreateTime).
						WithLastAuditableUpdateTime(storedRule.CreateTime).
						WithLastUpdateTime(storedRule.CreateTime).
						WithPredicateLastUpdateTime(storedRule.CreateTime).
						WithBugPriorityManagedLastUpdateTime(storedRule.CreateTime).
						Build()

					// Verify the rule was correctly created in the database.
					assert.Loosely(t, storedRule, should.Match(expectedRule))

					// Verify the returned rule matches our expectations.
					mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: true}
					assert.Loosely(t, rule, should.Match(createRulePB(expectedRule, cfg, mask)))
				})
				t.Run("Without audit access", func(t *ftt.Test) {
					authState.IdentityGroups = removeGroup(authState.IdentityGroups, auditUsersAccessGroup)

					rule, err := srv.CreateWithNewIssue(ctx, request)
					assert.Loosely(t, err, should.BeNil)

					// Verify the issue that was filed.
					assert.Loosely(t, buganizerClient.FakeStore.Issues, should.HaveLength(1))
					issue := buganizerClient.FakeStore.Issues[1]
					assert.Loosely(t, issue, should.NotBeNil)
					assert.Loosely(t, issue.Issue.IssueState.ComponentId, should.Equal(123456))
					assert.Loosely(t, issue.Issue.IssueState.Title, should.Equal("Issue title."))
					assert.Loosely(t, issue.Comments[0].Comment, should.Equal("Description.\n\n"+
						"View example failures and modify the failures associated with this bug in LUCI Analysis: https://analysis.luci.app/p/testproject/rules/"+rule.RuleId+". "+
						"Filed on behalf of someone@example.com."))
					assert.Loosely(t, issue.Issue.IssueState.AccessLimit.AccessLevel, should.Equal(issuetracker.IssueAccessLimit_LIMIT_VIEW_TRUSTED))
					assert.Loosely(t, issue.Issue.IssueState.Priority, should.Equal(pb.BuganizerPriority_P1))

					storedRule, err := rules.Read(span.Single(ctx), testProject, rule.RuleId)
					assert.Loosely(t, err, should.BeNil)

					expectedRule := expectedRuleBuilder.
						// Accept the randomly generated rule ID.
						WithRuleID(rule.RuleId).
						// Accept the ID of the filed bug.
						WithBug(bugs.BugID{System: "buganizer", ID: "1"}).
						WithCreateUser("someone@example.com").
						WithLastAuditableUpdateUser("someone@example.com").
						// Accept whatever CreationTime was assigned, as it
						// is determined by Spanner commit time.
						// Rule spanner data access code tests already validate
						// this is populated correctly.
						WithCreateTime(storedRule.CreateTime).
						WithLastAuditableUpdateTime(storedRule.CreateTime).
						WithLastUpdateTime(storedRule.CreateTime).
						WithPredicateLastUpdateTime(storedRule.CreateTime).
						WithBugPriorityManagedLastUpdateTime(storedRule.CreateTime).
						Build()

					// Verify the rule was correctly created in the database.
					assert.Loosely(t, storedRule, should.Match(expectedRule))

					// Verify the returned rule matches our expectations.
					mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: false}
					assert.Loosely(t, rule, should.Match(createRulePB(expectedRule, cfg, mask)))
				})
				t.Run("Comment with newlines, carriage returns and tabs is allowed", func(t *ftt.Test) {
					request.Issue.Comment = "Description with\nnewline, \r\ncarriage return and\ttab."

					rule, err := srv.CreateWithNewIssue(ctx, request)
					assert.Loosely(t, err, should.BeNil)

					// Verify the issue that was filed.
					assert.Loosely(t, buganizerClient.FakeStore.Issues, should.HaveLength(1))
					issue := buganizerClient.FakeStore.Issues[1]
					assert.Loosely(t, issue, should.NotBeNil)
					assert.Loosely(t, issue.Comments[0].Comment, should.Equal("Description with\nnewline, \r\ncarriage return and\ttab.\n\n"+
						"View example failures and modify the failures associated with this bug in LUCI Analysis: https://analysis.luci.app/p/testproject/rules/"+rule.RuleId+". "+
						"Filed on behalf of someone@example.com."))

					storedRule, err := rules.Read(span.Single(ctx), testProject, rule.RuleId)
					assert.Loosely(t, err, should.BeNil)

					expectedRule := expectedRuleBuilder.
						// Accept the randomly generated rule ID.
						WithRuleID(rule.RuleId).
						// Accept the ID of the filed bug.
						WithBug(bugs.BugID{System: "buganizer", ID: "1"}).
						WithCreateUser("someone@example.com").
						WithLastAuditableUpdateUser("someone@example.com").
						// Accept whatever CreationTime was assigned, as it
						// is determined by Spanner commit time.
						// Rule spanner data access code tests already validate
						// this is populated correctly.
						WithCreateTime(storedRule.CreateTime).
						WithLastAuditableUpdateTime(storedRule.CreateTime).
						WithLastUpdateTime(storedRule.CreateTime).
						WithPredicateLastUpdateTime(storedRule.CreateTime).
						WithBugPriorityManagedLastUpdateTime(storedRule.CreateTime).
						Build()

					// Verify the rule was correctly created in the database.
					assert.Loosely(t, storedRule, should.Match(expectedRule))

					// Verify the returned rule matches our expectations.
					mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: true}
					assert.Loosely(t, rule, should.Match(createRulePB(expectedRule, cfg, mask)))
				})
			})
		})
		t.Run("LookupBug", func(t *ftt.Test) {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "testproject:@project",
					Permission: perms.PermListRules,
				},
				{
					Realm:      "otherproject:@project",
					Permission: perms.PermListRules,
				},
			}

			t.Run("Exists None", func(t *ftt.Test) {
				request := &pb.LookupBugRequest{
					System: "buganizer",
					Id:     "999999",
				}

				response, err := srv.LookupBug(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response, should.Match(&pb.LookupBugResponse{
					Rules: []string{},
				}))
			})
			t.Run("Exists One", func(t *ftt.Test) {
				request := &pb.LookupBugRequest{
					System: ruleManaged.BugID.System,
					Id:     ruleManaged.BugID.ID,
				}

				response, err := srv.LookupBug(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response, should.Match(&pb.LookupBugResponse{
					Rules: []string{
						fmt.Sprintf("projects/%s/rules/%s",
							ruleManaged.Project, ruleManaged.RuleID),
					},
				}))

				t.Run("If no permission in relevant project", func(t *ftt.Test) {
					authState.IdentityPermissions = nil

					response, err := srv.LookupBug(ctx, request)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Match(&pb.LookupBugResponse{
						Rules: []string{},
					}))
				})
			})
			t.Run("Exists Many", func(t *ftt.Test) {
				request := &pb.LookupBugRequest{
					System: ruleTwoProject.BugID.System,
					Id:     ruleTwoProject.BugID.ID,
				}

				response, err := srv.LookupBug(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response, should.Match(&pb.LookupBugResponse{
					Rules: []string{
						// Rules are returned alphabetically by project.
						fmt.Sprintf("projects/otherproject/rules/%s", ruleTwoProjectOther.RuleID),
						fmt.Sprintf("projects/testproject/rules/%s", ruleTwoProject.RuleID),
					},
				}))

				t.Run("If list permission exists in only some projects", func(t *ftt.Test) {
					authState.IdentityPermissions = []authtest.RealmPermission{
						{
							Realm:      "testproject:@project",
							Permission: perms.PermListRules,
						},
					}

					response, err := srv.LookupBug(ctx, request)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Match(&pb.LookupBugResponse{
						Rules: []string{
							fmt.Sprintf("projects/testproject/rules/%s", ruleTwoProject.RuleID),
						},
					}))
				})
			})
			t.Run("PrepareDefaults", func(t *ftt.Test) {
				authState.IdentityPermissions = []authtest.RealmPermission{
					{
						Realm:      "testproject:@project",
						Permission: perms.PermGetConfig,
					},
				}

				request := &pb.PrepareRuleDefaultsRequest{
					Parent: fmt.Sprintf("projects/%s", testProject),
					TestResult: &pb.PrepareRuleDefaultsRequest_TestResult{
						TestId: "ninja://some_package/some_test",
						FailureReason: &pb.FailureReason{
							PrimaryErrorMessage: "Some error 12345678.",
						},
					},
				}
				t.Run("No get config permission", func(t *ftt.Test) {
					authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetConfig)

					_, err := srv.PrepareDefaults(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.PermissionDenied))
					assert.Loosely(t, err, should.ErrLike("caller does not have permission analysis.config.get"))
				})
				t.Run("Validation", func(t *ftt.Test) {
					t.Run("test ID", func(t *ftt.Test) {
						t.Run("unspecified", func(t *ftt.Test) {
							request.TestResult.TestId = ""

							_, err := srv.PrepareDefaults(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
						})
						t.Run("invalid", func(t *ftt.Test) {
							request.TestResult.TestId = strings.Repeat("a", 513)

							_, err := srv.PrepareDefaults(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike("test_result: test_id: longer than 512 bytes"))
						})
					})
					t.Run("failure reason", func(t *ftt.Test) {
						t.Run("invalid", func(t *ftt.Test) {
							request.TestResult.FailureReason.PrimaryErrorMessage = strings.Repeat("a", 1025)

							_, err := srv.PrepareDefaults(ctx, request)
							assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
							assert.Loosely(t, err, should.ErrLike("test_result: failure_reason: primary_error_message: exceeds the maximum size of 1024 bytes"))
						})
					})
				})
				t.Run("Success", func(t *ftt.Test) {
					t.Run("Baseline", func(t *ftt.Test) {
						response, err := srv.PrepareDefaults(ctx, request)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, response.Rule, should.Match(&pb.Rule{
							RuleDefinition: `test = "ninja://some_package/some_test" AND reason LIKE "Some error %."`,
							IsActive:       true,
						}))
					})
					t.Run("With no failure reason", func(t *ftt.Test) {
						request.TestResult.FailureReason = nil

						response, err := srv.PrepareDefaults(ctx, request)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, response.Rule, should.Match(&pb.Rule{
							RuleDefinition: `test = "ninja://some_package/some_test"`,
							IsActive:       true,
						}))
					})
					t.Run("With no test result", func(t *ftt.Test) {
						request.TestResult = nil

						response, err := srv.PrepareDefaults(ctx, request)
						assert.Loosely(t, err, should.BeNil)
						assert.Loosely(t, response.Rule, should.Match(&pb.Rule{
							RuleDefinition: ``,
							IsActive:       true,
						}))
					})
				})
			})
		})
	})
	ftt.Run("createRulePB", t, func(t *ftt.Test) {
		// The behaviour of createRulePB is assumed by many test cases.
		// This verifies that behaviour.

		rule := rules.NewRule(1).
			WithProject(testProject).
			WithBug(bugs.BugID{System: "buganizer", ID: "111"}).Build()

		expectedRule := &pb.Rule{
			Name:                    "projects/testproject/rules/8109e4f8d9c218e9a2e33a7a21395455",
			Project:                 "testproject",
			RuleId:                  "8109e4f8d9c218e9a2e33a7a21395455",
			RuleDefinition:          "reason LIKE \"%exit code 5%\" AND test LIKE \"tast.arc.%\"",
			IsActive:                true,
			PredicateLastUpdateTime: timestamppb.New(time.Date(1904, 4, 4, 4, 4, 4, 1, time.UTC)),
			Bug: &pb.AssociatedBug{
				System:   "buganizer",
				Id:       "111",
				LinkText: "b/111",
				Url:      "https://issuetracker.google.com/issues/111",
			},
			IsManagingBug:                       true,
			IsManagingBugPriority:               true,
			IsManagingBugPriorityLastUpdateTime: timestamppb.New(time.Date(1905, 5, 5, 5, 5, 5, 1, time.UTC)),
			SourceCluster: &pb.ClusterId{
				Algorithm: "clusteralg1-v9",
				Id:        "696431",
			},
			BugManagementState: &pb.BugManagementState{
				PolicyState: []*pb.BugManagementState_PolicyState{
					{
						PolicyId:           "policy-a",
						IsActive:           true,
						LastActivationTime: timestamppb.New(time.Date(1908, 8, 8, 8, 8, 8, 1, time.UTC)),
					},
				},
			},
			CreateTime:              timestamppb.New(time.Date(1900, 1, 2, 3, 4, 5, 1, time.UTC)),
			CreateUser:              "system",
			LastAuditableUpdateTime: timestamppb.New(time.Date(1907, 7, 7, 7, 7, 7, 1, time.UTC)), //FIX ME
			LastAuditableUpdateUser: "user@google.com",
			LastUpdateTime:          timestamppb.New(time.Date(1909, 9, 9, 9, 9, 9, 1, time.UTC)),
			Etag:                    `W/"+d+u/1909-09-09T09:09:09.000000001Z"`,
		}
		cfg := &configpb.ProjectConfig{}
		mask := ruleMask{
			IncludeDefinition: true,
			IncludeAuditUsers: true,
		}
		t.Run("With all fields", func(t *ftt.Test) {
			assert.Loosely(t, createRulePB(rule, cfg, mask), should.Match(expectedRule))
		})
		t.Run("Without definition field", func(t *ftt.Test) {
			mask.IncludeDefinition = false
			expectedRule.RuleDefinition = ""
			expectedRule.Etag = `W/"+u/1909-09-09T09:09:09.000000001Z"`
			assert.Loosely(t, createRulePB(rule, cfg, mask), should.Match(expectedRule))
		})
		t.Run("Without audit users", func(t *ftt.Test) {
			mask.IncludeAuditUsers = false
			expectedRule.CreateUser = ""
			expectedRule.LastAuditableUpdateUser = ""
			expectedRule.Etag = `W/"+d/1909-09-09T09:09:09.000000001Z"`
			assert.Loosely(t, createRulePB(rule, cfg, mask), should.Match(expectedRule))
		})
	})
	ftt.Run("isETagValid", t, func(t *ftt.Test) {
		rule := rules.NewRule(0).
			WithProject(testProject).
			WithBug(bugs.BugID{System: "buganizer", ID: "111"}).Build()

		t.Run("Should match ETags for same rule version", func(t *ftt.Test) {
			masks := []ruleMask{
				{IncludeDefinition: true, IncludeAuditUsers: true},
				{IncludeDefinition: true, IncludeAuditUsers: false},
				{IncludeDefinition: false, IncludeAuditUsers: true},
				{IncludeDefinition: false, IncludeAuditUsers: false},
			}
			for _, mask := range masks {
				assert.Loosely(t, isETagMatching(rule, ruleETag(rule, mask)), should.BeTrue)
			}
		})
		t.Run("Should not match ETag for different version of rule", func(t *ftt.Test) {
			mask := ruleMask{IncludeDefinition: true, IncludeAuditUsers: true}
			etag := ruleETag(rule, mask)

			rule.LastUpdateTime = time.Date(2021, 5, 4, 3, 2, 1, 0, time.UTC)
			assert.Loosely(t, isETagMatching(rule, etag), should.BeFalse)
		})
	})
	ftt.Run("formatRule", t, func(t *ftt.Test) {
		rule := rules.NewRule(0).
			WithProject(testProject).
			WithBug(bugs.BugID{System: "buganizer", ID: "123456"}).
			WithRuleDefinition(`test = "create"`).
			WithActive(false).
			WithBugManaged(true).
			WithSourceCluster(clustering.ClusterID{
				Algorithm: testname.AlgorithmName,
				ID:        strings.Repeat("aa", 16),
			}).Build()
		expectedRule := `{
	RuleDefinition: "test = \"create\"",
	BugID: "buganizer:123456",
	IsActive: false,
	IsManagingBug: true,
	IsManagingBugPriority: true,
	SourceCluster: "testname-v4:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	LastAuditableUpdate: "1907-07-07T07:07:07Z"
	LastUpdated: "1909-09-09T09:09:09Z"
}`
		assert.Loosely(t, formatRule(rule), should.Equal(expectedRule))
	})
}

func removeGroup(groups []string, group string) []string {
	var result []string
	for _, g := range groups {
		if g != group {
			result = append(result, g)
		}
	}
	return result
}
