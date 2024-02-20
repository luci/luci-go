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
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"google.golang.org/genproto/protobuf/field_mask"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/protobuf/types/known/fieldmaskpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/buildbucket/appengine/common"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateUpdate(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
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
				So(validateUpdate(ctx, req, nil), ShouldBeNil)
			})
		})

		Convey("fails", func() {
			Convey("with nil request", func() {
				So(validateUpdate(ctx, nil, nil), ShouldErrLike, "build.id: required")
			})

			Convey("with an invalid path", func() {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{
					"bucket.name",
				}}
				So(validateUpdate(ctx, req, nil), ShouldErrLike, `unsupported path "bucket.name"`)
			})

			Convey("with a mix of valid and invalid paths", func() {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{
					"build.tags",
					"bucket.name",
					"build.output",
				}}
				So(validateUpdate(ctx, req, nil), ShouldErrLike, `unsupported path "bucket.name"`)
			})
		})
	})

	Convey("validate status", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.status"}}

		Convey("succeeds", func() {
			req.Build.Status = pb.Status_SUCCESS
			So(validateUpdate(ctx, req, nil), ShouldBeNil)
		})

		Convey("fails", func() {
			req.Build.Status = pb.Status_SCHEDULED
			So(validateUpdate(ctx, req, nil), ShouldErrLike, "build.status: invalid status SCHEDULED for UpdateBuild")
		})
	})

	Convey("validate tags", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.tags"}}
		req.Build.Tags = []*pb.StringPair{{Key: "ci:builder", Value: ""}}
		So(validateUpdate(ctx, req, nil), ShouldErrLike, `tag key "ci:builder" cannot have a colon`)
	})

	Convey("validate summary_markdown", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.summary_markdown"}}
		req.Build.SummaryMarkdown = strings.Repeat("☕", protoutil.SummaryMarkdownMaxLength)
		So(validateUpdate(ctx, req, nil), ShouldErrLike, "too big to accept")
	})

	Convey("validate output.gitiles_ommit", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.output.gitiles_commit"}}
		req.Build.Output = &pb.Build_Output{GitilesCommit: &pb.GitilesCommit{
			Project: "project",
			Host:    "host",
			Id:      "id",
		}}
		So(validateUpdate(ctx, req, nil), ShouldErrLike, "ref is required")
	})

	Convey("validate output.properties", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.output.properties"}}

		Convey("succeeds", func() {
			props, _ := structpb.NewStruct(map[string]any{"key": "value"})
			req.Build.Output = &pb.Build_Output{Properties: props}
			So(validateUpdate(ctx, req, nil), ShouldBeNil)
		})

		Convey("fails", func() {
			props, _ := structpb.NewStruct(map[string]any{"key": nil})
			req.Build.Output = &pb.Build_Output{Properties: props}
			So(validateUpdate(ctx, req, nil), ShouldErrLike, "value is not set")
		})
	})

	Convey("validate output.status", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.output.status"}}

		Convey("succeeds", func() {
			req.Build.Output = &pb.Build_Output{Status: pb.Status_SUCCESS}
			So(validateUpdate(ctx, req, nil), ShouldBeNil)
		})

		Convey("fails", func() {
			req.Build.Output = &pb.Build_Output{Status: pb.Status_SCHEDULED}
			So(validateUpdate(ctx, req, nil), ShouldErrLike, "build.output.status: invalid status SCHEDULED for UpdateBuild")
		})
	})

	Convey("validate output.summary_markdown", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.output.summary_markdown"}}
		req.Build.Output = &pb.Build_Output{SummaryMarkdown: strings.Repeat("☕", protoutil.SummaryMarkdownMaxLength)}
		So(validateUpdate(ctx, req, nil), ShouldErrLike, "too big to accept")
	})

	Convey("validate output without sub masks", t, func() {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.output"}}
		Convey("ok", func() {
			props, _ := structpb.NewStruct(map[string]any{"key": "value"})
			req.Build.Output = &pb.Build_Output{
				Properties: props,
				GitilesCommit: &pb.GitilesCommit{
					Host:     "host",
					Project:  "project",
					Ref:      "refs/",
					Position: 1,
				},
				SummaryMarkdown: "summary",
			}
			So(validateUpdate(ctx, req, nil), ShouldBeNil)
		})
		Convey("properties is invalid", func() {
			props, _ := structpb.NewStruct(map[string]any{"key": nil})
			req.Build.Output = &pb.Build_Output{
				Properties:      props,
				SummaryMarkdown: "summary",
			}
			So(validateUpdate(ctx, req, nil), ShouldErrLike, "value is not set")
		})
		Convey("summary_markdown is invalid", func() {
			req.Build.Output = &pb.Build_Output{
				SummaryMarkdown: strings.Repeat("☕", protoutil.SummaryMarkdownMaxLength),
			}
			So(validateUpdate(ctx, req, nil), ShouldErrLike, "too big to accept")
		})
		Convey("gitiles_commit is invalid", func() {
			req.Build.Output = &pb.Build_Output{
				GitilesCommit: &pb.GitilesCommit{
					Host:     "host",
					Project:  "project",
					Position: 1,
				},
			}
			So(validateUpdate(ctx, req, nil), ShouldErrLike, "ref is required")
		})
	})

	Convey("validate steps", t, func() {
		ts := timestamppb.New(testclock.TestRecentTimeUTC)
		bs := &model.BuildSteps{ID: 1}
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.steps"}}

		Convey("succeeds", func() {
			req.Build.Steps = []*pb.Step{
				{Name: "step1", Status: pb.Status_SUCCESS, StartTime: ts, EndTime: ts},
				{Name: "step2", Status: pb.Status_SUCCESS, StartTime: ts, EndTime: ts},
			}
			So(validateUpdate(ctx, req, bs), ShouldBeNil)
		})

		Convey("fails with duplicates", func() {
			ts := timestamppb.New(testclock.TestRecentTimeUTC)
			req.Build.Steps = []*pb.Step{
				{Name: "step1", Status: pb.Status_SUCCESS, StartTime: ts, EndTime: ts},
				{Name: "step1", Status: pb.Status_SUCCESS, StartTime: ts, EndTime: ts},
			}
			So(validateUpdate(ctx, req, bs), ShouldErrLike, `duplicate: "step1"`)
		})

		Convey("with a parent step", func() {
			Convey("before child", func() {
				req.Build.Steps = []*pb.Step{
					{Name: "parent", Status: pb.Status_SUCCESS, StartTime: ts, EndTime: ts},
					{Name: "parent|child", Status: pb.Status_SUCCESS, StartTime: ts, EndTime: ts},
				}
				So(validateUpdate(ctx, req, bs), ShouldBeNil)
			})
			Convey("after child", func() {
				req.Build.Steps = []*pb.Step{
					{Name: "parent|child", Status: pb.Status_SUCCESS, StartTime: ts, EndTime: ts},
					{Name: "parent", Status: pb.Status_SUCCESS, StartTime: ts, EndTime: ts},
				}
				So(validateUpdate(ctx, req, bs), ShouldErrLike, `parent of "parent|child" must precede`)
			})
		})
	})

	Convey("validate agent output", t, func() {
		req := &pb.UpdateBuildRequest{
			Build: &pb.Build{
				Id: 1,
				Infra: &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Agent: &pb.BuildInfra_Buildbucket_Agent{},
					},
				},
			},
		}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.infra.buildbucket.agent.output"}}

		Convey("empty", func() {
			So(validateUpdate(ctx, req, nil), ShouldErrLike, "agent output is not set while its field path appears in update_mask")
		})

		Convey("invalid cipd", func() {
			// wrong or unresolved version
			req.Build.Infra.Buildbucket.Agent.Output = &pb.BuildInfra_Buildbucket_Agent_Output{
				ResolvedData: map[string]*pb.ResolvedDataRef{
					"cipd": {
						DataType: &pb.ResolvedDataRef_Cipd{
							Cipd: &pb.ResolvedDataRef_CIPD{
								Specs: []*pb.ResolvedDataRef_CIPD_PkgSpec{{Package: "package", Version: "unresolved_v"}},
							},
						},
					},
				},
			}
			So(validateUpdate(ctx, req, nil), ShouldErrLike, `build.infra.buildbucket.agent.output: cipd.version: not a valid package instance ID "unresolved_v"`)

			// wrong or unresolved package name
			req.Build.Infra.Buildbucket.Agent.Output = &pb.BuildInfra_Buildbucket_Agent_Output{
				ResolvedData: map[string]*pb.ResolvedDataRef{
					"cipd": {
						DataType: &pb.ResolvedDataRef_Cipd{
							Cipd: &pb.ResolvedDataRef_CIPD{
								Specs: []*pb.ResolvedDataRef_CIPD_PkgSpec{{Package: "infra/${platform}", Version: "GwXmwYBjad-WzXEyWWn8HkDsizOPFSH_gjJ35zQaA8IC"}},
							},
						},
					},
				},
			}
			So(validateUpdate(ctx, req, nil), ShouldErrLike, `cipd.package: invalid package name "infra/${platform}"`)

			// build.status and agent.output.status conflicts
			req.Build.Status = pb.Status_CANCELED
			req.Build.Infra.Buildbucket.Agent.Output.Status = pb.Status_STARTED
			So(validateUpdate(ctx, req, nil), ShouldErrLike, "build is in an ended status while agent output status is not ended")
		})

		Convey("valid", func() {
			req.Build.Infra.Buildbucket.Agent.Output = &pb.BuildInfra_Buildbucket_Agent_Output{
				Status:        pb.Status_SUCCESS,
				AgentPlatform: "linux-amd64",
				ResolvedData: map[string]*pb.ResolvedDataRef{
					"cipd": {
						DataType: &pb.ResolvedDataRef_Cipd{
							Cipd: &pb.ResolvedDataRef_CIPD{
								Specs: []*pb.ResolvedDataRef_CIPD_PkgSpec{
									{Package: "infra/tools/git/linux-amd64", Version: "GwXmwYBjad-WzXEyWWn8HkDsizOPFSH_gjJ35zQaA8IC"}},
							},
						},
					},
				},
			}
			So(validateUpdate(ctx, req, nil), ShouldBeNil)
		})

	})

	Convey("validate agent purpose", t, func() {
		req := &pb.UpdateBuildRequest{
			Build: &pb.Build{
				Id: 1,
				Infra: &pb.BuildInfra{
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Agent: &pb.BuildInfra_Buildbucket_Agent{},
					},
				},
			},
		}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.infra.buildbucket.agent.purposes"}}

		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		So(datastore.Put(ctx, &model.BuildInfra{
			Build: datastore.KeyForObj(ctx, &model.Build{ID: req.Build.Id}),
			Proto: &pb.BuildInfra{
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Agent: &pb.BuildInfra_Buildbucket_Agent{
						Input: &pb.BuildInfra_Buildbucket_Agent_Input{
							Data: map[string]*pb.InputDataRef{"p1": {}},
						},
					},
				},
			},
		}), ShouldBeNil)

		Convey("nil", func() {
			So(validateUpdate(ctx, req, nil), ShouldErrLike, "build.infra.buildbucket.agent.purposes: not set")
		})

		Convey("invalid agent purpose", func() {
			req.Build.Infra.Buildbucket.Agent.Purposes = map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
				"random_p": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
			}
			So(validateUpdate(ctx, req, nil), ShouldErrLike, "build.infra.buildbucket.agent.purposes: Invalid path random_p - not in either input or output dataRef")
		})

		Convey("valid", func() {
			// in input data.
			req.Build.Infra.Buildbucket.Agent.Purposes = map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
				"p1": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
			}
			So(validateUpdate(ctx, req, nil), ShouldBeNil)

			// in output data
			req.UpdateMask.Paths = append(req.UpdateMask.Paths, "build.infra.buildbucket.agent.output")
			req.Build.Infra.Buildbucket.Agent.Output = &pb.BuildInfra_Buildbucket_Agent_Output{
				ResolvedData: map[string]*pb.ResolvedDataRef{
					"output_p1": {}},
			}
			req.Build.Infra.Buildbucket.Agent.Purposes = map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
				"output_p1": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
			}
			So(validateUpdate(ctx, req, nil), ShouldBeNil)
		})
	})
}

