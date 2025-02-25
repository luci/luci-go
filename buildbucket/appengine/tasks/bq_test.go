// Copyright 2022 The LUCI Authors.
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

package tasks

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/googleapi"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"

	lucibq "go.chromium.org/luci/common/bq"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestBQ(t *testing.T) {
	t.Parallel()

	ftt.Run("ExportBuild", t, func(t *ftt.Test) {
		ctx := txndefer.FilterRDS(memory.Use(context.Background()))
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		now := testclock.TestRecentTimeLocal
		ctx, _ = testclock.UseTime(ctx, now)
		fakeBq := &fakeBqClient{}
		ctx = clients.WithBqClient(ctx, fakeBq)
		b := &model.Build{
			ID: 123,
			Proto: &pb.Build{
				Id: 123,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_CANCELED,
			},
		}
		bk := datastore.KeyForObj(ctx, b)
		bs := &model.BuildSteps{ID: 1, Build: bk}
		assert.Loosely(t, bs.FromProto([]*pb.Step{
			{
				Name:            "step",
				SummaryMarkdown: "summary",
				Logs: []*pb.Log{{
					Name:    "log1",
					Url:     "url",
					ViewUrl: "view_url",
				},
				},
			},
		}), should.BeNil)
		bi := &model.BuildInfra{
			ID:    1,
			Build: bk,
			Proto: &pb.BuildInfra{
				Backend: &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Id:     "s93k0402js90",
							Target: "swarming://chromium-swarm",
						},
						Status:   pb.Status_CANCELED,
						Link:     "www.google.com/404",
						UpdateId: 1100,
					},
				},
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Hostname: "hostname",
				},
			},
		}
		assert.Loosely(t, datastore.Put(ctx, b, bi, bs), should.BeNil)

		t.Run("build not found", func(t *ftt.Test) {
			err := ExportBuild(ctx, 111)
			assert.Loosely(t, tq.Fatal.In(err), should.BeTrue)
			assert.Loosely(t, err, should.ErrLike("build 111 not found when exporting into BQ"))
		})

		t.Run("bad row", func(t *ftt.Test) {
			ctx1 := context.WithValue(ctx, &fakeBqErrCtxKey, bigquery.PutMultiError{bigquery.RowInsertionError{}})
			err := ExportBuild(ctx1, 123)
			assert.Loosely(t, err, should.ErrLike("bad row for build 123"))
			assert.Loosely(t, tq.Fatal.In(err), should.BeTrue)
		})

		t.Run("transient BQ err", func(t *ftt.Test) {
			ctx1 := context.WithValue(ctx, &fakeBqErrCtxKey, errors.New("transient"))
			err := ExportBuild(ctx1, 123)
			assert.Loosely(t, err, should.ErrLike("transient error when inserting BQ for build 123"))
			assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
		})

		t.Run("output properties too large", func(t *ftt.Test) {
			originLimit := maxBuildSizeInBQ
			maxBuildSizeInBQ = 600
			defer func() {
				maxBuildSizeInBQ = originLimit
			}()
			bo := &model.BuildOutputProperties{
				Build: bk,
				Proto: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"output": {
							Kind: &structpb.Value_StringValue{
								StringValue: strings.Repeat("output value", 50),
							},
						},
					},
				},
			}
			assert.Loosely(t, datastore.Put(ctx, bo), should.BeNil)

			assert.Loosely(t, ExportBuild(ctx, 123), should.BeNil)
			rows := fakeBq.GetRows("raw", "completed_builds")
			assert.Loosely(t, len(rows), should.Equal(1))
			assert.Loosely(t, rows[0].InsertID, should.Equal("123"))
			p, _ := rows[0].Message.(*pb.Build)
			assert.Loosely(t, p, should.Match(&pb.Build{
				Id: 123,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_CANCELED,
				Steps: []*pb.Step{{
					Name: "step",
					Logs: []*pb.Log{{Name: "log1"}},
				}},
				Infra: &pb.BuildInfra{
					Backend: &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Id:     "s93k0402js90",
								Target: "swarming://chromium-swarm",
							},
							Status: pb.Status_CANCELED,
							Link:   "www.google.com/404",
						},
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{},
					Swarming: &pb.BuildInfra_Swarming{
						TaskId: "s93k0402js90",
					},
				},
				Input: &pb.Build_Input{},
				Output: &pb.Build_Output{
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"strip_reason": {
								Kind: &structpb.Value_StringValue{
									StringValue: "output properties is stripped because it's too large which makes the whole build larger than BQ limit(10MB)",
								},
							},
						},
					},
				},
			}))
		})

		t.Run("strip step log as well if build is still too big", func(t *ftt.Test) {
			originLimit := maxBuildSizeInBQ
			maxBuildSizeInBQ = 500
			defer func() {
				maxBuildSizeInBQ = originLimit
			}()
			bo := &model.BuildOutputProperties{
				Build: bk,
				Proto: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"output": {
							Kind: &structpb.Value_StringValue{
								StringValue: strings.Repeat("output value", 50),
							},
						},
					},
				},
			}
			assert.Loosely(t, datastore.Put(ctx, bo), should.BeNil)

			assert.Loosely(t, ExportBuild(ctx, 123), should.BeNil)
			rows := fakeBq.GetRows("raw", "completed_builds")
			assert.Loosely(t, len(rows), should.Equal(1))
			assert.Loosely(t, rows[0].InsertID, should.Equal("123"))
			p, _ := rows[0].Message.(*pb.Build)
			assert.Loosely(t, p, should.Match(&pb.Build{
				Id: 123,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_CANCELED,
				Steps: []*pb.Step{{
					Name: "step",
				}},
				Infra: &pb.BuildInfra{
					Backend: &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Id:     "s93k0402js90",
								Target: "swarming://chromium-swarm",
							},
							Status: pb.Status_CANCELED,
							Link:   "www.google.com/404",
						},
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{},
					Swarming: &pb.BuildInfra_Swarming{
						TaskId: "s93k0402js90",
					},
				},
				Input: &pb.Build_Input{},
				Output: &pb.Build_Output{
					Properties: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"strip_reason": {
								Kind: &structpb.Value_StringValue{
									StringValue: "output properties is stripped because it's too large which makes the whole build larger than BQ limit(10MB)",
								},
							},
						},
					},
				},
			}))
		})

		t.Run("fail export if build is still too big", func(t *ftt.Test) {
			originLimit := maxBuildSizeInBQ
			maxBuildSizeInBQ = 50
			defer func() {
				maxBuildSizeInBQ = originLimit
			}()
			bo := &model.BuildOutputProperties{
				Build: bk,
				Proto: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"output": {
							Kind: &structpb.Value_StringValue{
								StringValue: "output value",
							},
						},
					},
				},
			}
			assert.Loosely(t, datastore.Put(ctx, bo), should.BeNil)

			assert.Loosely(t, ExportBuild(ctx, 123), should.ErrLike(errRowTooBig))
		})

		t.Run("summary markdown and cancelation reason are concatenated", func(t *ftt.Test) {
			b.Proto.SummaryMarkdown = "summary"
			b.Proto.CancellationMarkdown = "cancelled"
			assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)

			assert.Loosely(t, ExportBuild(ctx, 123), should.BeNil)
			rows := fakeBq.GetRows("raw", "completed_builds")
			assert.Loosely(t, len(rows), should.Equal(1))
			assert.Loosely(t, rows[0].InsertID, should.Equal("123"))
			p, _ := rows[0].Message.(*pb.Build)
			assert.Loosely(t, p, should.Match(&pb.Build{
				Id: 123,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status:               pb.Status_CANCELED,
				SummaryMarkdown:      "summary\ncancelled",
				CancellationMarkdown: "cancelled",
				Steps: []*pb.Step{{
					Name: "step",
					Logs: []*pb.Log{{Name: "log1"}},
				}},
				Infra: &pb.BuildInfra{
					Backend: &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Id:     "s93k0402js90",
								Target: "swarming://chromium-swarm",
							},
							Status: pb.Status_CANCELED,
							Link:   "www.google.com/404",
						},
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{},
					Swarming: &pb.BuildInfra_Swarming{
						TaskId: "s93k0402js90",
					},
				},
				Input:  &pb.Build_Input{},
				Output: &pb.Build_Output{},
			}))
		})

		t.Run("success", func(t *ftt.Test) {
			b.Proto.CancellationMarkdown = "cancelled"
			assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)

			assert.Loosely(t, ExportBuild(ctx, 123), should.BeNil)
			rows := fakeBq.GetRows("raw", "completed_builds")
			assert.Loosely(t, len(rows), should.Equal(1))
			assert.Loosely(t, rows[0].InsertID, should.Equal("123"))
			p, _ := rows[0].Message.(*pb.Build)
			assert.Loosely(t, p, should.Match(&pb.Build{
				Id: 123,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status:               pb.Status_CANCELED,
				SummaryMarkdown:      "cancelled",
				CancellationMarkdown: "cancelled",
				Steps: []*pb.Step{{
					Name: "step",
					Logs: []*pb.Log{{Name: "log1"}},
				}},
				Infra: &pb.BuildInfra{
					Backend: &pb.BuildInfra_Backend{
						Task: &pb.Task{
							Id: &pb.TaskID{
								Id:     "s93k0402js90",
								Target: "swarming://chromium-swarm",
							},
							Status: pb.Status_CANCELED,
							Link:   "www.google.com/404",
						},
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{},
					Swarming: &pb.BuildInfra_Swarming{
						TaskId: "s93k0402js90",
					},
				},
				Input:  &pb.Build_Input{},
				Output: &pb.Build_Output{},
			}))
		})
	})
}

