// Copyright 2020 The LUCI Authors.
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

	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateUpdate(t *testing.T) {
	t.Parallel()

	Convey("validate UpdateMask", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}

		Convey("succeeds", func() {
			Convey("with nil mask", func() {
				So(validateUpdate(req), ShouldBeNil)
			})

			Convey("with empty path", func() {
				req.UpdateMask = &field_mask.FieldMask{}
				So(validateUpdate(req), ShouldBeNil)
			})

			Convey("with valid paths", func() {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{
					"build.tags",
					"build.output",
					"build.status_details",
					"build.output.gitiles_commit",
					"build.summary_markdown",
				}}
				So(validateUpdate(req), ShouldBeNil)
			})
		})

		Convey("fails", func() {
			Convey("with nil request", func() {
				So(validateUpdate(nil), ShouldErrLike, "build.id: required")
			})

			Convey("with an invalid path", func() {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{
					"bucket.name",
				}}
				So(validateUpdate(req), ShouldErrLike, `unsupported path "bucket.name"`)
			})

			Convey("with a mix of valid and invalid paths", func() {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{
					"build.tags",
					"bucket.name",
					"build.output",
				}}
				So(validateUpdate(req), ShouldErrLike, `unsupported path "bucket.name"`)
			})
		})
	})

	Convey("validate BuildStatus", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.status"}}

		Convey("succeeds", func() {
			req.Build.Status = pb.Status_SUCCESS
			So(validateUpdate(req), ShouldBeNil)
		})

		Convey("fails", func() {
			req.Build.Status = pb.Status_SCHEDULED
			So(validateUpdate(req), ShouldErrLike, "build.status: invalid status SCHEDULED for UpdateBuild")
		})
	})

	Convey("validate BuildTags", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.tags"}}
		req.Build.Tags = []*pb.StringPair{{Key: "k", Value: "v"}}
		So(validateUpdate(req), ShouldBeNil)
	})
}
