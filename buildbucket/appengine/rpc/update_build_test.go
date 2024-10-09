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

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/strpair"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/proto/mask"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/common/tsmon"
	"go.chromium.org/luci/gae/filter/featureBreaker"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/buildbucket"
	"go.chromium.org/luci/buildbucket/appengine/common"
	"go.chromium.org/luci/buildbucket/appengine/internal/buildtoken"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/buildbucket/protoutil"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestValidateUpdate(t *testing.T) {
	t.Parallel()
	ctx := memory.Use(context.Background())
	ftt.Run("validate UpdateMask", t, func(t *ftt.Test) {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}

		t.Run("succeeds", func(t *ftt.Test) {
			t.Run("with valid paths", func(t *ftt.Test) {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{
					"build.tags",
					"build.output",
					"build.status_details",
					"build.summary_markdown",
				}}
				req.Build.SummaryMarkdown = "this is a string"
				assert.Loosely(t, validateUpdate(ctx, req, nil), should.BeNil)
			})
		})

		t.Run("fails", func(t *ftt.Test) {
			t.Run("with nil request", func(t *ftt.Test) {
				assert.Loosely(t, validateUpdate(ctx, nil, nil), should.ErrLike("build.id: required"))
			})

			t.Run("with an invalid path", func(t *ftt.Test) {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{
					"bucket.name",
				}}
				assert.Loosely(t, validateUpdate(ctx, req, nil), should.ErrLike(`unsupported path "bucket.name"`))
			})

			t.Run("with a mix of valid and invalid paths", func(t *ftt.Test) {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{
					"build.tags",
					"bucket.name",
					"build.output",
				}}
				assert.Loosely(t, validateUpdate(ctx, req, nil), should.ErrLike(`unsupported path "bucket.name"`))
			})
		})
	})

	ftt.Run("validate status", t, func(t *ftt.Test) {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.status"}}

		t.Run("succeeds", func(t *ftt.Test) {
			req.Build.Status = pb.Status_SUCCESS
			assert.Loosely(t, validateUpdate(ctx, req, nil), should.BeNil)
		})

		t.Run("fails", func(t *ftt.Test) {
			req.Build.Status = pb.Status_SCHEDULED
			assert.Loosely(t, validateUpdate(ctx, req, nil), should.ErrLike("build.status: invalid status SCHEDULED for UpdateBuild"))
		})
	})

	ftt.Run("validate tags", t, func(t *ftt.Test) {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.tags"}}
		req.Build.Tags = []*pb.StringPair{{Key: "ci:builder", Value: ""}}
		assert.Loosely(t, validateUpdate(ctx, req, nil), should.ErrLike(`tag key "ci:builder" cannot have a colon`))
	})

	ftt.Run("validate summary_markdown", t, func(t *ftt.Test) {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.summary_markdown"}}
		req.Build.SummaryMarkdown = strings.Repeat("☕", protoutil.SummaryMarkdownMaxLength)
		assert.Loosely(t, validateUpdate(ctx, req, nil), should.ErrLike("too big to accept"))
	})

	ftt.Run("validate output.gitiles_ommit", t, func(t *ftt.Test) {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.output.gitiles_commit"}}
		req.Build.Output = &pb.Build_Output{GitilesCommit: &pb.GitilesCommit{
			Project: "project",
			Host:    "host",
			Id:      "id",
		}}
		assert.Loosely(t, validateUpdate(ctx, req, nil), should.ErrLike("ref is required"))
	})

	ftt.Run("validate output.properties", t, func(t *ftt.Test) {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.output.properties"}}

		t.Run("succeeds", func(t *ftt.Test) {
			props, _ := structpb.NewStruct(map[string]any{"key": "value"})
			req.Build.Output = &pb.Build_Output{Properties: props}
			assert.Loosely(t, validateUpdate(ctx, req, nil), should.BeNil)
		})

		t.Run("fails", func(t *ftt.Test) {
			props, _ := structpb.NewStruct(map[string]any{"key": nil})
			req.Build.Output = &pb.Build_Output{Properties: props}
			assert.Loosely(t, validateUpdate(ctx, req, nil), should.ErrLike("value is not set"))
		})
	})

	ftt.Run("validate output.status", t, func(t *ftt.Test) {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.output.status"}}

		t.Run("succeeds", func(t *ftt.Test) {
			req.Build.Output = &pb.Build_Output{Status: pb.Status_SUCCESS}
			assert.Loosely(t, validateUpdate(ctx, req, nil), should.BeNil)
		})

		t.Run("fails", func(t *ftt.Test) {
			req.Build.Output = &pb.Build_Output{Status: pb.Status_SCHEDULED}
			assert.Loosely(t, validateUpdate(ctx, req, nil), should.ErrLike("build.output.status: invalid status SCHEDULED for UpdateBuild"))
		})
	})

	ftt.Run("validate output.summary_markdown", t, func(t *ftt.Test) {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.output.summary_markdown"}}
		req.Build.Output = &pb.Build_Output{SummaryMarkdown: strings.Repeat("☕", protoutil.SummaryMarkdownMaxLength)}
		assert.Loosely(t, validateUpdate(ctx, req, nil), should.ErrLike("too big to accept"))
	})

	ftt.Run("validate output without sub masks", t, func(t *ftt.Test) {
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.output"}}
		t.Run("ok", func(t *ftt.Test) {
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
			assert.Loosely(t, validateUpdate(ctx, req, nil), should.BeNil)
		})
		t.Run("properties is invalid", func(t *ftt.Test) {
			props, _ := structpb.NewStruct(map[string]any{"key": nil})
			req.Build.Output = &pb.Build_Output{
				Properties:      props,
				SummaryMarkdown: "summary",
			}
			assert.Loosely(t, validateUpdate(ctx, req, nil), should.ErrLike("value is not set"))
		})
		t.Run("summary_markdown is invalid", func(t *ftt.Test) {
			req.Build.Output = &pb.Build_Output{
				SummaryMarkdown: strings.Repeat("☕", protoutil.SummaryMarkdownMaxLength),
			}
			assert.Loosely(t, validateUpdate(ctx, req, nil), should.ErrLike("too big to accept"))
		})
		t.Run("gitiles_commit is invalid", func(t *ftt.Test) {
			req.Build.Output = &pb.Build_Output{
				GitilesCommit: &pb.GitilesCommit{
					Host:     "host",
					Project:  "project",
					Position: 1,
				},
			}
			assert.Loosely(t, validateUpdate(ctx, req, nil), should.ErrLike("ref is required"))
		})
	})

	ftt.Run("validate steps", t, func(t *ftt.Test) {
		ts := timestamppb.New(testclock.TestRecentTimeUTC)
		bs := &model.BuildSteps{ID: 1}
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}
		req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.steps"}}

		t.Run("succeeds", func(t *ftt.Test) {
			req.Build.Steps = []*pb.Step{
				{Name: "step1", Status: pb.Status_SUCCESS, StartTime: ts, EndTime: ts},
				{Name: "step2", Status: pb.Status_SUCCESS, StartTime: ts, EndTime: ts},
			}
			assert.Loosely(t, validateUpdate(ctx, req, bs), should.BeNil)
		})

		t.Run("fails with duplicates", func(t *ftt.Test) {
			ts := timestamppb.New(testclock.TestRecentTimeUTC)
			req.Build.Steps = []*pb.Step{
				{Name: "step1", Status: pb.Status_SUCCESS, StartTime: ts, EndTime: ts},
				{Name: "step1", Status: pb.Status_SUCCESS, StartTime: ts, EndTime: ts},
			}
			assert.Loosely(t, validateUpdate(ctx, req, bs), should.ErrLike(`duplicate: "step1"`))
		})

		t.Run("with a parent step", func(t *ftt.Test) {
			t.Run("before child", func(t *ftt.Test) {
				req.Build.Steps = []*pb.Step{
					{Name: "parent", Status: pb.Status_SUCCESS, StartTime: ts, EndTime: ts},
					{Name: "parent|child", Status: pb.Status_SUCCESS, StartTime: ts, EndTime: ts},
				}
				assert.Loosely(t, validateUpdate(ctx, req, bs), should.BeNil)
			})
			t.Run("after child", func(t *ftt.Test) {
				req.Build.Steps = []*pb.Step{
					{Name: "parent|child", Status: pb.Status_SUCCESS, StartTime: ts, EndTime: ts},
					{Name: "parent", Status: pb.Status_SUCCESS, StartTime: ts, EndTime: ts},
				}
				assert.Loosely(t, validateUpdate(ctx, req, bs), should.ErrLike(`parent of "parent|child" must precede`))
			})
		})
	})

	ftt.Run("validate agent output", t, func(t *ftt.Test) {
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

		t.Run("empty", func(t *ftt.Test) {
			assert.Loosely(t, validateUpdate(ctx, req, nil), should.ErrLike("agent output is not set while its field path appears in update_mask"))
		})

		t.Run("invalid cipd", func(t *ftt.Test) {
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
			assert.Loosely(t, validateUpdate(ctx, req, nil), should.ErrLike(`build.infra.buildbucket.agent.output: cipd.version: not a valid package instance ID "unresolved_v"`))

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
			assert.Loosely(t, validateUpdate(ctx, req, nil), should.ErrLike(`cipd.package: invalid package name "infra/${platform}"`))

			// build.status and agent.output.status conflicts
			req.Build.Status = pb.Status_CANCELED
			req.Build.Infra.Buildbucket.Agent.Output.Status = pb.Status_STARTED
			assert.Loosely(t, validateUpdate(ctx, req, nil), should.ErrLike("build is in an ended status while agent output status is not ended"))
		})

		t.Run("valid", func(t *ftt.Test) {
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
			assert.Loosely(t, validateUpdate(ctx, req, nil), should.BeNil)
		})

	})

	ftt.Run("validate agent purpose", t, func(t *ftt.Test) {
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
		assert.Loosely(t, datastore.Put(ctx, &model.BuildInfra{
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
		}), should.BeNil)

		t.Run("nil", func(t *ftt.Test) {
			assert.Loosely(t, validateUpdate(ctx, req, nil), should.ErrLike("build.infra.buildbucket.agent.purposes: not set"))
		})

		t.Run("invalid agent purpose", func(t *ftt.Test) {
			req.Build.Infra.Buildbucket.Agent.Purposes = map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
				"random_p": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
			}
			assert.Loosely(t, validateUpdate(ctx, req, nil), should.ErrLike("build.infra.buildbucket.agent.purposes: Invalid path random_p - not in either input or output dataRef"))
		})

		t.Run("valid", func(t *ftt.Test) {
			// in input data.
			req.Build.Infra.Buildbucket.Agent.Purposes = map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
				"p1": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
			}
			assert.Loosely(t, validateUpdate(ctx, req, nil), should.BeNil)

			// in output data
			req.UpdateMask.Paths = append(req.UpdateMask.Paths, "build.infra.buildbucket.agent.output")
			req.Build.Infra.Buildbucket.Agent.Output = &pb.BuildInfra_Buildbucket_Agent_Output{
				ResolvedData: map[string]*pb.ResolvedDataRef{
					"output_p1": {}},
			}
			req.Build.Infra.Buildbucket.Agent.Purposes = map[string]pb.BuildInfra_Buildbucket_Agent_Purpose{
				"output_p1": pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD,
			}
			assert.Loosely(t, validateUpdate(ctx, req, nil), should.BeNil)
		})
	})
}

