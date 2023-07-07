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

	"google.golang.org/grpc/codes"

	pb "go.chromium.org/luci/config_service/proto"
	"go.chromium.org/luci/config_service/testutil"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestGetConfig(t *testing.T) {
	t.Parallel()

	Convey("validateGetConfig", t, func() {
		Convey("ok", func() {
			So(validateGetConfig(&pb.GetConfigRequest{ContentSha256: "sha256"}), ShouldBeNil)

			So(validateGetConfig(&pb.GetConfigRequest{
				ConfigSet: "services/abc",
				Path:      "path",
			}), ShouldBeNil)
		})

		Convey("invalid", func() {
			Convey("empty", func() {
				err := validateGetConfig(&pb.GetConfigRequest{})
				So(err, ShouldErrLike, "one of content_sha256 or (config_set and path) is required")
			})

			Convey("config_set", func() {
				err := validateGetConfig(&pb.GetConfigRequest{
					ConfigSet: "services/abc",
				})
				So(err, ShouldErrLike, "one of content_sha256 or (config_set and path) is required")

				err = validateGetConfig(&pb.GetConfigRequest{
					ConfigSet: "random/abc",
					Path:      "path",
				})
				So(err, ShouldErrLike, `config_set "random/abc": unknown domain "random" for config set "random/abc"; currently supported domains [projects, services]`)

				err = validateGetConfig(&pb.GetConfigRequest{
					ConfigSet: "services/a$c",
					Path:      "path",
				})
				So(err, ShouldErrLike, `config_set "services/a$c": invalid service name "a$c", expected to match "[a-z0-9\\-_]+"`)

				err = validateGetConfig(&pb.GetConfigRequest{
					ConfigSet: "projects/_abc",
					Path:      "path",
				})
				So(err, ShouldErrLike, `config_set "projects/_abc": invalid project name: must begin with a letter`)
			})

			Convey("path", func() {
				err := validateGetConfig(&pb.GetConfigRequest{
					ConfigSet: "services/abc",
					Path:      "/path",
				})
				So(err, ShouldErrLike, `path "/path": must not be absolute`)

				err = validateGetConfig(&pb.GetConfigRequest{
					ConfigSet: "services/abc",
					Path:      "./path",
				})
				So(err, ShouldErrLike, `path "./path": should not start with './' or '../'`)
			})
		})
	})

	Convey("GetConfig", t, func() {
		ctx := testutil.SetupContext()
		srv := &Configs{}
		Convey("invalid req", func() {
			res, err := srv.GetConfig(ctx, &pb.GetConfigRequest{})
			So(res, ShouldBeNil)
			So(err, ShouldHaveGRPCStatus, codes.InvalidArgument, "one of content_sha256 or (config_set and path) is required")
		})
	})
}