func TestTryBackfillSwarming(t *testing.T) {
	t.Parallel()

	ftt.Run("tryBackfillSwarming", t, func(t *ftt.Test) {
		b := &pb.Build{
			Id: 1,
			Builder: &pb.BuilderID{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
			},
			Status: pb.Status_SUCCESS,
			Infra:  &pb.BuildInfra{},
		}
		t.Run("noop", func(t *ftt.Test) {
			t.Run("no backend", func(t *ftt.Test) {
				assert.Loosely(t, tryBackfillSwarming(b), should.BeNil)
				assert.Loosely(t, b.Infra.Swarming, should.BeNil)
			})

			t.Run("no backend task", func(t *ftt.Test) {
				b.Infra.Backend = &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://chromium-swarm",
						},
					},
				}
				assert.Loosely(t, tryBackfillSwarming(b), should.BeNil)
				assert.Loosely(t, b.Infra.Swarming, should.BeNil)
			})

			t.Run("not a swarming implemented backend", func(t *ftt.Test) {
				b.Infra.Backend = &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Id:     "s93k0402js90",
							Target: "other://chromium-swarm",
						},
					},
				}
				assert.Loosely(t, tryBackfillSwarming(b), should.BeNil)
				assert.Loosely(t, b.Infra.Swarming, should.BeNil)
			})
		})

		t.Run("swarming backfilled", func(t *ftt.Test) {
			taskDims := []*pb.RequestedDimension{
				{
					Key:   "key",
					Value: "value",
				},
			}
			b.Infra.Backend = &pb.BuildInfra_Backend{
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "s93k0402js90",
						Target: "swarming://chromium-swarm",
					},
					Status: pb.Status_SUCCESS,
				},
				Hostname: "chromium-swarm.appspot.com",
				Caches: []*pb.CacheEntry{
					{
						Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
						Path: "builder",
						WaitForWarmCache: &durationpb.Duration{
							Seconds: 240,
						},
					},
				},
				Config: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"priority":        {Kind: &structpb.Value_NumberValue{NumberValue: 20}},
						"service_account": {Kind: &structpb.Value_StringValue{StringValue: "account"}},
					},
				},
				TaskDimensions: taskDims,
			}
			t.Run("partially fail", func(t *ftt.Test) {
				b.Infra.Backend.Task.Details = &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"bot_dimensions": {Kind: &structpb.Value_StringValue{StringValue: "wrong format"}},
					},
				}
				assert.Loosely(t, tryBackfillSwarming(b), should.ErrLike("failed to unmarshal task details JSON for build 1"))
				assert.Loosely(t, b.Infra.Swarming.BotDimensions, should.HaveLength(0))
			})

			t.Run("pass", func(t *ftt.Test) {
				b.Infra.Backend.Task.Details = &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"bot_dimensions": {
							Kind: &structpb.Value_StructValue{
								StructValue: &structpb.Struct{
									Fields: map[string]*structpb.Value{
										"cpu": {
											Kind: &structpb.Value_ListValue{
												ListValue: &structpb.ListValue{
													Values: []*structpb.Value{
														{Kind: &structpb.Value_StringValue{StringValue: "x86"}},
														{Kind: &structpb.Value_StringValue{StringValue: "x86-64"}},
													},
												},
											},
										},
										"os": {
											Kind: &structpb.Value_ListValue{
												ListValue: &structpb.ListValue{
													Values: []*structpb.Value{
														{Kind: &structpb.Value_StringValue{StringValue: "Linux"}},
													},
												},
											},
										},
									},
								},
							},
						},
					},
				}
				expected := &pb.BuildInfra_Swarming{
					Hostname: "chromium-swarm.appspot.com",
					TaskId:   "s93k0402js90",
					Caches: []*pb.BuildInfra_Swarming_CacheEntry{
						{
							Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
							Path: "builder",
							WaitForWarmCache: &durationpb.Duration{
								Seconds: 240,
							},
						},
					},
					TaskDimensions:     taskDims,
					Priority:           int32(20),
					TaskServiceAccount: "account",
					BotDimensions: []*pb.StringPair{
						{
							Key:   "cpu",
							Value: "x86",
						},
						{
							Key:   "cpu",
							Value: "x86-64",
						},
						{
							Key:   "os",
							Value: "Linux",
						},
					},
				}
				assert.Loosely(t, tryBackfillSwarming(b), should.BeNil)
				assert.Loosely(t, b.Infra.Swarming, should.Match(expected))
			})
		})
	})
}

