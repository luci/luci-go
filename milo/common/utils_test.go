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

package common

import (
	"context"
	"testing"

	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	. "github.com/smartystreets/goconvey/convey"
)

type TestStruct struct {
	Prop string
}

func TestTagGRPC(t *testing.T) {
	t.Parallel()

	Convey("GRPC tagging works", t, func() {
		c := memory.Use(context.Background())
		cUser := auth.WithState(c, &authtest.FakeState{Identity: "user:user@example.com"})
		cAnon := auth.WithState(c, &authtest.FakeState{Identity: identity.AnonymousIdentity})

		Convey("For not found errors", func() {
			errGRPCNotFound := status.Errorf(codes.NotFound, "not found")
			So(grpcutil.Code(TagGRPC(cAnon, errGRPCNotFound)), ShouldEqual, codes.Unauthenticated)
			So(grpcutil.Code(TagGRPC(cUser, errGRPCNotFound)), ShouldEqual, codes.NotFound)
		})

		Convey("For permission denied errors", func() {
			errGRPCPermissionDenied := status.Errorf(codes.PermissionDenied, "permission denied")
			So(grpcutil.Code(TagGRPC(cAnon, errGRPCPermissionDenied)), ShouldEqual, codes.Unauthenticated)
			So(grpcutil.Code(TagGRPC(cUser, errGRPCPermissionDenied)), ShouldEqual, codes.NotFound)
		})

		Convey("For invalid argument errors", func() {
			errGRPCInvalidArgument := status.Errorf(codes.InvalidArgument, "invalid argument")
			So(grpcutil.Code(TagGRPC(cAnon, errGRPCInvalidArgument)), ShouldEqual, codes.InvalidArgument)
			So(grpcutil.Code(TagGRPC(cUser, errGRPCInvalidArgument)), ShouldEqual, codes.InvalidArgument)
		})

		Convey("For invalid argument multi-errors", func() {
			errGRPCInvalidArgument := status.Errorf(codes.InvalidArgument, "invalid argument")
			errMulti := errors.NewMultiError(errGRPCInvalidArgument)
			So(grpcutil.Code(TagGRPC(cAnon, errMulti)), ShouldEqual, codes.InvalidArgument)
			So(grpcutil.Code(TagGRPC(cUser, errMulti)), ShouldEqual, codes.InvalidArgument)
		})
	})

	Convey("JSON(Un)MarshalCompressed works", t, func() {
		data := &TestStruct{Prop: "prop"}
		bytes, err := JSONMarshalCompressed(data)
		So(err, ShouldBeNil)

		var parsed TestStruct
		err = JSONUnmarshalCompressed(bytes, &parsed)
		So(err, ShouldBeNil)
		So(parsed.Prop, ShouldEqual, data.Prop)
	})

	Convey("ParseLegacyBuilderID", t, func() {
		Convey("For valid ID", func() {
			builderID, err := ParseLegacyBuilderID("buildbucket/luci.test project.test bucket/test builder")
			So(builderID, ShouldResemble, &buildbucketpb.BuilderID{
				Project: "test project",
				Bucket:  "test bucket",
				Builder: "test builder",
			})
			So(err, ShouldBeNil)
		})

		Convey("For invalid ID", func() {
			builderID, err := ParseLegacyBuilderID("buildbucket/123456")
			So(builderID, ShouldBeNil)
			So(err, ShouldEqual, ErrInvalidLegacyBuilderID)
		})
	})

	Convey("ParseLegacyBuildID", t, func() {
		Convey("For valid build ID", func() {
			builderID, buildNum, err := ParseLegacyBuildID("buildbucket/luci.test project.test bucket/test builder/123456")
			So(err, ShouldBeNil)
			So(builderID, ShouldResemble, &buildbucketpb.BuilderID{
				Project: "test project",
				Bucket:  "test bucket",
				Builder: "test builder",
			})
			So(buildNum, ShouldEqual, 123456)
		})

		Convey("For invalid build ID", func() {
			builderID, buildNum, err := ParseLegacyBuildID("buildbucket/123456")
			So(err, ShouldEqual, ErrInvalidLegacyBuildID)
			So(builderID, ShouldBeNil)
			So(buildNum, ShouldEqual, 0)
		})
	})
}