func TestValidateStep(t *testing.T) {
	t.Parallel()

	Convey("validate", t, func() {
		ts := timestamppb.New(testclock.TestRecentTimeUTC)
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
				step.StartTime = ts
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
				step.EndTime = ts
				So(validateStep(step, nil, bStatus), ShouldErrLike, `start_time: required by status "INFRA_FAILURE"`)
			})

			Convey("missing end_time", func() {
				step.StartTime = ts
				So(validateStep(step, nil, bStatus), ShouldErrLike, "end_time: must have both or neither end_time and a terminal status")
			})

			Convey("end_time is before start_time", func() {
				step.EndTime = ts
				sts := timestamppb.New(testclock.TestRecentTimeUTC.AddDate(0, 0, 1))
				step.StartTime = sts
				So(validateStep(step, nil, bStatus), ShouldErrLike, "end_time: is before the start_time")
			})
		})

		Convey("with logs", func() {
			step.Status = pb.Status_STARTED
			step.StartTime = ts

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

		Convey("with tags", func() {
			step.Status = pb.Status_STARTED
			step.StartTime = ts

			Convey("missing key", func() {
				step.Tags = []*pb.StringPair{{Value: "hi"}}
				So(validateStep(step, nil, bStatus), ShouldErrLike, "tags[0].key: required")
			})

			Convey("reserved key", func() {
				step.Tags = []*pb.StringPair{{Key: "luci.something", Value: "hi"}}
				So(validateStep(step, nil, bStatus), ShouldErrLike, "tags[0].key: reserved prefix")
			})

			Convey("missing value", func() {
				step.Tags = []*pb.StringPair{{Key: "my-service.tag"}}
				So(validateStep(step, nil, bStatus), ShouldErrLike, "tags[0].value: required")
			})

			Convey("long key", func() {
				step.Tags = []*pb.StringPair{{
					// len=297
					Key: ("my-service.my-service.my-service.my-service.my-service.my-service.my-service.my-service.my-service." +
						"my-service.my-service.my-service.my-service.my-service.my-service.my-service.my-service.my-service." +
						"my-service.my-service.my-service.my-service.my-service.my-service.my-service.my-service.my-service."),
					Value: "yo",
				}}
				So(validateStep(step, nil, bStatus), ShouldErrLike, "tags[0].key: len > 256")
			})

			Convey("long value", func() {
				step.Tags = []*pb.StringPair{{Key: "my-service.tag", Value: strings.Repeat("derp", 500)}}
				So(validateStep(step, nil, bStatus), ShouldErrLike, "tags[0].value: len > 1024")
			})
		})
	})
}

