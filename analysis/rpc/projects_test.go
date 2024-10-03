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
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	grpcStatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"

	"go.chromium.org/luci/analysis/internal/analysis/metrics"
	"go.chromium.org/luci/analysis/internal/config"
	"go.chromium.org/luci/analysis/internal/perms"
	configpb "go.chromium.org/luci/analysis/proto/config"
	pb "go.chromium.org/luci/analysis/proto/v1"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestProjects(t *testing.T) {
	ftt.Run("Given a projects server", t, func(t *ftt.Test) {
		ctx := context.Background()

		// For user identification.
		ctx = authtest.MockAuthConfig(ctx)
		authState := &authtest.FakeState{
			Identity:       "user:someone@example.com",
			IdentityGroups: []string{"luci-analysis-access"},
		}
		ctx = auth.WithState(ctx, authState)
		ctx = secrets.Use(ctx, &testsecrets.Store{})

		// Provides datastore implementation needed for project config.
		ctx = memory.Use(ctx)
		server := NewProjectsServer()

		t.Run("Unauthorised requests are rejected", func(t *ftt.Test) {
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity: "user:someone@example.com",
				// Not a member of luci-analysis-access.
				IdentityGroups: []string{"other-group"},
			})

			// Make some request (the request should not matter, as
			// a common decorator is used for all requests.)
			request := &pb.ListProjectsRequest{}

			rule, err := server.List(ctx, request)
			st, _ := grpcStatus.FromError(err)
			assert.Loosely(t, st.Code(), should.Equal(codes.PermissionDenied))
			assert.Loosely(t, st.Message(), should.Equal("not a member of luci-analysis-access"))
			assert.Loosely(t, rule, should.BeNil)
		})
		t.Run("GetConfig", func(t *ftt.Test) {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "testproject:@project",
					Permission: perms.PermGetConfig,
				},
			}

			// Setup.
			configs := make(map[string]*configpb.ProjectConfig)
			projectCfg := config.CreateConfigWithBothBuganizerAndMonorail(configpb.BugSystem_BUGANIZER)

			configs["testproject"] = projectCfg
			assert.Loosely(t, config.SetTestProjectConfig(ctx, configs), should.BeNil)

			request := &pb.GetProjectConfigRequest{
				Name: "projects/testproject/config",
			}

			t.Run("No permission to get project config", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetConfig)

				response, err := server.GetConfig(ctx, request)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCPermissionDenied)("caller does not have permission analysis.config.get"))
				assert.Loosely(t, response, should.BeNil)
			})
			t.Run("Valid request", func(t *ftt.Test) {
				expectedResponse := &pb.ProjectConfig{
					Name: "projects/testproject/config",
					BugManagement: &pb.BugManagement{
						Policies: []*pb.BugManagementPolicy{
							{
								Id:                "exoneration",
								Owners:            []string{"username@google.com"},
								HumanReadableName: "test variant(s) are being exonerated in presubmit",
								Priority:          pb.BuganizerPriority_P2,
								Metrics: []*pb.BugManagementPolicy_Metric{
									{
										MetricId: metrics.CriticalFailuresExonerated.ID.String(),
										ActivationThreshold: &pb.MetricThreshold{
											OneDay: proto.Int64(50),
										},
										DeactivationThreshold: &pb.MetricThreshold{
											ThreeDay: proto.Int64(20),
										},
									},
								},
								Explanation: &pb.BugManagementPolicy_Explanation{
									ProblemHtml: "Test variant(s) in the cluster are being exonerated because they are too flaky or failing.",
									ActionHtml:  "<ul><li>View recent failures and fix them</li><li>Demote the test(s) from CQ</li></ul>",
								},
							},
						},
						Monorail: &pb.MonorailProject{
							Project:       "chromium",
							DisplayPrefix: "crbug.com",
						},
					},
				}

				t.Run("baseline", func(t *ftt.Test) {
					// Run
					response, err := server.GetConfig(ctx, request)

					// Verify
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response, should.Resemble(expectedResponse))
				})
				t.Run("without monorail config", func(t *ftt.Test) {
					projectCfg.BugManagement.Monorail = nil
					assert.Loosely(t, config.SetTestProjectConfig(ctx, configs), should.BeNil)

					// Run
					response, err := server.GetConfig(ctx, request)

					// Verify
					assert.Loosely(t, err, should.BeNil)
					expectedResponse.BugManagement.Monorail = nil
					assert.Loosely(t, response, should.Resemble(expectedResponse))
				})
				t.Run("policy without activation threshold", func(t *ftt.Test) {
					projectCfg.BugManagement.Policies[0].Metrics[0].ActivationThreshold = nil
					assert.Loosely(t, config.SetTestProjectConfig(ctx, configs), should.BeNil)

					// Run
					response, err := server.GetConfig(ctx, request)

					// Verify
					assert.Loosely(t, err, should.BeNil)
					expectedResponse.BugManagement.Policies[0].Metrics[0].ActivationThreshold = nil
					assert.Loosely(t, response, should.Resemble(expectedResponse))
				})
			})
			t.Run("With project not configured", func(t *ftt.Test) {
				err := config.SetTestProjectConfig(ctx, map[string]*configpb.ProjectConfig{})
				assert.Loosely(t, err, should.BeNil)

				// Run
				response, err := server.GetConfig(ctx, request)

				// Verify
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, response, should.Resemble(&pb.ProjectConfig{
					Name:          "projects/testproject/config",
					BugManagement: &pb.BugManagement{},
				}))
			})
			t.Run("Invalid request", func(t *ftt.Test) {
				request.Name = "blah"

				// Run
				response, err := server.GetConfig(ctx, request)

				// Verify
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("name: invalid project config name, expected format: projects/{project}/config"))
				assert.Loosely(t, response, should.BeNil)
			})
		})
		t.Run("List", func(t *ftt.Test) {
			authState.IdentityPermissions = []authtest.RealmPermission{
				{
					Realm:      "chromium:@project",
					Permission: perms.PermGetConfig,
				},
				{
					Realm:      "chrome:@project",
					Permission: perms.PermGetConfig,
				},
				{
					Realm:      "chromeos:@project",
					Permission: perms.PermGetConfig,
				},
			}

			request := &pb.ListProjectsRequest{}

			t.Run("No permission to view any project", func(t *ftt.Test) {
				authState.IdentityPermissions = removePermission(authState.IdentityPermissions, perms.PermGetConfig)

				// Run
				projectsResponse, err := server.List(ctx, request)

				// Verify
				assert.Loosely(t, err, should.BeNil)
				expected := &pb.ListProjectsResponse{Projects: []*pb.Project{}}
				assert.Loosely(t, projectsResponse, should.Resemble(expected))
			})
			t.Run("Valid request", func(t *ftt.Test) {
				// Run
				projectsResponse, err := server.List(ctx, request)

				// Verify
				assert.Loosely(t, err, should.BeNil)
				expected := &pb.ListProjectsResponse{Projects: []*pb.Project{
					{
						Name:        "projects/chrome",
						DisplayName: "Chrome",
						Project:     "chrome",
					},
					{
						Name:        "projects/chromeos",
						DisplayName: "Chromeos",
						Project:     "chromeos",
					},
					{
						Name:        "projects/chromium",
						DisplayName: "Chromium",
						Project:     "chromium",
					},
				}}
				assert.Loosely(t, projectsResponse, should.Resemble(expected))
			})
		})
	})
}
