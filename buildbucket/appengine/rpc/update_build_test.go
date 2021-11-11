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

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/tq"

	// TODO(crbug/1242998): Remove once safe get becomes datastore default.
	_ "go.chromium.org/luci/gae/service/datastore/crbug1242998safeget"

	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/internal/perm"
	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

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

	Convey("validate status", t, func() {
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

	Convey("validate tags", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.tags"}}
		req.Build.Tags = []*pb.StringPair{{Key: "ci:builder", Value: ""}}
		So(validateUpdate(req, nil), ShouldErrLike, `tag key "ci:builder" cannot have a colon`)
	})

	Convey("validate summary_markdown", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.summary_markdown"}}
		req.Build.SummaryMarkdown = strings.Repeat("☕", summaryMarkdownMaxLength)
		So(validateUpdate(req, nil), ShouldErrLike, "too big to accept")
	})

	Convey("validate output.gitiles_ommit", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.output.gitiles_commit"}}
		req.Build.Output = &pb.Build_Output{GitilesCommit: &pb.GitilesCommit{
			Project: "project",
			Host:    "host",
			Id:      "id",
		}}
		So(validateUpdate(req, nil), ShouldErrLike, "ref is required")
	})

	Convey("validate output.properties", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.output.properties"}}

		Convey("succeeds", func() {
			props, _ := structpb.NewStruct(map[string]interface{}{"key": "value"})
			req.Build.Output = &pb.Build_Output{Properties: props}
			So(validateUpdate(req, nil), ShouldBeNil)
		})

		Convey("fails", func() {
			props, _ := structpb.NewStruct(map[string]interface{}{"key": nil})
			req.Build.Output = &pb.Build_Output{Properties: props}
			So(validateUpdate(req, nil), ShouldErrLike, "value is not set")
		})
	})

	Convey("validate steps", t, func() {
		t := timestamppb.New(testclock.TestRecentTimeUTC)
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
			t := timestamppb.New(testclock.TestRecentTimeUTC)
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
		t := timestamppb.New(testclock.TestRecentTimeUTC)
		step := &pb.Step{Name: "step1"}
		bStatus := pb.Status_STARTED

		Convey("with status unspecified", func() {
			step.Status = pb.Status_STATUS_UNSPECIFIED
			So(validateStep(step, nil, bStatus), ShouldErrLike, "status: is unspecified or unknown")
		})

		Convey("with status ENDED_MASK", func() {
			step.Status = pb.Status_ENDED_MASK
			So(validateStep(step, nil, bStatus), ShouldErrLike, "status: must not be ENDED_MASK")
		})

		Convey("with non-terminal status", func() {
			Convey("without start_time, when should have", func() {
				step.Status = pb.Status_STARTED
				So(validateStep(step, nil, bStatus), ShouldErrLike, `start_time: required by status "STARTED"`)
			})

			Convey("with start_time, when should not have", func() {
				step.Status = pb.Status_SCHEDULED
				step.StartTime = t
				So(validateStep(step, nil, bStatus), ShouldErrLike, `start_time: must not be specified for status "SCHEDULED"`)
			})

			Convey("with terminal build status", func() {
				bStatus = pb.Status_SUCCESS
				step.Status = pb.Status_STARTED
				So(validateStep(step, nil, bStatus), ShouldErrLike, `status: cannot be "STARTED" because the build has a terminal status "SUCCESS"`)
			})

		})

		Convey("with terminal status", func() {
			step.Status = pb.Status_INFRA_FAILURE

			Convey("missing start_time, but end_time", func() {
				step.EndTime = t
				So(validateStep(step, nil, bStatus), ShouldErrLike, `start_time: required by status "INFRA_FAILURE"`)
			})

			Convey("missing end_time", func() {
				step.StartTime = t
				So(validateStep(step, nil, bStatus), ShouldErrLike, "end_time: must have both or neither end_time and a terminal status")
			})

			Convey("end_time is before start_time", func() {
				step.EndTime = t
				st := timestamppb.New(testclock.TestRecentTimeUTC.AddDate(0, 0, 1))
				step.StartTime = st
				So(validateStep(step, nil, bStatus), ShouldErrLike, "end_time: is before the start_time")
			})
		})

		Convey("with logs", func() {
			step.Status = pb.Status_STARTED
			step.StartTime = t

			Convey("missing name", func() {
				step.Logs = []*pb.Log{{Url: "url", ViewUrl: "view_url"}}
				So(validateStep(step, nil, bStatus), ShouldErrLike, "logs[0].name: required")
			})

			Convey("missing url", func() {
				step.Logs = []*pb.Log{{Name: "name", ViewUrl: "view_url"}}
				So(validateStep(step, nil, bStatus), ShouldErrLike, "logs[0].url: required")
			})

			Convey("missing view_url", func() {
				step.Logs = []*pb.Log{{Name: "name", Url: "url"}}
				So(validateStep(step, nil, bStatus), ShouldErrLike, "logs[0].view_url: required")
			})

			Convey("duplicate name", func() {
				step.Logs = []*pb.Log{
					{Name: "name", Url: "url", ViewUrl: "view_url"},
					{Name: "name", Url: "url", ViewUrl: "view_url"},
				}
				So(validateStep(step, nil, bStatus), ShouldErrLike, `logs[1].name: duplicate: "name"`)
			})
		})
	})
}

