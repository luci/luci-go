// Copyright 2019 The LUCI Authors.
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

package utils

import (
	"context"
	"testing"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
)

type TestStruct struct {
	Prop string
}

func TestTagGRPC(t *testing.T) {
	t.Parallel()

	ftt.Run("GRPC tagging works", t, func(t *ftt.Test) {
		c := memory.Use(context.Background())
		cUser := auth.WithState(c, &authtest.FakeState{Identity: "user:user@example.com"})
		cAnon := auth.WithState(c, &authtest.FakeState{Identity: identity.AnonymousIdentity})

		t.Run("For not found errors", func(t *ftt.Test) {
			errGRPCNotFound := status.Errorf(codes.NotFound, "not found")
			assert.Loosely(t, grpcutil.Code(TagGRPC(cAnon, errGRPCNotFound)), should.Equal(codes.Unauthenticated))
			assert.Loosely(t, grpcutil.Code(TagGRPC(cUser, errGRPCNotFound)), should.Equal(codes.NotFound))
		})

		t.Run("For permission denied errors", func(t *ftt.Test) {
			errGRPCPermissionDenied := status.Errorf(codes.PermissionDenied, "permission denied")
			assert.Loosely(t, grpcutil.Code(TagGRPC(cAnon, errGRPCPermissionDenied)), should.Equal(codes.Unauthenticated))
			assert.Loosely(t, grpcutil.Code(TagGRPC(cUser, errGRPCPermissionDenied)), should.Equal(codes.NotFound))
		})

		t.Run("For invalid argument errors", func(t *ftt.Test) {
			errGRPCInvalidArgument := status.Errorf(codes.InvalidArgument, "invalid argument")
			assert.Loosely(t, grpcutil.Code(TagGRPC(cAnon, errGRPCInvalidArgument)), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, grpcutil.Code(TagGRPC(cUser, errGRPCInvalidArgument)), should.Equal(codes.InvalidArgument))
		})

		t.Run("For invalid argument multi-errors", func(t *ftt.Test) {
			errGRPCInvalidArgument := status.Errorf(codes.InvalidArgument, "invalid argument")
			errMulti := errors.NewMultiError(errGRPCInvalidArgument)
			assert.Loosely(t, grpcutil.Code(TagGRPC(cAnon, errMulti)), should.Equal(codes.InvalidArgument))
			assert.Loosely(t, grpcutil.Code(TagGRPC(cUser, errMulti)), should.Equal(codes.InvalidArgument))
		})
	})

	ftt.Run("ParseLegacyBuilderID", t, func(t *ftt.Test) {
		t.Run("For valid ID", func(t *ftt.Test) {
			builderID, err := ParseLegacyBuilderID("buildbucket/luci.test project.test bucket/test builder")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, builderID, should.Match(&buildbucketpb.BuilderID{
				Project: "test project",
				Bucket:  "test bucket",
				Builder: "test builder",
			}))
		})

		t.Run("Allow '.' in builder ID", func(t *ftt.Test) {
			builderID, err := ParseLegacyBuilderID("buildbucket/luci.test project.test bucket/test.builder")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, builderID, should.Match(&buildbucketpb.BuilderID{
				Project: "test project",
				Bucket:  "test bucket",
				Builder: "test.builder",
			}))
		})

		t.Run("Allow '.' in bucket ID", func(t *ftt.Test) {
			builderID, err := ParseLegacyBuilderID("buildbucket/luci.test project.test.bucket/test builder")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, builderID, should.Match(&buildbucketpb.BuilderID{
				Project: "test project",
				Bucket:  "test.bucket",
				Builder: "test builder",
			}))
		})

		t.Run("For invalid ID", func(t *ftt.Test) {
			builderID, err := ParseLegacyBuilderID("buildbucket/123456")
			assert.Loosely(t, err, should.Equal(ErrInvalidLegacyBuilderID))
			assert.Loosely(t, builderID, should.BeNil)
		})
	})

	ftt.Run("ParseBuilderID", t, func(t *ftt.Test) {
		t.Run("For valid ID", func(t *ftt.Test) {
			builderID, err := ParseBuilderID("test project/test bucket/test builder")
			assert.Loosely(t, builderID, should.Match(&buildbucketpb.BuilderID{
				Project: "test project",
				Bucket:  "test bucket",
				Builder: "test builder",
			}))
			assert.Loosely(t, err, should.BeNil)
		})

		t.Run("For invalid ID", func(t *ftt.Test) {
			builderID, err := ParseBuilderID("test project/123456")
			assert.Loosely(t, builderID, should.BeNil)
			assert.Loosely(t, err, should.Equal(ErrInvalidBuilderID))
		})
	})

	ftt.Run("ParseBuildbucketBuildID", t, func(t *ftt.Test) {
		t.Run("For valid build ID", func(t *ftt.Test) {
			bid, err := ParseBuildbucketBuildID("buildbucket/123456")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, bid, should.Equal(123456))
		})

		t.Run("For invalid build ID", func(t *ftt.Test) {
			bid, err := ParseBuildbucketBuildID("notbuildbucket/123456")
			assert.Loosely(t, err, should.Equal(ErrInvalidLegacyBuildID))
			assert.Loosely(t, bid, should.BeZero)
		})
	})

	ftt.Run("ParseLegacyBuildbucketBuildID", t, func(t *ftt.Test) {
		t.Run("For valid build ID", func(t *ftt.Test) {
			builderID, buildNum, err := ParseLegacyBuildbucketBuildID("buildbucket/luci.test project.test bucket/test builder/123456")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, builderID, should.Match(&buildbucketpb.BuilderID{
				Project: "test project",
				Bucket:  "test bucket",
				Builder: "test builder",
			}))
			assert.Loosely(t, buildNum, should.Equal(123456))
		})

		t.Run("Allow '.' in builder ID", func(t *ftt.Test) {
			builderID, buildNum, err := ParseLegacyBuildbucketBuildID("buildbucket/luci.test project.test bucket/test.builder/123456")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, builderID, should.Match(&buildbucketpb.BuilderID{
				Project: "test project",
				Bucket:  "test bucket",
				Builder: "test.builder",
			}))
			assert.Loosely(t, buildNum, should.Equal(123456))
		})

		t.Run("Allow '.' in bucket ID", func(t *ftt.Test) {
			builderID, buildNum, err := ParseLegacyBuildbucketBuildID("buildbucket/luci.test project.test.bucket/test builder/123456")
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, builderID, should.Match(&buildbucketpb.BuilderID{
				Project: "test project",
				Bucket:  "test.bucket",
				Builder: "test builder",
			}))
			assert.Loosely(t, buildNum, should.Equal(123456))
		})

		t.Run("For invalid build ID", func(t *ftt.Test) {
			builderID, buildNum, err := ParseLegacyBuildbucketBuildID("buildbucket/123456")
			assert.Loosely(t, err, should.Equal(ErrInvalidLegacyBuildID))
			assert.Loosely(t, builderID, should.BeNil)
			assert.Loosely(t, buildNum, should.BeZero)
		})
	})
}
