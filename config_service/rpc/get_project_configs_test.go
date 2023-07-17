// Copyright 2023 The LUCI Authors.
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
	"testing"

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"

	pb "go.chromium.org/luci/config_service/proto"
	"go.chromium.org/luci/config_service/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetProjectConfigs(t *testing.T) {
	t.Parallel()

	Convey("GetProjectConfigs", t, func() {
		ctx := testutil.SetupContext()
		srv := &Configs{}

		Convey("invalid path", func() {
			res, err := srv.GetProjectConfigs(ctx, &pb.GetProjectConfigsRequest{})
			So(res, ShouldBeNil)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, `invalid path - "": not specified`)

			res, err = srv.GetProjectConfigs(ctx, &pb.GetProjectConfigsRequest{Path: "/file"})
			So(res, ShouldBeNil)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, `invalid path - "/file": must not be absolute`)

			res, err = srv.GetProjectConfigs(ctx, &pb.GetProjectConfigsRequest{Path: "./file"})
			So(res, ShouldBeNil)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, `invalid path - "./file": should not start with './' or '../'`)
		})

		Convey("invalid mask", func() {
			res, err := srv.GetProjectConfigs(ctx, &pb.GetProjectConfigsRequest{
				Path: "file",
				Fields: &field_mask.FieldMask{
					Paths: []string{"random"},
				},
			})
			So(res, ShouldBeNil)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, `invalid fields mask: field "random" does not exist in message Config`)
		})
	})
}