func TestValidateStep(t *testing.T) {
	t.Parallel()

	ftt.Run("validate", t, func(t *ftt.Test) {
		ts := timestamppb.New(testclock.TestRecentTimeUTC)
		step := &pb.Step{Name: "step1"}
		bStatus := pb.Status_STARTED

		t.Run("with status unspecified", func(t *ftt.Test) {
			step.Status = pb.Status_STATUS_UNSPECIFIED
			assert.Loosely(t, validateStep(step, nil, bStatus), should.ErrLike("status: is unspecified or unknown"))
		})

		t.Run("with status ENDED_MASK", func(t *ftt.Test) {
			step.Status = pb.Status_ENDED_MASK
			assert.Loosely(t, validateStep(step, nil, bStatus), should.ErrLike("status: must not be ENDED_MASK"))
		})

		t.Run("with non-terminal status", func(t *ftt.Test) {
			t.Run("without start_time, when should have", func(t *ftt.Test) {
				step.Status = pb.Status_STARTED
				assert.Loosely(t, validateStep(step, nil, bStatus), should.ErrLike(`start_time: required by status "STARTED"`))
			})

			t.Run("with start_time, when should not have", func(t *ftt.Test) {
				step.Status = pb.Status_SCHEDULED
				step.StartTime = ts
				assert.Loosely(t, validateStep(step, nil, bStatus), should.ErrLike(`start_time: must not be specified for status "SCHEDULED"`))
			})

			t.Run("with terminal build status", func(t *ftt.Test) {
				bStatus = pb.Status_SUCCESS
				step.Status = pb.Status_STARTED
				assert.Loosely(t, validateStep(step, nil, bStatus), should.ErrLike(`status: cannot be "STARTED" because the build has a terminal status "SUCCESS"`))
			})

		})

		t.Run("with terminal status", func(t *ftt.Test) {
			step.Status = pb.Status_INFRA_FAILURE

			t.Run("missing start_time, but end_time", func(t *ftt.Test) {
				step.EndTime = ts
				assert.Loosely(t, validateStep(step, nil, bStatus), should.ErrLike(`start_time: required by status "INFRA_FAILURE"`))
			})

			t.Run("missing end_time", func(t *ftt.Test) {
				step.StartTime = ts
				assert.Loosely(t, validateStep(step, nil, bStatus), should.ErrLike(`end_time: must have both or neither end_time and a terminal status. Got end_time: "0001-01-01 00:00:00 +0000 UTC", status: "INFRA_FAILURE" for step "step1"`))
			})

			t.Run("end_time is before start_time", func(t *ftt.Test) {
				step.EndTime = ts
				sts := timestamppb.New(testclock.TestRecentTimeUTC.AddDate(0, 0, 1))
				step.StartTime = sts
				assert.Loosely(t, validateStep(step, nil, bStatus), should.ErrLike("end_time: is before the start_time"))
			})
		})

		t.Run("with logs", func(t *ftt.Test) {
			step.Status = pb.Status_STARTED
			step.StartTime = ts

			t.Run("missing name", func(t *ftt.Test) {
				step.Logs = []*pb.Log{{Url: "url", ViewUrl: "view_url"}}
				assert.Loosely(t, validateStep(step, nil, bStatus), should.ErrLike("logs[0].name: required"))
			})

			t.Run("missing url", func(t *ftt.Test) {
				step.Logs = []*pb.Log{{Name: "name", ViewUrl: "view_url"}}
				assert.Loosely(t, validateStep(step, nil, bStatus), should.ErrLike("logs[0].url: required"))
			})

			t.Run("missing view_url", func(t *ftt.Test) {
				step.Logs = []*pb.Log{{Name: "name", Url: "url"}}
				assert.Loosely(t, validateStep(step, nil, bStatus), should.ErrLike("logs[0].view_url: required"))
			})

			t.Run("duplicate name", func(t *ftt.Test) {
				step.Logs = []*pb.Log{
					{Name: "name", Url: "url", ViewUrl: "view_url"},
					{Name: "name", Url: "url", ViewUrl: "view_url"},
				}
				assert.Loosely(t, validateStep(step, nil, bStatus), should.ErrLike(`logs[1].name: duplicate: "name"`))
			})
		})

		t.Run("with tags", func(t *ftt.Test) {
			step.Status = pb.Status_STARTED
			step.StartTime = ts

			t.Run("missing key", func(t *ftt.Test) {
				step.Tags = []*pb.StringPair{{Value: "hi"}}
				assert.Loosely(t, validateStep(step, nil, bStatus), should.ErrLike("tags[0].key: required"))
			})

			t.Run("reserved key", func(t *ftt.Test) {
				step.Tags = []*pb.StringPair{{Key: "luci.something", Value: "hi"}}
				assert.Loosely(t, validateStep(step, nil, bStatus), should.ErrLike("tags[0].key: reserved prefix"))
			})

			t.Run("missing value", func(t *ftt.Test) {
				step.Tags = []*pb.StringPair{{Key: "my-service.tag"}}
				assert.Loosely(t, validateStep(step, nil, bStatus), should.ErrLike("tags[0].value: required"))
			})

			t.Run("long key", func(t *ftt.Test) {
				step.Tags = []*pb.StringPair{{
					// len=297
					Key: ("my-service.my-service.my-service.my-service.my-service.my-service.my-service.my-service.my-service." +
						"my-service.my-service.my-service.my-service.my-service.my-service.my-service.my-service.my-service." +
						"my-service.my-service.my-service.my-service.my-service.my-service.my-service.my-service.my-service."),
					Value: "yo",
				}}
				assert.Loosely(t, validateStep(step, nil, bStatus), should.ErrLike("tags[0].key: len > 256"))
			})

			t.Run("long value", func(t *ftt.Test) {
				step.Tags = []*pb.StringPair{{Key: "my-service.tag", Value: strings.Repeat("derp", 500)}}
				assert.Loosely(t, validateStep(step, nil, bStatus), should.ErrLike("tags[0].value: len > 1024"))
			})
		})
	})
}