func TestGetBuildForUpdate(t *testing.T) {
	t.Parallel()
	updateMask := func(req *pb.UpdateBuildRequest) *mask.Mask {
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
			Proto: &pb.Build{
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
			b, err := getBuildForUpdate(ctx, updateMask(req), req)
			So(err, ShouldBeNil)
			So(b.Proto, ShouldResembleProto, build.Proto)

			Convey("with build.steps", func() {
				req.Build.Status = pb.Status_STARTED
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.status", "build.steps"}}
				_, err = getBuildForUpdate(ctx, updateMask(req), req)
				So(err, ShouldBeNil)
			})
			Convey("with build.output", func() {
				req.Build.Status = pb.Status_STARTED
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.status", "build.output"}}
				_, err = getBuildForUpdate(ctx, updateMask(req), req)
				So(err, ShouldBeNil)
			})
		})

		Convey("fails", func() {
			Convey("if build doesn't exist", func() {
				req.Build.Id = 2
				_, err := getBuildForUpdate(ctx, updateMask(req), req)
				So(err, ShouldBeRPCNotFound)
			})

			Convey("if ended", func() {
				build.Proto.Status = pb.Status_SUCCESS
				So(datastore.Put(ctx, build), ShouldBeNil)
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.status"}}
				_, err := getBuildForUpdate(ctx, updateMask(req), req)
				So(err, ShouldBeRPCFailedPrecondition, "cannot update an ended build")
			})

			Convey("with build.steps", func() {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.steps"}}
				_, err := getBuildForUpdate(ctx, updateMask(req), req)
				So(err, ShouldBeRPCInvalidArgument, "cannot update steps of a SCHEDULED build")
			})
			Convey("with build.output", func() {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.output.properties"}}
				_, err := getBuildForUpdate(ctx, updateMask(req), req)
				So(err, ShouldBeRPCInvalidArgument, "cannot update build output fields of a SCHEDULED build")
			})
		})
	})
}