func TestCheckBuildForUpdate(t *testing.T) {
	t.Parallel()
	updateMask := func(req *pb.UpdateBuildRequest) *mask.Mask {
		fm, err := mask.FromFieldMask(req.UpdateMask, req, false, true)
		So(err, ShouldBeNil)
		return fm.MustSubmask("build")
	}

	Convey("checkBuildForUpdate", t, func() {
		ctx := metrics.WithServiceInfo(memory.Use(context.Background()), "sv", "job", "ins")
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
			b, err := common.GetBuild(ctx, 1)
			So(err, ShouldBeNil)
			err = checkBuildForUpdate(updateMask(req), req, b)
			So(err, ShouldBeNil)

			Convey("with build.steps", func() {
				req.Build.Status = pb.Status_STARTED
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.status", "build.steps"}}
				b, err := common.GetBuild(ctx, 1)
				So(err, ShouldBeNil)
				err = checkBuildForUpdate(updateMask(req), req, b)
				So(err, ShouldBeNil)
			})
			Convey("with build.output", func() {
				req.Build.Status = pb.Status_STARTED
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.status", "build.output"}}
				b, err := common.GetBuild(ctx, 1)
				So(err, ShouldBeNil)
				err = checkBuildForUpdate(updateMask(req), req, b)
				So(err, ShouldBeNil)
			})
		})

		Convey("fails", func() {
			Convey("if ended", func() {
				build.Proto.Status = pb.Status_SUCCESS
				So(datastore.Put(ctx, build), ShouldBeNil)
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.status"}}
				b, err := common.GetBuild(ctx, 1)
				So(err, ShouldBeNil)
				err = checkBuildForUpdate(updateMask(req), req, b)
				So(err, ShouldBeRPCFailedPrecondition, "cannot update an ended build")
			})

			Convey("with build.steps", func() {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.steps"}}
				b, err := common.GetBuild(ctx, 1)
				So(err, ShouldBeNil)
				err = checkBuildForUpdate(updateMask(req), req, b)
				So(err, ShouldBeRPCInvalidArgument, "cannot update steps of a SCHEDULED build")
			})
			Convey("with build.output", func() {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.output.properties"}}
				b, err := common.GetBuild(ctx, 1)
				So(err, ShouldBeNil)
				err = checkBuildForUpdate(updateMask(req), req, b)
				So(err, ShouldBeRPCInvalidArgument, "cannot update build output fields of a SCHEDULED build")
			})
			Convey("with build.infra.buildbucket.agent.output", func() {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.infra.buildbucket.agent.output"}}
				b, err := common.GetBuild(ctx, 1)
				So(err, ShouldBeNil)
				err = checkBuildForUpdate(updateMask(req), req, b)
				So(err, ShouldBeRPCInvalidArgument, "cannot update agent output of a SCHEDULED build")
			})
		})
	})
}

