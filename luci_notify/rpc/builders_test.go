// Copyright 2026 The LUCI Authors.
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
	"bytes"
	"testing"

	"cloud.google.com/go/spanner"

	"go.chromium.org/luci/buildbucket/bbperms"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
	"go.chromium.org/luci/server/span"

	pb "go.chromium.org/luci/luci_notify/api/service/v1"
	"go.chromium.org/luci/luci_notify/internal/testutil"
)

func TestBuilders(t *testing.T) {
	testingT := t
	ftt.Run("With a Builders server", t, func(t *ftt.Test) {
		ctx := testutil.IntegrationTestContext(t)
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = memlogger.Use(ctx)

		// For user identification.
		ctx = authtest.MockAuthConfig(ctx)
		ctx = secrets.Use(ctx, &testsecrets.Store{})

		server := NewBuildersServer()

		defer func() {
			if testingT.Failed() {
				buf := new(bytes.Buffer)
				_, _ = memlogger.Dump(ctx, buf)
				testingT.Logf("Captured Logs:\n%s", buf.String())
			}
		}()

		t.Run("List", func(t *ftt.Test) {
			// Grant access to all test projects by default in tests.
			perms := []authtest.RealmPermission{
				{Realm: "chromium:ci", Permission: bbperms.BuildersList},
				{Realm: "fuchsia:ci", Permission: bbperms.BuildersList},
			}
			ctx = auth.WithState(ctx, &authtest.FakeState{
				Identity:            "user:someone@example.com",
				IdentityGroups:      []string{luciNotifyAccessGroup},
				IdentityPermissions: perms,
			})

			// Pre-insert some builders.
			m1 := spanner.InsertOrUpdateMap("BuilderStatuses", map[string]any{
				"BuilderKey":      "chromium/ci/builder-1",
				"Project":         "chromium",
				"Bucket":          "ci",
				"Builder":         "builder-1",
				"Realm":           "chromium:ci",
				"Status":          "FAILURE",
				"UpdateTime":      spanner.CommitTimestamp,
				"BuildId":         int64(100),
				"OnCallRotations": []string{"angle"},
			})
			m2 := spanner.InsertOrUpdateMap("BuilderStatuses", map[string]any{
				"BuilderKey":      "chromium/ci/builder-2",
				"Project":         "chromium",
				"Bucket":          "ci",
				"Builder":         "builder-2",
				"Realm":           "chromium:ci",
				"Status":          "SUCCESS",
				"UpdateTime":      spanner.CommitTimestamp,
				"BuildId":         int64(200),
				"OnCallRotations": []string{"gardener"},
			})
			m3 := spanner.InsertOrUpdateMap("BuilderStatuses", map[string]any{
				"BuilderKey":      "fuchsia/ci/builder-3",
				"Project":         "fuchsia",
				"Bucket":          "ci",
				"Builder":         "builder-3",
				"Realm":           "fuchsia:ci",
				"Status":          "FAILURE",
				"UpdateTime":      spanner.CommitTimestamp,
				"BuildId":         int64(300),
				"OnCallRotations": []string{"fuchsia"},
			})
			_, err := span.Apply(ctx, []*spanner.Mutation{m1, m2, m3})
			assert.Loosely(t, err, should.BeNil)

			t.Run("List all", func(t *ftt.Test) {
				request := &pb.ListBuildersRequest{}
				response, err := server.List(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(response.Builders), should.Equal(3))
			})

			t.Run("Filter by project", func(t *ftt.Test) {
				request := &pb.ListBuildersRequest{
					Filter: `project = "chromium"`,
				}
				response, err := server.List(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(response.Builders), should.Equal(2))
				assert.Loosely(t, response.Builders[0].Project, should.Equal("chromium"))
				assert.Loosely(t, response.Builders[1].Project, should.Equal("chromium"))
			})

			t.Run("Filter by status", func(t *ftt.Test) {
				request := &pb.ListBuildersRequest{
					Filter: `status = "FAILURE"`,
				}
				response, err := server.List(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(response.Builders), should.Equal(2))
				assert.Loosely(t, response.Builders[0].Status, should.Equal("FAILURE"))
				assert.Loosely(t, response.Builders[1].Status, should.Equal("FAILURE"))
			})

			t.Run("Filter by rotation", func(t *ftt.Test) {
				request := &pb.ListBuildersRequest{
					Filter: `on_call_rotations : "angle"`,
				}
				response, err := server.List(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(response.Builders), should.Equal(1))
				assert.Loosely(t, response.Builders[0].Name, should.Equal("chromium/ci/builder-1"))
			})

			t.Run("Pagination", func(t *ftt.Test) {
				request := &pb.ListBuildersRequest{
					PageSize: 1,
				}
				response, err := server.List(ctx, request)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(response.Builders), should.Equal(1))
				assert.Loosely(t, response.NextPageToken, should.NotEqual(""))

				request2 := &pb.ListBuildersRequest{
					PageSize:  1,
					PageToken: response.NextPageToken,
				}
				response2, err := server.List(ctx, request2)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, len(response2.Builders), should.Equal(1))
				assert.Loosely(t, response2.Builders[0].Name, should.NotEqual(response.Builders[0].Name))
			})

			t.Run("ACL Enforcement", func(t *ftt.Test) {
				t.Run("No access returns empty", func(t *ftt.Test) {
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity:            "user:someone@example.com",
						IdentityGroups:      []string{luciNotifyAccessGroup},
						IdentityPermissions: []authtest.RealmPermission{},
					})
					request := &pb.ListBuildersRequest{}
					response, err := server.List(ctx, request)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, response.Builders, should.HaveLength(0))
				})

				t.Run("Access to specific realm returns filtered list", func(t *ftt.Test) {
					perms := []authtest.RealmPermission{
						{
							Realm:      "chromium:ci",
							Permission: bbperms.BuildersList,
						},
					}
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity:            "user:someone@example.com",
						IdentityGroups:      []string{luciNotifyAccessGroup},
						IdentityPermissions: perms,
					})
					request := &pb.ListBuildersRequest{}
					response, err := server.List(ctx, request)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(response.Builders), should.Equal(2)) // builder-1 and builder-2
				})

				t.Run("Access to project root returns all in project", func(t *ftt.Test) {
					perms := []authtest.RealmPermission{
						{
							Realm:      "chromium:@root",
							Permission: bbperms.BuildersList,
						},
					}
					ctx = auth.WithState(ctx, &authtest.FakeState{
						Identity:            "user:someone@example.com",
						IdentityGroups:      []string{luciNotifyAccessGroup},
						IdentityPermissions: perms,
					})
					request := &pb.ListBuildersRequest{}
					response, err := server.List(ctx, request)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, len(response.Builders), should.Equal(2))
				})
			})
		})
	})
}