func TestUpdateBuild(t *testing.T) {
	t.Parallel()

	tk := "a token"
	getBuildWithDetails := func(ctx context.Context, bid int64) *model.Build {
		b, err := getBuild(ctx, bid)
		So(err, ShouldBeNil)
		// ensure that the below fields were cleared when the build was saved.
		So(b.Proto.Tags, ShouldBeNil)
		So(b.Proto.Steps, ShouldBeNil)
		if b.Proto.Output != nil {
			So(b.Proto.Output.Properties, ShouldBeNil)
		}
		m := model.HardcodedBuildMask("output.properties", "steps", "tags")
		So(model.LoadBuildDetails(ctx, m, b.Proto), ShouldBeNil)
		return b
	}

	Convey("UpdateBuild", t, func() {
		srv := &Builds{}
		s := &authtest.FakeState{
			Identity: "user:user",
			FakeDB: authtest.NewFakeDB(
				authtest.MockMembership("user:user", perm.UpdateBuildAllowedUsers),
			),
		}
		ctx := auth.WithState(memory.Use(context.Background()), s)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(BuildTokenKey, tk))
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, _ = tsmon.WithDummyInMemory(ctx)
		store := tsmon.Store(ctx)
		ctx = txndefer.FilterRDS(ctx)
		ctx, sch := tq.TestingContext(ctx, nil)

		t0 := testclock.TestRecentTimeUTC
		ctx, tclock := testclock.UseTime(ctx, t0)

		// helper function to call UpdateBuild.
		updateBuild := func(ctx context.Context, req *pb.UpdateBuildRequest) error {
			_, err := srv.UpdateBuild(ctx, req)
			return err
		}

		// create and save a sample build in the datastore
		build := &model.Build{
			ID: 1,
			Proto: &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_STARTED,
			},
			CreateTime:  t0,
			UpdateToken: tk,
		}
		So(datastore.Put(ctx, build), ShouldBeNil)
		req := &pb.UpdateBuildRequest{
			Build: &pb.Build{Id: 1, SummaryMarkdown: "summary"},
			UpdateMask: &field_mask.FieldMask{Paths: []string{
				"build.summary_markdown",
			}},
		}

		Convey("permission deined, if sender is not in updater group", func() {
			s.Identity = "anonymous:anonymous"
			So(updateBuild(ctx, req), ShouldHaveRPCCode, codes.PermissionDenied)
		})

		Convey("open mask, empty request", func() {
			validMasks := []struct {
				name string
				err  string
			}{
				{"build.output", ""},
				{"build.output.properties", ""},
				{"build.status", "invalid status STATUS_UNSPECIFIED"},
				{"build.status_details", ""},
				{"build.steps", ""},
				{"build.summary_markdown", ""},
				{"build.tags", ""},
				{"build.output.gitiles_commit", "ref is required"},
			}
			for _, test := range validMasks {
				Convey(test.name, func() {
					req.UpdateMask.Paths[0] = test.name
					err := updateBuild(ctx, req)
					if test.err == "" {
						So(err, ShouldBeNil)
					} else {
						So(err, ShouldErrLike, test.err)
					}
				})
			}
		})

		Convey("build.update_time is always updated", func() {
			So(updateBuild(ctx, req), ShouldBeNil)
			b, err := getBuild(ctx, req.Build.Id)
			So(err, ShouldBeNil)
			So(b.Proto.UpdateTime, ShouldResembleProto, timestamppb.New(t0))

			tclock.Add(time.Second)

			So(updateBuild(ctx, req), ShouldBeNil)
			b, err = getBuild(ctx, req.Build.Id)
			So(err, ShouldBeNil)
			So(b.Proto.UpdateTime, ShouldResembleProto, timestamppb.New(t0.Add(time.Second)))
		})

		Convey("build.output.properties", func() {
			props, err := structpb.NewStruct(map[string]interface{}{"key": "value"})
			So(err, ShouldBeNil)
			req.Build.Output = &pb.Build_Output{Properties: props}

			Convey("with mask", func() {
				req.UpdateMask.Paths[0] = "build.output.properties"
				So(updateBuild(ctx, req), ShouldBeRPCOK)
				b := getBuildWithDetails(ctx, req.Build.Id)
				So(b.Proto.Output.Properties, ShouldResembleProtoJSON, `{"key": "value"}`)
			})

			Convey("without mask", func() {
				So(updateBuild(ctx, req), ShouldBeRPCOK)
				b := getBuildWithDetails(ctx, req.Build.Id)
				So(b.Proto.Output.Properties, ShouldResembleProtoJSON, `{}`)
			})

		})

		Convey("build.steps", func() {
			step := &pb.Step{
				Name:      "step",
				StartTime: &timestamppb.Timestamp{Seconds: 1},
				EndTime:   &timestamppb.Timestamp{Seconds: 12},
				Status:    pb.Status_SUCCESS,
			}
			req.Build.Steps = []*pb.Step{step}

			Convey("with mask", func() {
				req.UpdateMask.Paths[0] = "build.steps"
				So(updateBuild(ctx, req), ShouldBeRPCOK)
				b := getBuildWithDetails(ctx, req.Build.Id)
				So(b.Proto.Steps[0], ShouldResembleProto, step)
			})

			Convey("without mask", func() {
				So(updateBuild(ctx, req), ShouldBeRPCOK)
				b := getBuildWithDetails(ctx, req.Build.Id)
				So(b.Proto.Steps, ShouldBeNil)
			})

			Convey("incomplete steps with non-terminal Build status", func() {
				req.UpdateMask.Paths = []string{"build.status", "build.steps"}
				req.Build.Status = pb.Status_STARTED
				req.Build.Steps[0].Status = pb.Status_STARTED
				req.Build.Steps[0].EndTime = nil
				So(updateBuild(ctx, req), ShouldBeRPCOK)
			})

			Convey("incomplete steps with terminal Build status", func() {
				req.UpdateMask.Paths = []string{"build.status", "build.steps"}
				req.Build.Status = pb.Status_SUCCESS

				Convey("with mask", func() {
					req.Build.Steps[0].Status = pb.Status_STARTED
					req.Build.Steps[0].EndTime = nil

					// Should be rejected.
					msg := `cannot be "STARTED" because the build has a terminal status "SUCCESS"`
					So(updateBuild(ctx, req), ShouldHaveRPCCode, codes.InvalidArgument, msg)
				})

				Convey("w/o mask", func() {
					// update the build with incomplete steps first.
					req.Build.Status = pb.Status_STARTED
					req.Build.Steps[0].Status = pb.Status_STARTED
					req.Build.Steps[0].EndTime = nil
					So(updateBuild(ctx, req), ShouldBeRPCOK)

					// update the build again with a terminal status, but w/o step mask.
					req.UpdateMask.Paths = []string{"build.status"}
					req.Build.Status = pb.Status_SUCCESS
					So(updateBuild(ctx, req), ShouldBeRPCOK)

					// the step should have been cancelled.
					b := getBuildWithDetails(ctx, req.Build.Id)
					expected := &pb.Step{
						Name:      step.Name,
						Status:    pb.Status_CANCELED,
						StartTime: step.StartTime,
						EndTime:   timestamppb.New(t0),
					}
					So(b.Proto.Steps[0], ShouldResembleProto, expected)
				})
			})
		})

		Convey("build.tags", func() {
			tag := &pb.StringPair{Key: "resultdb", Value: "disabled"}
			req.Build.Tags = []*pb.StringPair{tag}

			Convey("with mask", func() {
				req.UpdateMask.Paths[0] = "build.tags"
				So(updateBuild(ctx, req), ShouldBeRPCOK)

				b := getBuildWithDetails(ctx, req.Build.Id)
				expected := []string{strpair.Format("resultdb", "disabled")}
				So(b.Tags, ShouldResemble, expected)

				// change the value and update it again
				tag.Value = "enabled"
				So(updateBuild(ctx, req), ShouldBeRPCOK)

				// both tags should exist
				b = getBuildWithDetails(ctx, req.Build.Id)
				expected = append(expected, strpair.Format("resultdb", "enabled"))
				So(b.Tags, ShouldResemble, expected)
			})

			Convey("without mask", func() {
				So(updateBuild(ctx, req), ShouldBeRPCOK)
				b := getBuildWithDetails(ctx, req.Build.Id)
				So(b.Tags, ShouldBeNil)
			})
		})

		Convey("build-start event", func() {
			Convey("Status_STARTED w/o status change", func() {
				req.UpdateMask.Paths[0] = "build.status"
				req.Build.Status = pb.Status_STARTED
				So(updateBuild(ctx, req), ShouldBeRPCOK)

				// no TQ tasks should be scheduled.
				So(sch.Tasks(), ShouldBeEmpty)

				// no metric update, either.
				So(store.Get(ctx, metrics.V1.BuildCountStarted, time.Time{}, fv(false)), ShouldEqual, nil)
			})

			Convey("Status_STARTED w/ status change", func() {
				// create a sample task with SCHEDULED.
				build.Proto.Id++
				build.ID++
				build.Proto.Status, build.Status = pb.Status_SCHEDULED, pb.Status_SCHEDULED
				So(datastore.Put(ctx, build), ShouldBeNil)

				// update it with STARTED
				req.Build.Id = build.ID
				req.UpdateMask.Paths[0] = "build.status"
				req.Build.Status = pb.Status_STARTED
				So(updateBuild(ctx, req), ShouldBeRPCOK)

				// TQ task for pubsub-notification.
				tasks := sch.Tasks()
				So(tasks, ShouldHaveLength, 1)
				So(tasks[0].Payload.(*taskdefs.NotifyPubSub).GetBuildId(), ShouldEqual, build.ID)

				// BuildStarted metric should be set 1.
				So(store.Get(ctx, metrics.V1.BuildCountStarted, time.Time{}, fv(false)), ShouldEqual, 1)
			})
		})

		Convey("build-completion event", func() {
			Convey("Status_SUCCESSS w/o status change", func() {
				req.UpdateMask.Paths[0] = "build.status"
				req.Build.Status = pb.Status_SUCCESS
				So(updateBuild(ctx, req), ShouldBeRPCOK)

				// TQ tasks for pubsub-notification, bq-export, and invocation-finalization.
				tasks := sch.Tasks()
				So(tasks, ShouldHaveLength, 3)
				sum := 0
				for _, task := range tasks {
					switch v := task.Payload.(type) {
					case *taskdefs.NotifyPubSub:
						sum++
						So(v.GetBuildId(), ShouldEqual, req.Build.Id)
					case *taskdefs.ExportBigQuery:
						sum += 2
						So(v.GetBuildId(), ShouldEqual, req.Build.Id)
					case *taskdefs.FinalizeResultDB:
						sum += 4
						So(v.GetBuildId(), ShouldEqual, req.Build.Id)
					default:
						panic("invalid task payload")
					}
				}
				So(sum, ShouldEqual, 7)

				// BuildCompleted metric should be set to 1 with SUCCESS.
				fvs := fv(model.Success.String(), "", "", false)
				So(store.Get(ctx, metrics.V1.BuildCountCompleted, time.Time{}, fvs), ShouldEqual, 1)
			})
		})
	})
}