func TestTryBackfillBackend(t *testing.T) {
	t.Parallel()

	ftt.Run("tryBackfillBackend", t, func(t *ftt.Test) {
		b := &pb.Build{
			Id: 1,
			Builder: &pb.BuilderID{
				Project: "project",
				Bucket:  "bucket",
				Builder: "builder",
			},
			Status: pb.Status_SUCCESS,
			Infra:  &pb.BuildInfra{},
		}
		t.Run("noop", func(t *ftt.Test) {
			t.Run("no swarming", func(t *ftt.Test) {
				assert.Loosely(t, tryBackfillBackend(b), should.BeNil)
				assert.Loosely(t, b.Infra.Backend, should.BeNil)
			})

			t.Run("no swarming task", func(t *ftt.Test) {
				b.Infra.Swarming = &pb.BuildInfra_Swarming{
					Hostname: "host",
				}
				assert.Loosely(t, tryBackfillBackend(b), should.BeNil)
				assert.Loosely(t, b.Infra.Backend, should.BeNil)
			})
		})

		t.Run("backend backfilled", func(t *ftt.Test) {
			taskDims := []*pb.RequestedDimension{
				{
					Key:   "key",
					Value: "value",
				},
			}
			b.Infra.Swarming = &pb.BuildInfra_Swarming{
				Hostname: "chromium-swarm.appspot.com",
				TaskId:   "s93k0402js90",
				Caches: []*pb.BuildInfra_Swarming_CacheEntry{
					{
						Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
						Path: "builder",
						WaitForWarmCache: &durationpb.Duration{
							Seconds: 240,
						},
					},
				},
				TaskDimensions:     taskDims,
				Priority:           int32(20),
				TaskServiceAccount: "account",
				BotDimensions: []*pb.StringPair{
					{
						Key:   "cpu",
						Value: "x86",
					},
					{
						Key:   "cpu",
						Value: "x86-64",
					},
					{
						Key:   "os",
						Value: "Linux",
					},
				},
			}
			expected := &pb.BuildInfra_Backend{
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "s93k0402js90",
						Target: "swarming://chromium-swarm",
					},
				},
				Hostname: "chromium-swarm.appspot.com",
				Caches: []*pb.CacheEntry{
					{
						Name: "builder_1809c38861a9996b1748e4640234fbd089992359f6f23f62f68deb98528f5f2b_v2",
						Path: "builder",
						WaitForWarmCache: &durationpb.Duration{
							Seconds: 240,
						},
					},
				},
				Config: &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"priority":        {Kind: &structpb.Value_NumberValue{NumberValue: 20}},
						"service_account": {Kind: &structpb.Value_StringValue{StringValue: "account"}},
					},
				},
				TaskDimensions: taskDims,
			}

			expected.Task.Details = &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"bot_dimensions": {
						Kind: &structpb.Value_StructValue{
							StructValue: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"cpu": {
										Kind: &structpb.Value_ListValue{
											ListValue: &structpb.ListValue{
												Values: []*structpb.Value{
													{Kind: &structpb.Value_StringValue{StringValue: "x86"}},
													{Kind: &structpb.Value_StringValue{StringValue: "x86-64"}},
												},
											},
										},
									},
									"os": {
										Kind: &structpb.Value_ListValue{
											ListValue: &structpb.ListValue{
												Values: []*structpb.Value{
													{Kind: &structpb.Value_StringValue{StringValue: "Linux"}},
												},
											},
										},
									},
								},
							},
						},
					},
				},
			}
			assert.Loosely(t, tryBackfillBackend(b), should.BeNil)
			assert.Loosely(t, b.Infra.Backend, should.Match(expected))
		})
	})
}

type fakeBqClient struct {
	mu sync.RWMutex
	// data persists all inserted rows.
	data map[string][]*lucibq.Row
}

var fakeBqErrCtxKey = "used only in tests to make BQ return a fake error"

func (f *fakeBqClient) Insert(ctx context.Context, dataset, table string, row *lucibq.Row) error {
	if err, ok := ctx.Value(&fakeBqErrCtxKey).(error); ok {
		return err
	}
	if !buildIsSmallEnough(row.Message) {
		return &googleapi.Error{
			Code: http.StatusRequestEntityTooLarge,
		}
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	key := dataset + "." + table
	if f.data == nil {
		f.data = make(map[string][]*lucibq.Row)
	}
	f.data[key] = append(f.data[key], row)
	return nil
}

func (f *fakeBqClient) GetRows(dataset, table string) []*lucibq.Row {
	f.mu.RLock()
	defer f.mu.RUnlock()
	return f.data[dataset+"."+table]
}
