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
	"strings"
	"testing"

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/genproto/protobuf/field_mask"

	"go.chromium.org/luci/common/clock/testclock"

	"go.chromium.org/luci/buildbucket/appengine/model"
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
				So(validateUpdate(req, nil), ShouldBeNil)
			})

			Convey("with empty path", func() {
				req.UpdateMask = &field_mask.FieldMask{}
				So(validateUpdate(req, nil), ShouldBeNil)
			})

			Convey("with valid paths", func() {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{
					"build.tags",
					"build.output",
					"build.status_details",
					"build.summary_markdown",
				}}
				req.Build.SummaryMarkdown = "this is a string"
				So(validateUpdate(req, nil), ShouldBeNil)
			})
		})

		Convey("fails", func() {
			Convey("with nil request", func() {
				So(validateUpdate(nil, nil), ShouldErrLike, "build.id: required")
			})

			Convey("with an invalid path", func() {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{
					"bucket.name",
				}}
				So(validateUpdate(req, nil), ShouldErrLike, `unsupported path "bucket.name"`)
			})

			Convey("with a mix of valid and invalid paths", func() {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{
					"build.tags",
					"bucket.name",
					"build.output",
				}}
				So(validateUpdate(req, nil), ShouldErrLike, `unsupported path "bucket.name"`)
			})
		})
	})

	Convey("validate BuildStatus", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.status"}}

		Convey("succeeds", func() {
			req.Build.Status = pb.Status_SUCCESS
			So(validateUpdate(req, nil), ShouldBeNil)
		})

		Convey("fails", func() {
			req.Build.Status = pb.Status_SCHEDULED
			So(validateUpdate(req, nil), ShouldErrLike, "build.status: invalid status SCHEDULED for UpdateBuild")
		})
	})

	Convey("validate BuildTags", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.tags"}}
		req.Build.Tags = []*pb.StringPair{{Key: "ci:builder", Value: ""}}
		So(validateUpdate(req, nil), ShouldErrLike, `tag key "ci:builder" cannot have a colon`)
	})

	Convey("validate SummaryMarkdown", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.summary_markdown"}}
		req.Build.SummaryMarkdown = strings.Repeat("☕", summaryMarkdownMaxLength)
		So(validateUpdate(req, nil), ShouldErrLike, "too big to accept")
	})

	Convey("validate with Commit", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.output.gitiles_commit"}}
		req.Build.Output = &pb.Build_Output{GitilesCommit: &pb.GitilesCommit{
			Project: "project",
			Host:    "host",
			Id:      "id",
		}}
		So(validateUpdate(req, nil), ShouldErrLike, "ref is required")
	})

	Convey("validate with Steps", t, func() {
		t, _ := ptypes.TimestampProto(testclock.TestRecentTimeUTC)
		bs := &model.BuildSteps{ID: 1}
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.steps"}}

		Convey("succeeds", func() {
			req.Build.Steps = []*pb.Step{
				{Name: "step1", Status: pb.Status_SUCCESS, StartTime: t, EndTime: t},
				{Name: "step2", Status: pb.Status_SUCCESS, StartTime: t, EndTime: t},
			}
			So(validateUpdate(req, bs), ShouldBeNil)
		})

		Convey("fails with duplicates", func() {
			t, _ := ptypes.TimestampProto(testclock.TestRecentTimeUTC)
			req.Build.Steps = []*pb.Step{
				{Name: "step1", Status: pb.Status_SUCCESS, StartTime: t, EndTime: t},
				{Name: "step1", Status: pb.Status_SUCCESS, StartTime: t, EndTime: t},
			}
			So(validateUpdate(req, bs), ShouldErrLike, `duplicate: "step1"`)
		})
	})
}

func TestValidateStep(t *testing.T) {
	t.Parallel()

	Convey("validate", t, func() {
		t, _ := ptypes.TimestampProto(testclock.TestRecentTimeUTC)
		step := &pb.Step{Name: "step1"}

		Convey("with status unspecified", func() {
			step.Status = pb.Status_STATUS_UNSPECIFIED
			So(validateStep(step), ShouldErrLike, "status: is unspecified or unknown")
		})

		Convey("with status ENDED_MASK", func() {
			step.Status = pb.Status_ENDED_MASK
			So(validateStep(step), ShouldErrLike, "status: must not be ENDED_MASK")
		})

		Convey("with non-terminal status", func() {
			Convey("without start_time, when should have", func() {
				step.Status = pb.Status_STARTED
				So(validateStep(step), ShouldErrLike, `start_time: required by status "STARTED"`)
			})

			Convey("with start_time, when should not have", func() {
				step.Status = pb.Status_SCHEDULED
				step.StartTime = t
				So(validateStep(step), ShouldErrLike, `start_time: must not be specified for status "SCHEDULED"`)
			})

		})

		Convey("with terminal status", func() {
			step.Status = pb.Status_INFRA_FAILURE

			Convey("missing start_time, but end_time", func() {
				step.EndTime = t
				So(validateStep(step), ShouldBeNil)
			})

			Convey("missing end_time", func() {
				step.StartTime = t
				So(validateStep(step), ShouldErrLike, "end_time: must have both or neither end_time and a terminal status")
			})

			Convey("end_time is before start_time", func() {
				step.EndTime = t
				st, _ := ptypes.TimestampProto(testclock.TestRecentTimeUTC.AddDate(0, 0, 1))
				step.StartTime = st
				So(validateStep(step), ShouldErrLike, "start_time: is after the end_time")
			})
		})

		Convey("with logs", func() {
			step.Status = pb.Status_STARTED
			step.StartTime = t

			Convey("missing name", func() {
				step.Logs = []*pb.Log{{Url: "url", ViewUrl: "view_url"}}
				So(validateStep(step), ShouldErrLike, "logs[0].name: required")
			})

			Convey("missing url", func() {
				step.Logs = []*pb.Log{{Name: "name", ViewUrl: "view_url"}}
				So(validateStep(step), ShouldErrLike, "logs[0].url: required")
			})

			Convey("missing view_url", func() {
				step.Logs = []*pb.Log{{Name: "name", Url: "url"}}
				So(validateStep(step), ShouldErrLike, "logs[0].view_url: required")
			})

			Convey("duplicate name", func() {
				step.Logs = []*pb.Log{
					{Name: "name", Url: "url", ViewUrl: "view_url"},
					{Name: "name", Url: "url", ViewUrl: "view_url"},
				}
				So(validateStep(step), ShouldErrLike, `logs[1].name: duplicate: "name"`)
			})
		})
	})
}
