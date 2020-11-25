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
	"context"
	"strings"
	"testing"
	"time"

	"github.com/golang/protobuf/ptypes"
	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"

	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func addTS(ts *timestamppb.Timestamp, d time.Duration) *timestamppb.Timestamp {
	t, _ := ptypes.Timestamp(ts)
	ts, _ = ptypes.TimestampProto(t.Add(d))
	return ts
}

func TestValidateUpdate(t *testing.T) {
	t.Parallel()

	Convey("validate UpdateMask", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}

		Convey("succeeds", func() {
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
			Convey("with nil mask", func() {
				So(validateUpdate(req, nil), ShouldErrLike, "build.update_mask: required")
			})

			Convey("without paths", func() {
				req.UpdateMask = &field_mask.FieldMask{}
				So(validateUpdate(req, nil), ShouldErrLike, "build.update_mask: required")
			})

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

		Convey("with a parent step", func() {
			Convey("before child", func() {
				req.Build.Steps = []*pb.Step{
					{Name: "parent", Status: pb.Status_SUCCESS, StartTime: t, EndTime: t},
					{Name: "parent|child", Status: pb.Status_SUCCESS, StartTime: t, EndTime: t},
				}
				So(validateUpdate(req, bs), ShouldBeNil)
			})
			Convey("after child", func() {
				req.Build.Steps = []*pb.Step{
					{Name: "parent|child", Status: pb.Status_SUCCESS, StartTime: t, EndTime: t},
					{Name: "parent", Status: pb.Status_SUCCESS, StartTime: t, EndTime: t},
				}
				So(validateUpdate(req, bs), ShouldErrLike, `parent of "parent|child" must precede`)
			})
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
			So(validateStep(step, nil), ShouldErrLike, "status: is unspecified or unknown")
		})

		Convey("with status ENDED_MASK", func() {
			step.Status = pb.Status_ENDED_MASK
			So(validateStep(step, nil), ShouldErrLike, "status: must not be ENDED_MASK")
		})

		Convey("with non-terminal status", func() {
			Convey("without start_time, when should have", func() {
				step.Status = pb.Status_STARTED
				So(validateStep(step, nil), ShouldErrLike, `start_time: required by status "STARTED"`)
			})

			Convey("with start_time, when should not have", func() {
				step.Status = pb.Status_SCHEDULED
				step.StartTime = t
				So(validateStep(step, nil), ShouldErrLike, `start_time: must not be specified for status "SCHEDULED"`)
			})

		})

		Convey("with terminal status", func() {
			step.Status = pb.Status_INFRA_FAILURE

			Convey("missing start_time, but end_time", func() {
				step.EndTime = t
				So(validateStep(step, nil), ShouldErrLike, `start_time: required by status "INFRA_FAILURE"`)
			})

			Convey("missing end_time", func() {
				step.StartTime = t
				So(validateStep(step, nil), ShouldErrLike, "end_time: must have both or neither end_time and a terminal status")
			})

			Convey("end_time is before start_time", func() {
				step.EndTime = t
				st, _ := ptypes.TimestampProto(testclock.TestRecentTimeUTC.AddDate(0, 0, 1))
				step.StartTime = st
				So(validateStep(step, nil), ShouldErrLike, "start_time: is after the end_time")
			})
		})

		Convey("with logs", func() {
			step.Status = pb.Status_STARTED
			step.StartTime = t

			Convey("missing name", func() {
				step.Logs = []*pb.Log{{Url: "url", ViewUrl: "view_url"}}
				So(validateStep(step, nil), ShouldErrLike, "logs[0].name: required")
			})

			Convey("missing url", func() {
				step.Logs = []*pb.Log{{Name: "name", ViewUrl: "view_url"}}
				So(validateStep(step, nil), ShouldErrLike, "logs[0].url: required")
			})

			Convey("missing view_url", func() {
				step.Logs = []*pb.Log{{Name: "name", Url: "url"}}
				So(validateStep(step, nil), ShouldErrLike, "logs[0].view_url: required")
			})

			Convey("duplicate name", func() {
				step.Logs = []*pb.Log{
					{Name: "name", Url: "url", ViewUrl: "view_url"},
					{Name: "name", Url: "url", ViewUrl: "view_url"},
				}
				So(validateStep(step, nil), ShouldErrLike, `logs[1].name: duplicate: "name"`)
			})
		})

		Convey("with a parent step", func() {
			parent := &pb.Step{Name: "step1"}
			setST := func(s, ps pb.Status) { step.Status, parent.Status = s, ps }
			setTS := func(s, e, ps, pe *timestamppb.Timestamp) {
				step.StartTime, step.EndTime = s, e
				parent.StartTime, parent.EndTime = ps, pe
			}

			Convey("parent status is SCHEDULED", func() {
				setST(pb.Status_STARTED, pb.Status_SCHEDULED)
				setTS(t, nil, t, nil)
				So(validateStep(step, parent), ShouldErrLike, `parent "step1" must be at least STARTED`)
			})

			Convey("child status is STARTED, but parent status is not", func() {
				setST(pb.Status_STARTED, pb.Status_SUCCESS)
				setTS(t, nil, t, t)
				So(validateStep(step, parent), ShouldErrLike, "the parent status must be STARTED")
			})

			Convey("child status is better", func() {
				setST(pb.Status_SUCCESS, pb.Status_FAILURE)
				setTS(t, t, t, t)
				So(validateStep(step, parent), ShouldBeNil)
			})

			Convey("with start_time", func() {
				Convey("parent missing start_time", func() {
					setST(pb.Status_SUCCESS, pb.Status_INFRA_FAILURE)
					setTS(t, t, nil, t)
					So(validateStep(step, parent), ShouldErrLike, "parent's start_time not specified")
				})

				Convey("preceding to parent.start_time", func() {
					setST(pb.Status_STARTED, pb.Status_STARTED)
					setTS(t, nil, addTS(t, time.Second), nil)
					So(validateStep(step, parent), ShouldErrLike, "cannot precede parent's")
				})

				Convey("following parent.end_time", func() {
					setST(pb.Status_FAILURE, pb.Status_CANCELED)
					setTS(addTS(t, time.Minute), addTS(t, time.Hour), t, addTS(t, time.Second))
					So(validateStep(step, parent), ShouldErrLike, "cannot follow parent's")
				})
			})

			Convey("with end_time", func() {
				Convey("preceding to parent.start_time", func() {
					setST(pb.Status_SUCCESS, pb.Status_FAILURE)
					setTS(addTS(t, -time.Hour), addTS(t, -time.Minute), t, t)
					So(validateStep(step, parent), ShouldErrLike, "cannot precede parent's")
				})

				Convey("following parent.end_time", func() {
					setST(pb.Status_SUCCESS, pb.Status_FAILURE)
					setTS(t, addTS(t, time.Hour), t, t)
					So(validateStep(step, parent), ShouldErrLike, "cannot follow parent's")
				})
			})
		})
	})
}