func TestUpdateBuild(t *testing.T) {

	updateContextForNewBuildToken := func(ctx context.Context, buildID int64) (string, context.Context) {
		newToken, _ := buildtoken.GenerateToken(ctx, buildID, pb.TokenBody_BUILD)
		ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, newToken))
		return newToken, ctx
	}
	sortTasksByClassName := func(tasks tqtesting.TaskList) {
		sort.Slice(tasks, func(i, j int) bool {
			return tasks[i].Class < tasks[j].Class
		})
	}

	t.Parallel()

	getBuildWithDetails := func(ctx context.Context, bid int64) *model.Build {
		b, err := common.GetBuild(ctx, bid)
		So(err, ShouldBeNil)
		// ensure that the below fields were cleared when the build was saved.
		So(b.Proto.Tags, ShouldBeNil)
		So(b.Proto.Steps, ShouldBeNil)
		if b.Proto.Output != nil {
			So(b.Proto.Output.Properties, ShouldBeNil)
		}
		m := model.HardcodedBuildMask("output.properties", "steps", "tags", "infra")
		So(model.LoadBuildDetails(ctx, m, nil, b.Proto), ShouldBeNil)
		return b
	}

	Convey("UpdateBuild", t, func() {
		srv := &Builds{}
		ctx := memory.Use(context.Background())
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx = installTestSecret(ctx)

		tk, ctx := updateContextForNewBuildToken(ctx, 1)
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
				Output: &pb.Build_Output{
					Status: pb.Status_STARTED,
				},
			},
			CreateTime:  t0,
			UpdateToken: tk,
		}
		bk := datastore.KeyForObj(ctx, build)
		infra := &model.BuildInfra{
			Build: bk,
			Proto: &pb.BuildInfra{
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Hostname: "bbhost",
					Agent: &pb.BuildInfra_Buildbucket_Agent{
						Input: &pb.BuildInfra_Buildbucket_Agent_Input{
							Data: map[string]*pb.InputDataRef{},
						},
					},
				},
			},
		}
		bs := &model.BuildStatus{
			Build:  bk,
			Status: pb.Status_STARTED,
		}
		So(datastore.Put(ctx, build, infra, bs), ShouldBeNil)

		req := &pb.UpdateBuildRequest{
			Build: &pb.Build{Id: 1, SummaryMarkdown: "summary"},
			UpdateMask: &field_mask.FieldMask{Paths: []string{
				"build.summary_markdown",
			}},
		}

		Convey("open mask, empty request", func() {
			validMasks := []struct {
				name string
				err  string
			}{
				{"build.output", ""},
				{"build.output.properties", ""},
				{"build.output.status", "invalid status STATUS_UNSPECIFIED"},
				{"build.status", "invalid status STATUS_UNSPECIFIED"},
				{"build.output.status_details", ""},
				{"build.status_details", ""},
				{"build.steps", ""},
				{"build.output.summary_markdown", ""},
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
			req.UpdateMask = nil
			So(updateBuild(ctx, req), ShouldBeNil)
			b, err := common.GetBuild(ctx, req.Build.Id)
			So(err, ShouldBeNil)
			So(b.Proto.UpdateTime, ShouldResembleProto, timestamppb.New(t0))
			So(b.Proto.Status, ShouldEqual, pb.Status_STARTED)

			tclock.Add(time.Second)

			So(updateBuild(ctx, req), ShouldBeNil)
			b, err = common.GetBuild(ctx, req.Build.Id)
			So(err, ShouldBeNil)
			So(b.Proto.UpdateTime, ShouldResembleProto, timestamppb.New(t0.Add(time.Second)))
		})

		Convey("build.view_url", func() {
			url := "https://redirect.com"
			req.Build.ViewUrl = url
			req.UpdateMask.Paths[0] = "build.view_url"
			So(updateBuild(ctx, req), ShouldBeRPCOK)
			b, err := common.GetBuild(ctx, req.Build.Id)
			So(err, ShouldBeNil)
			So(b.Proto.ViewUrl, ShouldEqual, url)
		})

		Convey("build.output.properties", func() {
			props, err := structpb.NewStruct(map[string]any{"key": "value"})
			So(err, ShouldBeNil)
			req.Build.Output = &pb.Build_Output{Properties: props}

			Convey("with mask", func() {
				req.UpdateMask.Paths[0] = "build.output.properties"
				So(updateBuild(ctx, req), ShouldBeRPCOK)
				b := getBuildWithDetails(ctx, req.Build.Id)
				m, err := structpb.NewStruct(map[string]any{"key": "value"})
				So(err, ShouldBeNil)
				So(b.Proto.Output.Properties, ShouldResembleProto, m)
			})

			Convey("without mask", func() {
				So(updateBuild(ctx, req), ShouldBeRPCOK)
				b := getBuildWithDetails(ctx, req.Build.Id)
				So(b.Proto.Output.Properties, ShouldBeNil)
			})

		})

		Convey("build.output.properties large", func() {
			largeProps, err := structpb.NewStruct(map[string]any{})
			So(err, ShouldBeNil)
			k := "laaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaarge_key"
			v := "laaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaarge_value"
			for i := 0; i < 10000; i++ {
				largeProps.Fields[k+strconv.Itoa(i)] = &structpb.Value{
					Kind: &structpb.Value_StringValue{
						StringValue: v,
					},
				}
			}
			So(err, ShouldBeNil)
			req.Build.Output = &pb.Build_Output{Properties: largeProps}

			Convey("with mask", func() {
				req.UpdateMask.Paths[0] = "build.output"
				So(updateBuild(ctx, req), ShouldBeRPCOK)
				b := getBuildWithDetails(ctx, req.Build.Id)
				So(b.Proto.Output.Properties, ShouldResembleProto, largeProps)
				count, err := datastore.Count(ctx, datastore.NewQuery("PropertyChunk"))
				So(err, ShouldBeNil)
				So(count, ShouldEqual, 1)
			})

			Convey("without mask", func() {
				So(updateBuild(ctx, req), ShouldBeRPCOK)
				b := getBuildWithDetails(ctx, req.Build.Id)
				So(b.Proto.Output.Properties, ShouldBeNil)
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
					nbs := &model.BuildStatus{Build: bk}
					err := datastore.Get(ctx, nbs)
					So(err, ShouldBeNil)
					So(nbs.Status, ShouldEqual, pb.Status_SUCCESS)

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

		Convey("build.infra.buildbucket.agent.output", func() {
			agentOutput := &pb.BuildInfra_Buildbucket_Agent_Output{
				Status:        pb.Status_SUCCESS,
				AgentPlatform: "linux-amd64",
				ResolvedData: map[string]*pb.ResolvedDataRef{
					"cipd": {
						DataType: &pb.ResolvedDataRef_Cipd{
							Cipd: &pb.ResolvedDataRef_CIPD{
								Specs: []*pb.ResolvedDataRef_CIPD_PkgSpec{{Package: "infra/tools/git/linux-amd64", Version: "GwXmwYBjad-WzXEyWWn8HkDsizOPFSH_gjJ35zQaA8IC"}},
							},
						},
					},
				},
			}
			req.Build.Infra = &pb.BuildInfra{
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Agent: &pb.BuildInfra_Buildbucket_Agent{
						Output: agentOutput,
					},
				},
			}
			req.Build.Input = &pb.Build_Input{
				Experiments: []string{"luci.buildbucket.agent.cipd_installation"},
			}

			Convey("with mask", func() {
				req.UpdateMask.Paths[0] = "build.infra.buildbucket.agent.output"
				So(updateBuild(ctx, req), ShouldBeRPCOK)
				b := getBuildWithDetails(ctx, req.Build.Id)
				So(b.Proto.Infra.Buildbucket.Agent.Output, ShouldResembleProto, agentOutput)
			})

			Convey("without mask", func() {
				So(updateBuild(ctx, req), ShouldBeRPCOK)
				b := getBuildWithDetails(ctx, req.Build.Id)
				So(b.Proto.Infra.Buildbucket.Agent.Output, ShouldBeNil)
			})

		})

		Convey("build.infra.buildbucket.agent.purposes", func() {
			req.Build.Infra = &pb.BuildInfra{
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Agent: &pb.BuildInfra_Buildbucket_Agent{
						Purposes: map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
							"p1": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
						},
						Output: &pb.BuildInfra_Buildbucket_Agent_Output{
							ResolvedData: map[string]*pb.ResolvedDataRef{
								"p1": {},
							},
						},
					},
				},
			}

			Convey("with mask", func() {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{
					"build.infra.buildbucket.agent.output",
					"build.infra.buildbucket.agent.purposes",
				}}
				So(updateBuild(ctx, req), ShouldBeRPCOK)
				b := getBuildWithDetails(ctx, req.Build.Id)
				So(b.Proto.Infra.Buildbucket.Agent.Purposes["p1"], ShouldEqual, pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD)
			})

			Convey("without mask", func() {
				So(updateBuild(ctx, req), ShouldBeRPCOK)
				b := getBuildWithDetails(ctx, req.Build.Id)
				So(b.Proto.Infra.Buildbucket.Agent.Purposes, ShouldBeNil)
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
				tk, ctx = updateContextForNewBuildToken(ctx, build.ID)
				build.UpdateToken = tk
				build.Proto.Status, build.Status = pb.Status_SCHEDULED, pb.Status_SCHEDULED
				buildStatus := &model.BuildStatus{
					Build:  datastore.KeyForObj(ctx, build),
					Status: pb.Status_SCHEDULED,
				}
				So(datastore.Put(ctx, build, buildStatus), ShouldBeNil)

				// update it with STARTED
				req.Build.Id = build.ID
				req.UpdateMask.Paths[0] = "build.status"
				req.Build.Status = pb.Status_STARTED
				So(updateBuild(ctx, req), ShouldBeRPCOK)

				// TQ tasks for pubsub-notification.
				tasks := sch.Tasks()
				sortTasksByClassName(tasks)
				So(tasks, ShouldHaveLength, 2)
				So(tasks[0].Payload.(*taskdefs.NotifyPubSub).GetBuildId(), ShouldEqual, build.ID)
				So(tasks[1].Payload.(*taskdefs.NotifyPubSubGoProxy).GetBuildId(), ShouldEqual, 2)
				So(tasks[1].Payload.(*taskdefs.NotifyPubSubGoProxy).GetProject(), ShouldEqual, "project")

				// BuildStarted metric should be set 1.
				So(store.Get(ctx, metrics.V1.BuildCountStarted, time.Time{}, fv(false)), ShouldEqual, 1)

				// BuildStatus should be updated.
				buildStatus = &model.BuildStatus{Build: datastore.KeyForObj(ctx, build)}
				So(datastore.Get(ctx, buildStatus), ShouldBeNil)
				So(buildStatus.Status, ShouldEqual, pb.Status_STARTED)
			})

			Convey("output.status Status_STARTED w/o status change", func() {
				req.UpdateMask.Paths[0] = "build.output.status"
				req.Build.Output = &pb.Build_Output{Status: pb.Status_STARTED}
				So(updateBuild(ctx, req), ShouldBeRPCOK)

				// no TQ tasks should be scheduled.
				So(sch.Tasks(), ShouldBeEmpty)

				// no metric update, either.
				So(store.Get(ctx, metrics.V1.BuildCountStarted, time.Time{}, fv(false)), ShouldEqual, nil)
			})

			Convey("output.status Status_STARTED w/ status change", func() {
				// create a sample task with SCHEDULED.
				build.Proto.Id++
				build.ID++
				tk, ctx = updateContextForNewBuildToken(ctx, build.ID)
				build.UpdateToken = tk
				build.Proto.Status, build.Status = pb.Status_SCHEDULED, pb.Status_SCHEDULED
				buildStatus := &model.BuildStatus{
					Build:  datastore.KeyForObj(ctx, build),
					Status: pb.Status_SCHEDULED,
				}
				So(datastore.Put(ctx, build, buildStatus), ShouldBeNil)

				// update it with STARTED
				req.Build.Id = build.ID
				req.UpdateMask.Paths[0] = "build.output.status"
				req.Build.Output = &pb.Build_Output{Status: pb.Status_STARTED}
				So(updateBuild(ctx, req), ShouldBeRPCOK)

				// TQ tasks for pubsub-notification.
				tasks := sch.Tasks()
				sortTasksByClassName(tasks)
				So(tasks, ShouldHaveLength, 2)
				So(tasks[0].Payload.(*taskdefs.NotifyPubSub).GetBuildId(), ShouldEqual, build.ID)
				So(tasks[1].Payload.(*taskdefs.NotifyPubSubGoProxy).GetBuildId(), ShouldEqual, 2)
				So(tasks[1].Payload.(*taskdefs.NotifyPubSubGoProxy).GetProject(), ShouldEqual, "project")

				// BuildStarted metric should be set 1.
				So(store.Get(ctx, metrics.V1.BuildCountStarted, time.Time{}, fv(false)), ShouldEqual, 1)

				// BuildStatus should be updated.
				buildStatus = &model.BuildStatus{Build: datastore.KeyForObj(ctx, build)}
				So(datastore.Get(ctx, build, buildStatus), ShouldBeNil)
				So(buildStatus.Status, ShouldEqual, pb.Status_STARTED)
				So(build.Proto.Status, ShouldEqual, pb.Status_STARTED)
			})
		})

		Convey("build-completion event", func() {
			Convey("Status_SUCCESSS w/ status change", func() {
				req.UpdateMask.Paths[0] = "build.status"
				req.Build.Status = pb.Status_SUCCESS
				So(updateBuild(ctx, req), ShouldBeRPCOK)

				// TQ tasks for pubsub-notification, bq-export, and invocation-finalization.
				tasks := sch.Tasks()
				So(tasks, ShouldHaveLength, 4)
				sum := 0
				for _, task := range tasks {
					switch v := task.Payload.(type) {
					case *taskdefs.NotifyPubSub:
						sum++
						So(v.GetBuildId(), ShouldEqual, req.Build.Id)
					case *taskdefs.ExportBigQuery:
						sum += 2
						So(v.GetBuildId(), ShouldEqual, req.Build.Id)
					case *taskdefs.FinalizeResultDBGo:
						sum += 4
						So(v.GetBuildId(), ShouldEqual, req.Build.Id)
					case *taskdefs.NotifyPubSubGoProxy:
						sum += 8
						So(v.GetBuildId(), ShouldEqual, req.Build.Id)
					default:
						panic("invalid task payload")
					}
				}
				So(sum, ShouldEqual, 15)

				// BuildCompleted metric should be set to 1 with SUCCESS.
				fvs := fv(model.Success.String(), "", "", false)
				So(store.Get(ctx, metrics.V1.BuildCountCompleted, time.Time{}, fvs), ShouldEqual, 1)
			})
			Convey("output.status Status_SUCCESSS w/ status change", func() {
				buildStatus := &model.BuildStatus{
					Build:  datastore.KeyForObj(ctx, build),
					Status: pb.Status_STARTED,
				}
				So(datastore.Put(ctx, build, buildStatus), ShouldBeNil)

				req.UpdateMask.Paths[0] = "build.output.status"
				req.Build.Output = &pb.Build_Output{Status: pb.Status_SUCCESS}
				So(updateBuild(ctx, req), ShouldBeRPCOK)

				// TQ tasks for pubsub-notification, bq-export, and invocation-finalization.
				tasks := sch.Tasks()
				So(tasks, ShouldHaveLength, 0)

				// BuildCompleted metric should not be set.
				fvs := fv(model.Success.String(), "", "", false)
				So(store.Get(ctx, metrics.V1.BuildCountCompleted, time.Time{}, fvs), ShouldBeNil)

				// BuildStatus should not be updated.
				buildStatus = &model.BuildStatus{Build: datastore.KeyForObj(ctx, build)}
				So(datastore.Get(ctx, build, buildStatus), ShouldBeNil)
				So(buildStatus.Status, ShouldEqual, pb.Status_STARTED)
				So(build.Proto.Status, ShouldEqual, pb.Status_STARTED)
			})
			Convey("update output without output.status should not affect overall status", func() {
				buildStatus := &model.BuildStatus{
					Build:  datastore.KeyForObj(ctx, build),
					Status: pb.Status_STARTED,
				}
				So(datastore.Put(ctx, build, buildStatus), ShouldBeNil)

				req.UpdateMask.Paths[0] = "build.output"
				req.Build.Output = &pb.Build_Output{
					Status: pb.Status_SUCCESS,
				}
				req.Build.Status = pb.Status_SUCCESS
				So(updateBuild(ctx, req), ShouldBeRPCOK)

				// TQ tasks for pubsub-notification, bq-export, and invocation-finalization.
				tasks := sch.Tasks()
				So(tasks, ShouldHaveLength, 0)

				// BuildCompleted metric should not be set.
				fvs := fv(model.Success.String(), "", "", false)
				So(store.Get(ctx, metrics.V1.BuildCountCompleted, time.Time{}, fvs), ShouldBeNil)

				// BuildStatus should not be updated.
				buildStatus = &model.BuildStatus{Build: datastore.KeyForObj(ctx, build)}
				So(datastore.Get(ctx, build, buildStatus), ShouldBeNil)
				So(buildStatus.Status, ShouldEqual, pb.Status_STARTED)
				So(build.Proto.Status, ShouldEqual, pb.Status_STARTED)
				So(build.Proto.Output.Status, ShouldEqual, pb.Status_STARTED)
			})
		})

		Convey("read mask", func() {
			Convey("w/ read mask", func() {
				req.UpdateMask.Paths[0] = "build.status"
				req.Build.Status = pb.Status_SUCCESS
				req.Mask = &pb.BuildMask{
					Fields: &fieldmaskpb.FieldMask{
						Paths: []string{
							"status",
						},
					},
				}
				b, err := srv.UpdateBuild(ctx, req)
				So(err, ShouldBeNil)
				So(b, ShouldResembleProto, &pb.Build{
					Status: pb.Status_SUCCESS,
				})
			})
		})

		Convey("update build with parent", func() {
			parent := &model.Build{
				ID: 10,
				Proto: &pb.Build{
					Id: 10,
					Builder: &pb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: pb.Status_SUCCESS,
				},
				CreateTime:  t0,
				UpdateToken: tk,
			}
			ps := &model.BuildStatus{
				Build:  datastore.KeyForObj(ctx, parent),
				Status: pb.Status_STARTED,
			}
			So(datastore.Put(ctx, parent, ps), ShouldBeNil)

			Convey("child can outlive parent", func() {
				child := &model.Build{
					ID: 11,
					Proto: &pb.Build{
						Id: 11,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Status:           pb.Status_SCHEDULED,
						AncestorIds:      []int64{10},
						CanOutliveParent: true,
					},
					CreateTime:  t0,
					UpdateToken: tk,
				}
				So(datastore.Put(ctx, child), ShouldBeNil)
				req.UpdateMask.Paths[0] = "build.status"
				req.Build.Status = pb.Status_STARTED
				So(updateBuild(ctx, req), ShouldBeRPCOK)
			})

			Convey("child cannot outlive parent", func() {
				child := &model.Build{
					ID: 11,
					Proto: &pb.Build{
						Id: 11,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Status:           pb.Status_SCHEDULED,
						AncestorIds:      []int64{10},
						CanOutliveParent: false,
					},
					CreateTime:  t0,
					UpdateToken: tk,
				}
				tk, ctx = updateContextForNewBuildToken(ctx, 11)
				child.UpdateToken = tk
				cs := &model.BuildStatus{
					Build:  datastore.KeyForObj(ctx, child),
					Status: pb.Status_STARTED,
				}
				So(datastore.Put(ctx, child, cs), ShouldBeNil)

				Convey("request is to terminate the child", func() {
					req.UpdateMask.Paths[0] = "build.status"
					req.Build.Id = 11
					req.Build.Status = pb.Status_SUCCESS
					req.Mask = &pb.BuildMask{
						Fields: &fieldmaskpb.FieldMask{
							Paths: []string{
								"status",
								"cancel_time",
							},
						},
					}
					build, err := srv.UpdateBuild(ctx, req)
					So(err, ShouldBeRPCOK)
					So(build.Status, ShouldEqual, pb.Status_SUCCESS)
					So(build.CancelTime, ShouldBeNil)

					tasks := sch.Tasks()
					So(tasks, ShouldHaveLength, 4)
					sum := 0
					for _, task := range tasks {
						switch v := task.Payload.(type) {
						case *taskdefs.NotifyPubSub:
							sum++
							So(v.GetBuildId(), ShouldEqual, req.Build.Id)
						case *taskdefs.ExportBigQuery:
							sum += 2
							So(v.GetBuildId(), ShouldEqual, req.Build.Id)
						case *taskdefs.FinalizeResultDBGo:
							sum += 4
							So(v.GetBuildId(), ShouldEqual, req.Build.Id)
						case *taskdefs.NotifyPubSubGoProxy:
							sum += 8
							So(v.GetBuildId(), ShouldEqual, req.Build.Id)
						default:
							panic("invalid task payload")
						}
					}
					So(sum, ShouldEqual, 15)

					// BuildCompleted metric should be set to 1 with SUCCESS.
					fvs := fv(model.Success.String(), "", "", false)
					So(store.Get(ctx, metrics.V1.BuildCountCompleted, time.Time{}, fvs), ShouldEqual, 1)
				})

				Convey("start the cancel process if parent has ended", func() {
					// Child of the requested build.
					So(datastore.Put(ctx, &model.Build{
						ID: 12,
						Proto: &pb.Build{
							Id: 12,
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							AncestorIds:      []int64{11},
							CanOutliveParent: false,
						},
						UpdateToken: tk,
					}), ShouldBeNil)
					req.Build.Id = 11
					req.Build.Status = pb.Status_STARTED
					req.UpdateMask.Paths[0] = "build.status"
					req.Mask = &pb.BuildMask{
						Fields: &fieldmaskpb.FieldMask{
							Paths: []string{
								"status",
								"cancel_time",
								"cancellation_markdown",
							},
						},
					}
					build, err := srv.UpdateBuild(ctx, req)
					So(err, ShouldBeRPCOK)
					So(build.Status, ShouldEqual, pb.Status_STARTED)
					So(build.CancelTime.AsTime(), ShouldEqual, t0)
					So(build.CancellationMarkdown, ShouldEqual, "canceled because its parent 10 has terminated")
					// One pubsub notification for the status update in the request,
					// one CancelBuildTask for the requested build,
					// one CancelBuildTask for the child build.
					So(sch.Tasks(), ShouldHaveLength, 4)

					// BuildStatus is updated.
					updatedStatus := &model.BuildStatus{Build: datastore.MakeKey(ctx, "Build", 11)}
					So(datastore.Get(ctx, updatedStatus), ShouldBeNil)
					So(updatedStatus.Status, ShouldEqual, pb.Status_STARTED)
				})

				Convey("start the cancel process if parent is missing", func() {
					tk, ctx = updateContextForNewBuildToken(ctx, 15)
					b := &model.Build{
						ID: 15,
						Proto: &pb.Build{
							Id: 15,
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							AncestorIds:      []int64{3000000},
							CanOutliveParent: false,
							Status:           pb.Status_SCHEDULED,
						},
						UpdateToken: tk,
					}
					buildStatus := &model.BuildStatus{
						Build:  datastore.KeyForObj(ctx, b),
						Status: b.Proto.Status,
					}
					So(datastore.Put(ctx, b, buildStatus), ShouldBeNil)
					req.Build.Id = 15
					req.Build.Status = pb.Status_STARTED
					req.UpdateMask.Paths[0] = "build.status"
					req.Mask = &pb.BuildMask{
						Fields: &fieldmaskpb.FieldMask{
							Paths: []string{
								"status",
								"cancel_time",
								"cancellation_markdown",
							},
						},
					}
					build, err := srv.UpdateBuild(ctx, req)
					So(err, ShouldBeRPCOK)
					So(build.Status, ShouldEqual, pb.Status_STARTED)
					So(build.CancelTime.AsTime(), ShouldEqual, t0)
					So(build.CancellationMarkdown, ShouldEqual, "canceled because its parent 3000000 is missing")
					So(sch.Tasks(), ShouldHaveLength, 3)

					// BuildStatus is updated.
					updatedStatus := &model.BuildStatus{Build: datastore.MakeKey(ctx, "Build", 15)}
					So(datastore.Get(ctx, updatedStatus), ShouldBeNil)
					So(updatedStatus.Status, ShouldEqual, pb.Status_STARTED)
				})

				Convey("return err if failed to get parent", func() {
					So(datastore.Put(ctx, &model.Build{
						ID: 31,
						Proto: &pb.Build{
							Id: 31,
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							AncestorIds:      []int64{30},
							CanOutliveParent: false,
						},
						UpdateToken: tk,
					}), ShouldBeNil)

					// Mock datastore.Get failure.
					var fb featureBreaker.FeatureBreaker
					ctx, fb = featureBreaker.FilterRDS(ctx, nil)
					// Break GetMulti will ingest the error to datastore.Get,
					// directly breaking "Get" doesn't work.
					fb.BreakFeatures(errors.New("get error"), "GetMulti")

					req.Build.Id = 31
					req.Build.Status = pb.Status_STARTED
					tk, ctx = updateContextForNewBuildToken(ctx, 31)
					req.UpdateMask.Paths[0] = "build.status"
					_, err := srv.UpdateBuild(ctx, req)
					So(err, ShouldErrLike, "get error")

				})

				Convey("build is being canceled", func() {
					tk, ctx = updateContextForNewBuildToken(ctx, 13)
					So(datastore.Put(ctx, &model.Build{
						ID: 13,
						Proto: &pb.Build{
							Id: 13,
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							CancelTime:      timestamppb.New(t0.Add(-time.Minute)),
							SummaryMarkdown: "original summary",
						},
						UpdateToken: tk,
					}), ShouldBeNil)
					// Child of the requested build.
					So(datastore.Put(ctx, &model.Build{
						ID: 14,
						Proto: &pb.Build{
							Id: 14,
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							AncestorIds:      []int64{13},
							CanOutliveParent: false,
						},
						UpdateToken: tk,
					}), ShouldBeNil)
					req.Build.Id = 13
					req.Build.SummaryMarkdown = "new summary"
					req.UpdateMask.Paths[0] = "build.summary_markdown"
					req.Mask = &pb.BuildMask{
						Fields: &fieldmaskpb.FieldMask{
							Paths: []string{
								"cancel_time",
								"summary_markdown",
							},
						},
					}
					build, err := srv.UpdateBuild(ctx, req)
					So(err, ShouldBeRPCOK)
					So(build.CancelTime.AsTime(), ShouldEqual, t0.Add(-time.Minute))
					So(build.SummaryMarkdown, ShouldEqual, "new summary")
					So(sch.Tasks(), ShouldBeEmpty)
				})

				Convey("build is ended, should cancel children", func() {
					tk, ctx = updateContextForNewBuildToken(ctx, 20)
					p := &model.Build{
						ID: 20,
						Proto: &pb.Build{
							Id: 20,
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
						},
						UpdateToken: tk,
					}
					ps := &model.BuildStatus{
						Build:  datastore.KeyForObj(ctx, p),
						Status: pb.Status_STARTED,
					}
					So(datastore.Put(ctx, p, ps), ShouldBeNil)
					// Child of the requested build.
					c := &model.Build{
						ID: 21,
						Proto: &pb.Build{
							Id: 21,
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							AncestorIds:      []int64{20},
							CanOutliveParent: false,
						},
						UpdateToken: tk,
					}
					So(datastore.Put(ctx, c), ShouldBeNil)
					req.Build.Id = 20
					req.Build.Status = pb.Status_INFRA_FAILURE
					req.UpdateMask.Paths[0] = "build.status"
					_, err := srv.UpdateBuild(ctx, req)
					So(err, ShouldBeRPCOK)

					child, err := common.GetBuild(ctx, 21)
					So(err, ShouldBeNil)
					So(child.Proto.CancelTime, ShouldNotBeNil)
				})

				Convey("build gets cancel signal from backend, should cancel children", func() {
					tk, ctx = updateContextForNewBuildToken(ctx, 20)
					So(datastore.Put(ctx, &model.Build{
						ID: 20,
						Proto: &pb.Build{
							Id: 20,
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
						},
						UpdateToken: tk,
					}), ShouldBeNil)
					// Child of the requested build.
					So(datastore.Put(ctx, &model.Build{
						ID: 21,
						Proto: &pb.Build{
							Id: 21,
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							AncestorIds:      []int64{20},
							CanOutliveParent: false,
						},
						UpdateToken: tk,
					}), ShouldBeNil)
					req.Build.Id = 20
					req.UpdateMask.Paths = []string{"build.cancel_time", "build.cancellation_markdown"}
					req.Build.CancelTime = timestamppb.New(t0.Add(-time.Minute))
					req.Build.CancellationMarkdown = "swarming task is cancelled"
					_, err := srv.UpdateBuild(ctx, req)
					So(err, ShouldBeRPCOK)

					child, err := common.GetBuild(ctx, 21)
					So(err, ShouldBeNil)
					So(child.Proto.CancelTime, ShouldNotBeNil)

					// One CancelBuildTask for the requested build,
					// one CancelBuildTask for the child build.
					So(sch.Tasks(), ShouldHaveLength, 2)
				})
			})
		})
	})
}
