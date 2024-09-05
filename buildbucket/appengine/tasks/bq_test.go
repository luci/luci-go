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
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"

	. "go.chromium.org/luci/common/testing/assertions"
)

func TestBQ(t *testing.T) {
	t.Parallel()

	Convey("ExportBuild", t, func() {
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
		So(bs.FromProto([]*pb.Step{
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
		}), ShouldBeNil)
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
		So(datastore.Put(ctx, b, bi, bs), ShouldBeNil)

		Convey("build not found", func() {
			err := ExportBuild(ctx, 111)
			So(tq.Fatal.In(err), ShouldBeTrue)
			So(err, ShouldErrLike, "build 111 not found when exporting into BQ")
		})

		Convey("bad row", func() {
			ctx1 := context.WithValue(ctx, &fakeBqErrCtxKey, bigquery.PutMultiError{bigquery.RowInsertionError{}})
			err := ExportBuild(ctx1, 123)
			So(err, ShouldErrLike, "bad row for build 123")
			So(tq.Fatal.In(err), ShouldBeTrue)
		})

		Convey("transient BQ err", func() {
			ctx1 := context.WithValue(ctx, &fakeBqErrCtxKey, errors.New("transient"))
			err := ExportBuild(ctx1, 123)
			So(err, ShouldErrLike, "transient error when inserting BQ for build 123")
			So(transient.Tag.In(err), ShouldBeTrue)
		})

		Convey("output properties too large", func() {
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
			So(datastore.Put(ctx, bo), ShouldBeNil)

			So(ExportBuild(ctx, 123), ShouldBeNil)
			rows := fakeBq.GetRows("raw", "completed_builds")
			So(len(rows), ShouldEqual, 1)
			So(rows[0].InsertID, ShouldEqual, "123")
			p, _ := rows[0].Message.(*pb.Build)
			So(p, ShouldResembleProto, &pb.Build{
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
			})
		})

		Convey("strip step log as well if build is still too big", func() {
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
			So(datastore.Put(ctx, bo), ShouldBeNil)

			So(ExportBuild(ctx, 123), ShouldBeNil)
			rows := fakeBq.GetRows("raw", "completed_builds")
			So(len(rows), ShouldEqual, 1)
			So(rows[0].InsertID, ShouldEqual, "123")
			p, _ := rows[0].Message.(*pb.Build)
			So(p, ShouldResembleProto, &pb.Build{
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
			})
		})

		Convey("fail export if build is still too big", func() {
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
			So(datastore.Put(ctx, bo), ShouldBeNil)

			So(ExportBuild(ctx, 123), ShouldErrLike, errRowTooBig)
		})

		Convey("summary markdown and cancelation reason are concatenated", func() {
			b.Proto.SummaryMarkdown = "summary"
			b.Proto.CancellationMarkdown = "cancelled"
			So(datastore.Put(ctx, b), ShouldBeNil)

			So(ExportBuild(ctx, 123), ShouldBeNil)
			rows := fakeBq.GetRows("raw", "completed_builds")
			So(len(rows), ShouldEqual, 1)
			So(rows[0].InsertID, ShouldEqual, "123")
			p, _ := rows[0].Message.(*pb.Build)
			So(p, ShouldResembleProto, &pb.Build{
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
			})
		})

		Convey("success", func() {
			b.Proto.CancellationMarkdown = "cancelled"
			So(datastore.Put(ctx, b), ShouldBeNil)

			So(ExportBuild(ctx, 123), ShouldBeNil)
			rows := fakeBq.GetRows("raw", "completed_builds")
			So(len(rows), ShouldEqual, 1)
			So(rows[0].InsertID, ShouldEqual, "123")
			p, _ := rows[0].Message.(*pb.Build)
			So(p, ShouldResembleProto, &pb.Build{
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
			})
		})
	})
}