func TestGetBuildForUpdate(t *testing.T) {
	t.Parallel()
	buildMask := func(req *pb.UpdateBuildRequest) mask.Mask {
		fm, err := mask.FromFieldMask(req.UpdateMask, req, false, true)
		So(err, ShouldBeNil)
		return fm.MustSubmask("build")
	}

	Convey("getBuildForUpdate", t, func() {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		build := &model.Build{
			ID: 1,
			Proto: pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_SCHEDULED,
			},
			CreateTime: testclock.TestRecentTimeUTC,
		}
		So(datastore.Put(ctx, build), ShouldBeNil)
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}

		Convey("works", func() {
			b, err := getBuildForUpdate(ctx, buildMask(req), req)
			So(err, ShouldBeNil)
			So(&b.Proto, ShouldResembleProto, &build.Proto)

			Convey("with build.steps", func() {
				req.Build.Status = pb.Status_STARTED
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.status", "build.steps"}}
				_, err = getBuildForUpdate(ctx, buildMask(req), req)
				So(err, ShouldBeNil)
			})
			Convey("with build.output", func() {
				req.Build.Status = pb.Status_STARTED
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.status", "build.output"}}
				_, err = getBuildForUpdate(ctx, buildMask(req), req)
				So(err, ShouldBeNil)
			})
		})

		Convey("fails", func() {
			Convey("if build doesn't exist", func() {
				req.Build.Id = 2
				_, err := getBuildForUpdate(ctx, buildMask(req), req)
				So(err, ShouldBeRPCNotFound)
			})

			Convey("if ended", func() {
				build.Proto.Status = pb.Status_SUCCESS
				So(datastore.Put(ctx, build), ShouldBeNil)
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.status"}}
				_, err := getBuildForUpdate(ctx, buildMask(req), req)
				So(err, ShouldBeRPCFailedPrecondition, "cannot update an ended build")
			})

			Convey("with build.steps", func() {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.steps"}}
				_, err := getBuildForUpdate(ctx, buildMask(req), req)
				So(err, ShouldBeRPCInvalidArgument, "cannot update steps of a SCHEDULED build")
			})
			Convey("with build.output", func() {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.output.properties"}}
				_, err := getBuildForUpdate(ctx, buildMask(req), req)
				So(err, ShouldBeRPCInvalidArgument, "cannot update build output fields of a SCHEDULED build")
			})
		})
	})
}

func TestUpdateBuild(t *testing.T) {
	t.Parallel()

	Convey("UpdateBuild", t, func() {
		srv := &Builds{}
		s := &authtest.FakeState{
			Identity: "user:user",
			FakeDB: authtest.NewFakeDB(
				authtest.MockMembership("user:user", perm.UpdateBuildAllowedUsers),
			),
		}
		ctx := auth.WithState(memory.Use(context.Background()), s)
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}

		Convey("permission deined, if sender is not in updater group", func() {
			s.Identity = "anonymous:anonymous"
			_, err := srv.UpdateBuild(ctx, req)
			So(err, ShouldHaveRPCCode, codes.PermissionDenied)
		})
	})
}
