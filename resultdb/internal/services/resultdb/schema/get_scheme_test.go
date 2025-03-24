// Copyright 2025 The LUCI Authors.
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

package schema

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"

	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil/testing/grpccode"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"

	"go.chromium.org/luci/resultdb/internal/config"
	"go.chromium.org/luci/resultdb/pbutil"
	configpb "go.chromium.org/luci/resultdb/proto/config"
	pb "go.chromium.org/luci/resultdb/proto/v1"
)

func TestGetScheme(t *testing.T) {
	ftt.Run("With a schemas server", t, func(t *ftt.Test) {
		// For user identification.
		ctx := authtest.MockAuthConfig(context.Background())
		authState := &authtest.FakeState{
			Identity: "user:someone@example.com",
		}
		ctx = auth.WithState(ctx, authState)

		ctx = caching.WithEmptyProcessCache(ctx) // For config in-process cache.
		ctx = memory.Use(ctx)                    // Provides datastore implementation needed for project config.

		cfg := &configpb.Config{
			Schemes: []*configpb.Scheme{
				{
					Id:                "gtest",
					HumanReadableName: "GTest",
					Fine: &configpb.Scheme_Level{
						HumanReadableName: "Suite",
					},
					Case: &configpb.Scheme_Level{
						HumanReadableName: "Method",
					},
				},
				{
					Id:                "junit",
					HumanReadableName: "JUnit",
					Coarse: &configpb.Scheme_Level{
						HumanReadableName: "Package",
						ValidationRegexp:  "^PackageRe.*$",
					},
					Fine: &configpb.Scheme_Level{
						HumanReadableName: "Class",
						ValidationRegexp:  "^ClassRe.*$",
					},
					Case: &configpb.Scheme_Level{
						HumanReadableName: "Method",
						ValidationRegexp:  "^MethodRe.*$",
					},
				},
			},
		}
		err := config.SetServiceConfigForTesting(ctx, cfg)
		assert.NoErr(t, err)

		server := NewSchemasServer()

		request := &pb.GetSchemeRequest{
			Name: "schema/schemes/junit",
		}

		expectedResponse := &pb.Scheme{
			Name:              "schema/schemes/junit",
			Id:                "junit",
			HumanReadableName: "JUnit",
			Coarse: &pb.Scheme_Level{
				HumanReadableName: "Package",
				ValidationRegexp:  "^PackageRe.*$",
			},
			Fine: &pb.Scheme_Level{
				HumanReadableName: "Class",
				ValidationRegexp:  "^ClassRe.*$",
			},
			Case: &pb.Scheme_Level{
				HumanReadableName: "Method",
				ValidationRegexp:  "^MethodRe.*$",
			},
		}

		t.Run("Exists", func(t *ftt.Test) {
			response, err := server.GetScheme(ctx, request)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, response, should.Match(expectedResponse))
		})
		t.Run("Built-in legacy scheme", func(t *ftt.Test) {
			request := &pb.GetSchemeRequest{
				Name: "schema/schemes/legacy",
			}
			response, err := server.GetScheme(ctx, request)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, response, should.Match(&pb.Scheme{
				Name:              "schema/schemes/legacy",
				Id:                pbutil.LegacySchemeID,
				HumanReadableName: "Legacy test results",
				Case: &pb.Scheme_Level{
					HumanReadableName: "Test Identifier",
				},
			}))
		})
		t.Run("Not exists", func(t *ftt.Test) {
			request.Name = "schema/schemes/notexisting"

			_, err := server.GetScheme(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike(`scheme with ID "notexisting" not found`))
		})
		t.Run("Invalid request", func(t *ftt.Test) {
			t.Run("Name", func(t *ftt.Test) {
				t.Run("Empty", func(t *ftt.Test) {
					request.Name = ""
					_, err := server.GetScheme(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike("name: unspecified"))
				})
				t.Run("Invalid", func(t *ftt.Test) {
					request.Name = "blah"
					_, err := server.GetScheme(ctx, request)
					assert.Loosely(t, err, grpccode.ShouldBe(codes.InvalidArgument))
					assert.Loosely(t, err, should.ErrLike(`name: invalid scheme name, expected format: "^schema/schemes/([a-z][a-z0-9]{0,19})$"`))
				})
			})
		})
		t.Run("No project config", func(t *ftt.Test) {
			err := config.SetServiceConfigForTesting(ctx, &configpb.Config{})
			assert.NoErr(t, err)

			_, err = server.GetScheme(ctx, request)
			assert.Loosely(t, err, grpccode.ShouldBe(codes.NotFound))
			assert.Loosely(t, err, should.ErrLike(`scheme with ID "junit" not found`))
		})
	})
}