func TestTryBackfillSwarming(t *testing.T) {
	t.Parallel()

	Convey("tryBackfillSwarming", t, func() {
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
		Convey("noop", func() {
			Convey("no backend", func() {
				So(tryBackfillSwarming(b), ShouldBeNil)
				So(b.Infra.Swarming, ShouldBeNil)
			})

			Convey("no backend task", func() {
				b.Infra.Backend = &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Target: "swarming://chromium-swarm",
						},
					},
				}
				So(tryBackfillSwarming(b), ShouldBeNil)
				So(b.Infra.Swarming, ShouldBeNil)
			})

			Convey("not a swarming implemented backend", func() {
				b.Infra.Backend = &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Id:     "s93k0402js90",
							Target: "other://chromium-swarm",
						},
					},
				}
				So(tryBackfillSwarming(b), ShouldBeNil)
				So(b.Infra.Swarming, ShouldBeNil)
			})
		})

		Convey("swarming backfilled", func() {
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
						"priority":        &structpb.Value{Kind: &structpb.Value_NumberValue{NumberValue: 20}},
						"service_account": &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "account"}},
					},
				},
				TaskDimensions: taskDims,
			}
			Convey("partially fail", func() {
				b.Infra.Backend.Task.Details = &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"bot_dimensions": &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "wrong format"}},
					},
				}
				So(tryBackfillSwarming(b), ShouldErrLike, "failed to unmarshal task details JSON for build 1")
				So(b.Infra.Swarming.BotDimensions, ShouldHaveLength, 0)
			})

			Convey("pass", func() {
				b.Infra.Backend.Task.Details = &structpb.Struct{
					Fields: map[string]*structpb.Value{
						"bot_dimensions": &structpb.Value{
							Kind: &structpb.Value_StructValue{
								StructValue: &structpb.Struct{
									Fields: map[string]*structpb.Value{
										"cpu": &structpb.Value{
											Kind: &structpb.Value_ListValue{
												ListValue: &structpb.ListValue{
													Values: []*structpb.Value{
														&structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "x86"}},
														&structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "x86-64"}},
													},
												},
											},
										},
										"os": &structpb.Value{
											Kind: &structpb.Value_ListValue{
												ListValue: &structpb.ListValue{
													Values: []*structpb.Value{
														&structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "Linux"}},
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
				So(tryBackfillSwarming(b), ShouldBeNil)
				So(b.Infra.Swarming, ShouldResembleProto, expected)
			})
		})
	})
}

func TestTryBackfillBackend(t *testing.T) {
	t.Parallel()

	Convey("tryBackfillBackend", t, func() {
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
		Convey("noop", func() {
			Convey("no swarming", func() {
				So(tryBackfillBackend(b), ShouldBeNil)
				So(b.Infra.Backend, ShouldBeNil)
			})

			Convey("no swarming task", func() {
				b.Infra.Swarming = &pb.BuildInfra_Swarming{
					Hostname: "host",
				}
				So(tryBackfillBackend(b), ShouldBeNil)
				So(b.Infra.Backend, ShouldBeNil)
			})
		})

		Convey("backend backfilled", func() {
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
						"priority":        &structpb.Value{Kind: &structpb.Value_NumberValue{NumberValue: 20}},
						"service_account": &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "account"}},
					},
				},
				TaskDimensions: taskDims,
			}

			expected.Task.Details = &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"bot_dimensions": &structpb.Value{
						Kind: &structpb.Value_StructValue{
							StructValue: &structpb.Struct{
								Fields: map[string]*structpb.Value{
									"cpu": &structpb.Value{
										Kind: &structpb.Value_ListValue{
											ListValue: &structpb.ListValue{
												Values: []*structpb.Value{
													&structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "x86"}},
													&structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "x86-64"}},
												},
											},
										},
									},
									"os": &structpb.Value{
										Kind: &structpb.Value_ListValue{
											ListValue: &structpb.ListValue{
												Values: []*structpb.Value{
													&structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "Linux"}},
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
			So(tryBackfillBackend(b), ShouldBeNil)
			So(b.Infra.Backend, ShouldResembleProto, expected)
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