func TestCheckBuildForUpdate(t *testing.T) {
	t.Parallel()
	updateMask := func(req *pb.UpdateBuildRequest) *mask.Mask {
		fm, err := mask.FromFieldMask(req.UpdateMask, req, false, true)
		assert.Loosely(t, err, should.BeNil)
		return fm.MustSubmask("build")
	}

	ftt.Run("checkBuildForUpdate", t, func(t *ftt.Test) {
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
		assert.Loosely(t, datastore.Put(ctx, build), should.BeNil)
		req := &pb.UpdateBuildRequest{Build: &pb.Build{Id: 1}}

		t.Run("works", func(t *ftt.Test) {
			b, err := common.GetBuild(ctx, 1)
			assert.Loosely(t, err, should.BeNil)
			err = checkBuildForUpdate(updateMask(req), req, b)
			assert.Loosely(t, err, should.BeNil)

			t.Run("with build.steps", func(t *ftt.Test) {
				req.Build.Status = pb.Status_STARTED
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.status", "build.steps"}}
				b, err := common.GetBuild(ctx, 1)
				assert.Loosely(t, err, should.BeNil)
				err = checkBuildForUpdate(updateMask(req), req, b)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("with build.output", func(t *ftt.Test) {
				req.Build.Status = pb.Status_STARTED
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.status", "build.output"}}
				b, err := common.GetBuild(ctx, 1)
				assert.Loosely(t, err, should.BeNil)
				err = checkBuildForUpdate(updateMask(req), req, b)
				assert.Loosely(t, err, should.BeNil)
			})
			t.Run("with nothing to update", func(t *ftt.Test) {
				build.Proto.Status = pb.Status_SUCCESS
				assert.Loosely(t, datastore.Put(ctx, build), should.BeNil)
				b, err := common.GetBuild(ctx, 1)
				assert.Loosely(t, err, should.BeNil)
				err = checkBuildForUpdate(updateMask(req), req, b)
				assert.Loosely(t, err, should.BeNil)
			})
		})

		t.Run("fails", func(t *ftt.Test) {
			t.Run("if ended", func(t *ftt.Test) {
				build.Proto.Status = pb.Status_SUCCESS
				assert.Loosely(t, datastore.Put(ctx, build), should.BeNil)
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.status"}}
				b, err := common.GetBuild(ctx, 1)
				assert.Loosely(t, err, should.BeNil)
				err = checkBuildForUpdate(updateMask(req), req, b)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCFailedPrecondition)("cannot update an ended build"))
			})

			t.Run("with build.steps", func(t *ftt.Test) {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.steps"}}
				b, err := common.GetBuild(ctx, 1)
				assert.Loosely(t, err, should.BeNil)
				err = checkBuildForUpdate(updateMask(req), req, b)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("cannot update steps of a SCHEDULED build"))
			})
			t.Run("with build.output", func(t *ftt.Test) {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.output.properties"}}
				b, err := common.GetBuild(ctx, 1)
				assert.Loosely(t, err, should.BeNil)
				err = checkBuildForUpdate(updateMask(req), req, b)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("cannot update build output fields of a SCHEDULED build"))
			})
			t.Run("with build.infra.buildbucket.agent.output", func(t *ftt.Test) {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{"build.infra.buildbucket.agent.output"}}
				b, err := common.GetBuild(ctx, 1)
				assert.Loosely(t, err, should.BeNil)
				err = checkBuildForUpdate(updateMask(req), req, b)
				assert.Loosely(t, err, convey.Adapt(ShouldBeRPCInvalidArgument)("cannot update agent output of a SCHEDULED build"))
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
		assert.Loosely(t, err, should.BeNil)
		// ensure that the below fields were cleared when the build was saved.
		assert.Loosely(t, b.Proto.Tags, should.BeNil)
		assert.Loosely(t, b.Proto.Steps, should.BeNil)
		if b.Proto.Output != nil {
			assert.Loosely(t, b.Proto.Output.Properties, should.BeNil)
		}
		m := model.HardcodedBuildMask("output.properties", "steps", "tags", "infra")
		assert.Loosely(t, model.LoadBuildDetails(ctx, m, nil, b.Proto), should.BeNil)
		return b
	}

	ftt.Run("UpdateBuild", t, func(t *ftt.Test) {
		srv := &Builds{}
		ctx := memory.Use(context.Background())
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx, _ = metrics.WithCustomMetrics(ctx, &pb.SettingsCfg{})
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
			CustomMetrics: []model.CustomMetric{
				{
					Base: pb.CustomMetricBase_CUSTOM_METRIC_BASE_COUNT,
					Metric: &pb.CustomMetricDefinition{
						Name:       "chrome/infra/custom/builds/count",
						Predicates: []string{`build.tags.get_value("resultdb")!=""`},
					},
				},
			},
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
				Resultdb: &pb.BuildInfra_ResultDB{
					Hostname:   "rdbhost",
					Invocation: "inv",
				},
			},
		}
		bs := &model.BuildStatus{
			Build:  bk,
			Status: pb.Status_STARTED,
		}
		bldr := &model.Builder{
			ID:     "builder",
			Parent: model.BucketKey(ctx, "project", "bucket"),
			Config: &pb.BuilderConfig{
				MaxConcurrentBuilds: 2,
			},
		}
		assert.Loosely(t, datastore.Put(ctx, build, infra, bs, bldr), should.BeNil)

		req := &pb.UpdateBuildRequest{
			Build: &pb.Build{Id: 1, SummaryMarkdown: "summary"},
			UpdateMask: &field_mask.FieldMask{Paths: []string{
				"build.summary_markdown",
			}},
		}

		t.Run("wrong purpose token", func(t *ftt.Test) {
			tk, _ = buildtoken.GenerateToken(ctx, 1, pb.TokenBody_START_BUILD)
			ctx = metadata.NewIncomingContext(ctx, metadata.Pairs(buildbucket.BuildbucketTokenHeader, tk))
			assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldHaveGRPCStatus)(codes.Unauthenticated))
		})

		t.Run("open mask, empty request", func(t *ftt.Test) {
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
				t.Run(test.name, func(t *ftt.Test) {
					req.UpdateMask.Paths[0] = test.name
					err := updateBuild(ctx, req)
					if test.err == "" {
						assert.Loosely(t, err, should.BeNil)
					} else {
						assert.Loosely(t, err, should.ErrLike(test.err))
					}
				})
			}
		})

		t.Run("build.update_time is always updated", func(t *ftt.Test) {
			req.UpdateMask = nil
			assert.Loosely(t, updateBuild(ctx, req), should.BeNil)
			b, err := common.GetBuild(ctx, req.Build.Id)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, b.Proto.UpdateTime, should.Resemble(timestamppb.New(t0)))
			assert.Loosely(t, b.Proto.Status, should.Equal(pb.Status_STARTED))

			tclock.Add(time.Second)

			assert.Loosely(t, updateBuild(ctx, req), should.BeNil)
			b, err = common.GetBuild(ctx, req.Build.Id)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, b.Proto.UpdateTime, should.Resemble(timestamppb.New(t0.Add(time.Second))))
		})

		t.Run("build.view_url", func(t *ftt.Test) {
			url := "https://redirect.com"
			req.Build.ViewUrl = url
			req.UpdateMask.Paths[0] = "build.view_url"
			assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())
			b, err := common.GetBuild(ctx, req.Build.Id)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, b.Proto.ViewUrl, should.Equal(url))
			assert.Loosely(t, len(b.CustomBuilderCountMetrics), should.BeZero)
		})

		t.Run("build.output.properties", func(t *ftt.Test) {
			props, err := structpb.NewStruct(map[string]any{"key": "value"})
			assert.Loosely(t, err, should.BeNil)
			req.Build.Output = &pb.Build_Output{Properties: props}

			t.Run("with mask", func(t *ftt.Test) {
				req.UpdateMask.Paths[0] = "build.output.properties"
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())
				b := getBuildWithDetails(ctx, req.Build.Id)
				m, err := structpb.NewStruct(map[string]any{"key": "value"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, b.Proto.Output.Properties, should.Resemble(m))
			})

			t.Run("without mask", func(t *ftt.Test) {
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())
				b := getBuildWithDetails(ctx, req.Build.Id)
				assert.Loosely(t, b.Proto.Output.Properties, should.BeNil)
			})

		})

		t.Run("build.output.properties large", func(t *ftt.Test) {
			largeProps, err := structpb.NewStruct(map[string]any{})
			assert.Loosely(t, err, should.BeNil)
			k := "laaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaarge_key"
			v := "laaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaarge_value"
			for i := 0; i < 10000; i++ {
				largeProps.Fields[k+strconv.Itoa(i)] = &structpb.Value{
					Kind: &structpb.Value_StringValue{
						StringValue: v,
					},
				}
			}
			assert.Loosely(t, err, should.BeNil)
			req.Build.Output = &pb.Build_Output{Properties: largeProps}

			t.Run("with mask", func(t *ftt.Test) {
				req.UpdateMask.Paths[0] = "build.output"
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())
				b := getBuildWithDetails(ctx, req.Build.Id)
				assert.Loosely(t, b.Proto.Output.Properties, should.Resemble(largeProps))
				count, err := datastore.Count(ctx, datastore.NewQuery("PropertyChunk"))
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, count, should.Equal(1))
			})

			t.Run("without mask", func(t *ftt.Test) {
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())
				b := getBuildWithDetails(ctx, req.Build.Id)
				assert.Loosely(t, b.Proto.Output.Properties, should.BeNil)
			})
		})

		t.Run("build.steps", func(t *ftt.Test) {
			step := &pb.Step{
				Name:      "step",
				StartTime: &timestamppb.Timestamp{Seconds: 1},
				EndTime:   &timestamppb.Timestamp{Seconds: 12},
				Status:    pb.Status_SUCCESS,
			}
			req.Build.Steps = []*pb.Step{step}

			t.Run("with mask", func(t *ftt.Test) {
				req.UpdateMask.Paths[0] = "build.steps"
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())
				b := getBuildWithDetails(ctx, req.Build.Id)
				assert.Loosely(t, b.Proto.Steps[0], should.Resemble(step))
			})

			t.Run("without mask", func(t *ftt.Test) {
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())
				b := getBuildWithDetails(ctx, req.Build.Id)
				assert.Loosely(t, b.Proto.Steps, should.BeNil)
			})

			t.Run("incomplete steps with non-terminal Build status", func(t *ftt.Test) {
				req.UpdateMask.Paths = []string{"build.status", "build.steps"}
				req.Build.Status = pb.Status_STARTED
				req.Build.Steps[0].Status = pb.Status_STARTED
				req.Build.Steps[0].EndTime = nil
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())
			})

			t.Run("incomplete steps with terminal Build status", func(t *ftt.Test) {
				req.UpdateMask.Paths = []string{"build.status", "build.steps"}
				req.Build.Status = pb.Status_SUCCESS

				t.Run("with mask", func(t *ftt.Test) {
					req.Build.Steps[0].Status = pb.Status_STARTED
					req.Build.Steps[0].EndTime = nil

					// Should be rejected.
					msg := `cannot be "STARTED" because the build has a terminal status "SUCCESS"`
					assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldHaveRPCCode)(codes.InvalidArgument, msg))
				})

				t.Run("w/o mask", func(t *ftt.Test) {
					// update the build with incomplete steps first.
					req.Build.Status = pb.Status_STARTED
					req.Build.Steps[0].Status = pb.Status_STARTED
					req.Build.Steps[0].EndTime = nil
					assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())

					// update the build again with a terminal status, but w/o step mask.
					req.UpdateMask.Paths = []string{"build.status"}
					req.Build.Status = pb.Status_SUCCESS
					assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())
					nbs := &model.BuildStatus{Build: bk}
					err := datastore.Get(ctx, nbs)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, nbs.Status, should.Equal(pb.Status_SUCCESS))

					// the step should have been cancelled.
					b := getBuildWithDetails(ctx, req.Build.Id)
					expected := &pb.Step{
						Name:      step.Name,
						Status:    pb.Status_CANCELED,
						StartTime: step.StartTime,
						EndTime:   timestamppb.New(t0),
					}
					assert.Loosely(t, b.Proto.Steps[0], should.Resemble(expected))
				})
			})
		})

		t.Run("build.tags", func(t *ftt.Test) {
			tag := &pb.StringPair{Key: "resultdb", Value: "disabled"}
			req.Build.Tags = []*pb.StringPair{tag}

			t.Run("with mask", func(t *ftt.Test) {
				req.UpdateMask.Paths[0] = "build.tags"
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())

				b := getBuildWithDetails(ctx, req.Build.Id)
				expected := []string{strpair.Format("resultdb", "disabled")}
				assert.Loosely(t, b.Tags, should.Resemble(expected))
				assert.Loosely(t, b.CustomBuilderCountMetrics, should.Resemble([]string{"chrome/infra/custom/builds/count"}))

				// change the value and update it again
				tag.Value = "enabled"
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())

				// both tags should exist
				b = getBuildWithDetails(ctx, req.Build.Id)
				expected = append(expected, strpair.Format("resultdb", "enabled"))
				assert.Loosely(t, b.Tags, should.Resemble(expected))
				assert.Loosely(t, b.CustomBuilderCountMetrics, should.Resemble([]string{"chrome/infra/custom/builds/count"}))
			})

			t.Run("without mask", func(t *ftt.Test) {
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())
				b := getBuildWithDetails(ctx, req.Build.Id)
				assert.Loosely(t, b.Tags, should.BeNil)
			})
		})

		t.Run("build.infra.buildbucket.agent.output", func(t *ftt.Test) {
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

			t.Run("with mask", func(t *ftt.Test) {
				req.UpdateMask.Paths[0] = "build.infra.buildbucket.agent.output"
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())
				b := getBuildWithDetails(ctx, req.Build.Id)
				assert.Loosely(t, b.Proto.Infra.Buildbucket.Agent.Output, should.Resemble(agentOutput))
			})

			t.Run("without mask", func(t *ftt.Test) {
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())
				b := getBuildWithDetails(ctx, req.Build.Id)
				assert.Loosely(t, b.Proto.Infra.Buildbucket.Agent.Output, should.BeNil)
			})

		})

		t.Run("build.infra.buildbucket.agent.purposes", func(t *ftt.Test) {
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

			t.Run("with mask", func(t *ftt.Test) {
				req.UpdateMask = &field_mask.FieldMask{Paths: []string{
					"build.infra.buildbucket.agent.output",
					"build.infra.buildbucket.agent.purposes",
				}}
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())
				b := getBuildWithDetails(ctx, req.Build.Id)
				assert.Loosely(t, b.Proto.Infra.Buildbucket.Agent.Purposes["p1"], should.Equal(pb.BuildInfra_Buildbucket_Agent_PURPOSE_EXE_PAYLOAD))
			})

			t.Run("without mask", func(t *ftt.Test) {
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())
				b := getBuildWithDetails(ctx, req.Build.Id)
				assert.Loosely(t, b.Proto.Infra.Buildbucket.Agent.Purposes, should.BeNil)
			})
		})

		t.Run("build-start event", func(t *ftt.Test) {
			t.Run("Status_STARTED w/o status change", func(t *ftt.Test) {
				req.UpdateMask.Paths[0] = "build.status"
				req.Build.Status = pb.Status_STARTED
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())

				// no TQ tasks should be scheduled.
				assert.Loosely(t, sch.Tasks(), should.BeEmpty)

				// no metric update, either.
				assert.Loosely(t, store.Get(ctx, metrics.V1.BuildCountStarted, time.Time{}, fv(false)), should.BeNil)
			})

			t.Run("Status_STARTED w/ status change", func(t *ftt.Test) {
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
				assert.Loosely(t, datastore.Put(ctx, build, buildStatus), should.BeNil)

				// update it with STARTED
				req.Build.Id = build.ID
				req.UpdateMask.Paths[0] = "build.status"
				req.Build.Status = pb.Status_STARTED
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())

				// TQ tasks for pubsub-notification.
				tasks := sch.Tasks()
				sortTasksByClassName(tasks)
				assert.Loosely(t, tasks, should.HaveLength(2))
				assert.Loosely(t, tasks[0].Payload.(*taskdefs.NotifyPubSub).GetBuildId(), should.Equal(build.ID))
				assert.Loosely(t, tasks[1].Payload.(*taskdefs.NotifyPubSubGoProxy).GetBuildId(), should.Equal(2))
				assert.Loosely(t, tasks[1].Payload.(*taskdefs.NotifyPubSubGoProxy).GetProject(), should.Equal("project"))

				// BuildStarted metric should be set 1.
				assert.Loosely(t, store.Get(ctx, metrics.V1.BuildCountStarted, time.Time{}, fv(false)), should.Equal(1))

				// BuildStatus should be updated.
				buildStatus = &model.BuildStatus{Build: datastore.KeyForObj(ctx, build)}
				assert.Loosely(t, datastore.Get(ctx, buildStatus), should.BeNil)
				assert.Loosely(t, buildStatus.Status, should.Equal(pb.Status_STARTED))
			})

			t.Run("output.status Status_STARTED w/o status change", func(t *ftt.Test) {
				req.UpdateMask.Paths[0] = "build.output.status"
				req.Build.Output = &pb.Build_Output{Status: pb.Status_STARTED}
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())

				// no TQ tasks should be scheduled.
				assert.Loosely(t, sch.Tasks(), should.BeEmpty)

				// no metric update, either.
				assert.Loosely(t, store.Get(ctx, metrics.V1.BuildCountStarted, time.Time{}, fv(false)), should.BeNil)
			})

			t.Run("output.status Status_STARTED w/ status change", func(t *ftt.Test) {
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
				assert.Loosely(t, datastore.Put(ctx, build, buildStatus), should.BeNil)

				// update it with STARTED
				req.Build.Id = build.ID
				req.UpdateMask.Paths[0] = "build.output.status"
				req.Build.Output = &pb.Build_Output{Status: pb.Status_STARTED}
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())

				// TQ tasks for pubsub-notification.
				tasks := sch.Tasks()
				sortTasksByClassName(tasks)
				assert.Loosely(t, tasks, should.HaveLength(2))
				assert.Loosely(t, tasks[0].Payload.(*taskdefs.NotifyPubSub).GetBuildId(), should.Equal(build.ID))
				assert.Loosely(t, tasks[1].Payload.(*taskdefs.NotifyPubSubGoProxy).GetBuildId(), should.Equal(2))
				assert.Loosely(t, tasks[1].Payload.(*taskdefs.NotifyPubSubGoProxy).GetProject(), should.Equal("project"))

				// BuildStarted metric should be set 1.
				assert.Loosely(t, store.Get(ctx, metrics.V1.BuildCountStarted, time.Time{}, fv(false)), should.Equal(1))

				// BuildStatus should be updated.
				buildStatus = &model.BuildStatus{Build: datastore.KeyForObj(ctx, build)}
				assert.Loosely(t, datastore.Get(ctx, build, buildStatus), should.BeNil)
				assert.Loosely(t, buildStatus.Status, should.Equal(pb.Status_STARTED))
				assert.Loosely(t, build.Proto.Status, should.Equal(pb.Status_STARTED))
			})
		})

		t.Run("build-completion event", func(t *ftt.Test) {
			t.Run("Status_SUCCESSS w/ status change", func(t *ftt.Test) {
				base := pb.CustomMetricBase_CUSTOM_METRIC_BASE_COMPLETED
				name := "chrome/infra/custom/builds/completed"
				globalCfg := &pb.SettingsCfg{
					CustomMetrics: []*pb.CustomMetric{
						{
							Name:        name,
							ExtraFields: []string{"experiments"},
							Class: &pb.CustomMetric_MetricBase{
								MetricBase: base,
							},
						},
					},
				}
				ctx, _ = metrics.WithCustomMetrics(ctx, globalCfg)

				bld := &model.Build{ID: req.Build.Id}
				assert.Loosely(t, datastore.Get(ctx, bld), should.BeNil)
				bld.CustomMetrics = []model.CustomMetric{
					{
						Base: base,
						Metric: &pb.CustomMetricDefinition{
							Name:       name,
							Predicates: []string{`build.status.to_string()!=""`},
							ExtraFields: map[string]string{
								"experiments": "build.input.experiments.to_string()",
							},
						},
					},
				}
				assert.Loosely(t, datastore.Put(ctx, bld), should.BeNil)

				req.UpdateMask.Paths[0] = "build.status"
				req.Build.Status = pb.Status_SUCCESS
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())

				// TQ tasks for pubsub-notification, bq-export, and invocation-finalization.
				tasks := sch.Tasks()
				assert.Loosely(t, tasks, should.HaveLength(5))
				sum := 0
				for _, task := range tasks {
					switch v := task.Payload.(type) {
					case *taskdefs.NotifyPubSub:
						sum++
						assert.Loosely(t, v.GetBuildId(), should.Equal(req.Build.Id))
					case *taskdefs.ExportBigQueryGo:
						sum += 2
						assert.Loosely(t, v.GetBuildId(), should.Equal(req.Build.Id))
					case *taskdefs.FinalizeResultDBGo:
						sum += 4
						assert.Loosely(t, v.GetBuildId(), should.Equal(req.Build.Id))
					case *taskdefs.NotifyPubSubGoProxy:
						sum += 8
						assert.Loosely(t, v.GetBuildId(), should.Equal(req.Build.Id))
					case *taskdefs.PopPendingBuildTask:
						sum += 16
						assert.Loosely(t, v.GetBuildId(), should.Equal(req.Build.Id))
					default:
						panic("invalid task payload")
					}
				}
				assert.Loosely(t, sum, should.Equal(31))

				// BuildCompleted metric should be set to 1 with SUCCESS.
				fvs := fv(model.Success.String(), "", "", false)
				assert.Loosely(t, store.Get(ctx, metrics.V1.BuildCountCompleted, time.Time{}, fvs), should.Equal(1))

				ctx = metrics.WithBuilder(ctx, "project", "bucket", "builder")
				assert.Loosely(t, store.Get(ctx, metrics.V2.BuildCountCompleted, time.Time{}, []any{"SUCCESS", "None"}), should.Equal(1))

				val, err := metrics.GetCustomMetricsData(ctx, base, name, time.Time{}, []any{"SUCCESS", "None"})
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, val, should.Equal(1))
			})
			t.Run("output.status Status_SUCCESSS w/ status change", func(t *ftt.Test) {
				buildStatus := &model.BuildStatus{
					Build:  datastore.KeyForObj(ctx, build),
					Status: pb.Status_STARTED,
				}
				assert.Loosely(t, datastore.Put(ctx, build, buildStatus), should.BeNil)

				req.UpdateMask.Paths[0] = "build.output.status"
				req.Build.Output = &pb.Build_Output{Status: pb.Status_SUCCESS}
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())

				// TQ tasks for pubsub-notification, bq-export, and invocation-finalization.
				tasks := sch.Tasks()
				assert.Loosely(t, tasks, should.HaveLength(0))

				// BuildCompleted metric should not be set.
				fvs := fv(model.Success.String(), "", "", false)
				assert.Loosely(t, store.Get(ctx, metrics.V1.BuildCountCompleted, time.Time{}, fvs), should.BeNil)

				// BuildStatus should not be updated.
				buildStatus = &model.BuildStatus{Build: datastore.KeyForObj(ctx, build)}
				assert.Loosely(t, datastore.Get(ctx, build, buildStatus), should.BeNil)
				assert.Loosely(t, buildStatus.Status, should.Equal(pb.Status_STARTED))
				assert.Loosely(t, build.Proto.Status, should.Equal(pb.Status_STARTED))
			})
			t.Run("update output without output.status should not affect overall status", func(t *ftt.Test) {
				buildStatus := &model.BuildStatus{
					Build:  datastore.KeyForObj(ctx, build),
					Status: pb.Status_STARTED,
				}
				assert.Loosely(t, datastore.Put(ctx, build, buildStatus), should.BeNil)

				req.UpdateMask.Paths[0] = "build.output"
				req.Build.Output = &pb.Build_Output{
					Status: pb.Status_SUCCESS,
				}
				req.Build.Status = pb.Status_SUCCESS
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())

				// TQ tasks for pubsub-notification, bq-export, and invocation-finalization.
				tasks := sch.Tasks()
				assert.Loosely(t, tasks, should.HaveLength(0))

				// BuildCompleted metric should not be set.
				fvs := fv(model.Success.String(), "", "", false)
				assert.Loosely(t, store.Get(ctx, metrics.V1.BuildCountCompleted, time.Time{}, fvs), should.BeNil)

				// BuildStatus should not be updated.
				buildStatus = &model.BuildStatus{Build: datastore.KeyForObj(ctx, build)}
				assert.Loosely(t, datastore.Get(ctx, build, buildStatus), should.BeNil)
				assert.Loosely(t, buildStatus.Status, should.Equal(pb.Status_STARTED))
				assert.Loosely(t, build.Proto.Status, should.Equal(pb.Status_STARTED))
				assert.Loosely(t, build.Proto.Output.Status, should.Equal(pb.Status_STARTED))
			})
			t.Run("Status_SUCCESSS w/ status change with max_concurrent_builds disabled", func(t *ftt.Test) {
				bldr := &model.Builder{
					ID:     "builder",
					Parent: model.BucketKey(ctx, "project", "bucket"),
				}
				assert.Loosely(t, datastore.Put(ctx, bldr), should.BeNil)
				req.UpdateMask.Paths[0] = "build.status"
				req.Build.Status = pb.Status_SUCCESS
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())
				// TQ tasks for pubsub-notification, bq-export, and invocation-finalization.
				tasks := sch.Tasks()
				assert.Loosely(t, tasks, should.HaveLength(4))
			})
			t.Run("led build - Status_SUCCESSS w/ status change", func(t *ftt.Test) {
				tk, ctx := updateContextForNewBuildToken(ctx, 2)
				build := &model.Build{
					ID: 2,
					Proto: &pb.Build{
						Id: 2,
						Builder: &pb.BuilderID{
							Project: "project",
							Bucket:  "bucket.shadow",
							Builder: "builder",
						},
						Infra: &pb.BuildInfra{Led: &pb.BuildInfra_Led{
							ShadowedBucket: "bucket",
						}},
					},
					UpdateToken: tk,
				}
				bk := datastore.KeyForObj(ctx, build)
				infra := &model.BuildInfra{
					Build: bk,
					Proto: &pb.BuildInfra{
						Resultdb: &pb.BuildInfra_ResultDB{
							Hostname:   "rdbhost",
							Invocation: "inv",
						},
						Led: &pb.BuildInfra_Led{
							ShadowedBucket: "bucket",
						},
					},
				}
				bs := &model.BuildStatus{
					Build:  bk,
					Status: pb.Status_STARTED,
				}
				assert.Loosely(t, datastore.Put(ctx, build, infra, bs), should.BeNil)
				req.Build = &pb.Build{Id: 2, SummaryMarkdown: "summary", Infra: &pb.BuildInfra{Led: &pb.BuildInfra_Led{
					ShadowedBucket: "bucket",
				}}}
				req.UpdateMask.Paths[0] = "build.status"
				req.Build.Status = pb.Status_SUCCESS
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())
				// TQ tasks for pubsub-notification, bq-export, and invocation-finalization.
				tasks := sch.Tasks()
				// led builds not supported by max_concurrent_builds.
				assert.Loosely(t, tasks, should.HaveLength(4))
			})
		})

		t.Run("read mask", func(t *ftt.Test) {
			t.Run("w/ read mask", func(t *ftt.Test) {
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
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, b, should.Resemble(&pb.Build{
					Status: pb.Status_SUCCESS,
				}))
			})
		})

		t.Run("update build with parent", func(t *ftt.Test) {
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
			assert.Loosely(t, datastore.Put(ctx, parent, ps), should.BeNil)

			t.Run("child can outlive parent", func(t *ftt.Test) {
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
				assert.Loosely(t, datastore.Put(ctx, child), should.BeNil)
				req.UpdateMask.Paths[0] = "build.status"
				req.Build.Status = pb.Status_STARTED
				assert.Loosely(t, updateBuild(ctx, req), convey.Adapt(ShouldBeRPCOK)())
			})

			t.Run("child cannot outlive parent", func(t *ftt.Test) {
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
				cinf := &model.BuildInfra{
					ID:    1,
					Build: datastore.KeyForObj(ctx, child),
					Proto: &pb.BuildInfra{
						Resultdb: &pb.BuildInfra_ResultDB{
							Hostname:   "rdbhost",
							Invocation: "inv",
						},
					},
				}
				tk, ctx = updateContextForNewBuildToken(ctx, 11)
				child.UpdateToken = tk
				cs := &model.BuildStatus{
					Build:  datastore.KeyForObj(ctx, child),
					Status: pb.Status_STARTED,
				}
				assert.Loosely(t, datastore.Put(ctx, child, cinf, cs), should.BeNil)

				t.Run("request is to terminate the child", func(t *ftt.Test) {
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
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCOK)())
					assert.Loosely(t, build.Status, should.Equal(pb.Status_SUCCESS))
					assert.Loosely(t, build.CancelTime, should.BeNil)

					tasks := sch.Tasks()
					assert.Loosely(t, tasks, should.HaveLength(5))
					sum := 0
					for _, task := range tasks {
						switch v := task.Payload.(type) {
						case *taskdefs.NotifyPubSub:
							sum++
							assert.Loosely(t, v.GetBuildId(), should.Equal(req.Build.Id))
						case *taskdefs.ExportBigQueryGo:
							sum += 2
							assert.Loosely(t, v.GetBuildId(), should.Equal(req.Build.Id))
						case *taskdefs.FinalizeResultDBGo:
							sum += 4
							assert.Loosely(t, v.GetBuildId(), should.Equal(req.Build.Id))
						case *taskdefs.NotifyPubSubGoProxy:
							sum += 8
							assert.Loosely(t, v.GetBuildId(), should.Equal(req.Build.Id))
						case *taskdefs.PopPendingBuildTask:
							sum += 16
							assert.Loosely(t, v.GetBuildId(), should.Equal(req.Build.Id))
						default:
							panic("invalid task payload")
						}
					}
					assert.Loosely(t, sum, should.Equal(31))

					// BuildCompleted metric should be set to 1 with SUCCESS.
					fvs := fv(model.Success.String(), "", "", false)
					assert.Loosely(t, store.Get(ctx, metrics.V1.BuildCountCompleted, time.Time{}, fvs), should.Equal(1))
				})

				t.Run("start the cancel process if parent has ended", func(t *ftt.Test) {
					// Child of the requested build.
					assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
					}), should.BeNil)
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
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCOK)())
					assert.Loosely(t, build.Status, should.Equal(pb.Status_STARTED))
					assert.Loosely(t, build.CancelTime.AsTime(), should.Resemble(t0))
					assert.Loosely(t, build.CancellationMarkdown, should.Equal("canceled because its parent 10 has terminated"))
					// One pubsub notification for the status update in the request,
					// one CancelBuildTask for the requested build,
					// one CancelBuildTask for the child build.
					assert.Loosely(t, sch.Tasks(), should.HaveLength(4))

					// BuildStatus is updated.
					updatedStatus := &model.BuildStatus{Build: datastore.MakeKey(ctx, "Build", 11)}
					assert.Loosely(t, datastore.Get(ctx, updatedStatus), should.BeNil)
					assert.Loosely(t, updatedStatus.Status, should.Equal(pb.Status_STARTED))
				})

				t.Run("start the cancel process if parent is missing", func(t *ftt.Test) {
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
					assert.Loosely(t, datastore.Put(ctx, b, buildStatus), should.BeNil)
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
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCOK)())
					assert.Loosely(t, build.Status, should.Equal(pb.Status_STARTED))
					assert.Loosely(t, build.CancelTime.AsTime(), should.Resemble(t0))
					assert.Loosely(t, build.CancellationMarkdown, should.Equal("canceled because its parent 3000000 is missing"))
					assert.Loosely(t, sch.Tasks(), should.HaveLength(3))

					// BuildStatus is updated.
					updatedStatus := &model.BuildStatus{Build: datastore.MakeKey(ctx, "Build", 15)}
					assert.Loosely(t, datastore.Get(ctx, updatedStatus), should.BeNil)
					assert.Loosely(t, updatedStatus.Status, should.Equal(pb.Status_STARTED))
				})

				t.Run("return err if failed to get parent", func(t *ftt.Test) {
					assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
					}), should.BeNil)

					// Mock datastore.Get failure.
					var fb featureBreaker.FeatureBreaker
					ctx, fb = featureBreaker.FilterRDS(ctx, nil)
					// Break GetMulti will ingest the error to datastore.Get,
					// directly breaking "Get" doesn't work.
					// This error is only logged, and the response only contains a
					// general error message.
					fb.BreakFeatures(errors.New("get error"), "GetMulti")

					req.Build.Id = 31
					req.Build.Status = pb.Status_STARTED
					tk, ctx = updateContextForNewBuildToken(ctx, 31)
					req.UpdateMask.Paths[0] = "build.status"
					_, err := srv.UpdateBuild(ctx, req)
					assert.Loosely(t, err, should.ErrLike("error fetching build entities with ID"))

				})

				t.Run("build is being canceled", func(t *ftt.Test) {
					tk, ctx = updateContextForNewBuildToken(ctx, 13)
					assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
							Status:          pb.Status_STARTED,
						},
						UpdateToken: tk,
					}), should.BeNil)
					// Child of the requested build.
					assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
							Status:           pb.Status_STARTED,
						},
						UpdateToken: tk,
					}), should.BeNil)
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
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCOK)())
					assert.Loosely(t, build.CancelTime.AsTime(), should.Resemble(t0.Add(-time.Minute)))
					assert.Loosely(t, build.SummaryMarkdown, should.Equal("new summary"))
					assert.Loosely(t, sch.Tasks(), should.BeEmpty)
				})

				t.Run("build is ended, should cancel children", func(t *ftt.Test) {
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
					pinf := &model.BuildInfra{
						ID:    1,
						Build: datastore.KeyForObj(ctx, p),
						Proto: &pb.BuildInfra{
							Resultdb: &pb.BuildInfra_ResultDB{
								Hostname:   "rdbhost",
								Invocation: "inv",
							},
						},
					}
					ps := &model.BuildStatus{
						Build:  datastore.KeyForObj(ctx, p),
						Status: pb.Status_STARTED,
					}
					assert.Loosely(t, datastore.Put(ctx, p, pinf, ps), should.BeNil)
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
					assert.Loosely(t, datastore.Put(ctx, c), should.BeNil)
					req.Build.Id = 20
					req.Build.Status = pb.Status_INFRA_FAILURE
					req.UpdateMask.Paths[0] = "build.status"
					_, err := srv.UpdateBuild(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCOK)())

					child, err := common.GetBuild(ctx, 21)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, child.Proto.CancelTime, should.NotBeNil)
				})

				t.Run("build gets cancel signal from backend, should cancel children", func(t *ftt.Test) {
					tk, ctx = updateContextForNewBuildToken(ctx, 20)
					assert.Loosely(t, datastore.Put(ctx, &model.Build{
						ID: 20,
						Proto: &pb.Build{
							Id: 20,
							Builder: &pb.BuilderID{
								Project: "project",
								Bucket:  "bucket",
								Builder: "builder",
							},
							Status: pb.Status_STARTED,
						},
						UpdateToken: tk,
					}), should.BeNil)
					// Child of the requested build.
					assert.Loosely(t, datastore.Put(ctx, &model.Build{
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
							Status:           pb.Status_STARTED,
						},
						UpdateToken: tk,
					}), should.BeNil)
					req.Build.Id = 20
					req.UpdateMask.Paths = []string{"build.cancel_time", "build.cancellation_markdown"}
					req.Build.CancelTime = timestamppb.New(t0.Add(-time.Minute))
					req.Build.CancellationMarkdown = "swarming task is cancelled"
					_, err := srv.UpdateBuild(ctx, req)
					assert.Loosely(t, err, convey.Adapt(ShouldBeRPCOK)())

					child, err := common.GetBuild(ctx, 21)
					assert.Loosely(t, err, should.BeNil)
					assert.Loosely(t, child.Proto.CancelTime, should.NotBeNil)

					// One CancelBuildTask for the requested build,
					// one CancelBuildTask for the child build.
					assert.Loosely(t, sch.Tasks(), should.HaveLength(2))
				})
			})
		})
	})
}
