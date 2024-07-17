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
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"

	codepb "google.golang.org/genproto/googleapis/rpc/code"
	statuspb "google.golang.org/genproto/googleapis/rpc/status"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	cipdpb "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/secrets"
	"go.chromium.org/luci/server/secrets/testsecrets"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/clients"
	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	taskdefs "go.chromium.org/luci/buildbucket/appengine/tasks/defs"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

// This will help track the number of times the cipd server is called to test if the cache is working as intended.
var numCipdCalls int

func describeBootstrapBundle(c C, hasBadPkgFile bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c.So(r.URL.Path, ShouldEqual, "/prpc/cipd.Repository/DescribeBootstrapBundle")
		numCipdCalls++
		reqBody, err := io.ReadAll(r.Body)
		c.So(err, ShouldBeNil)
		req := &cipdpb.DescribeBootstrapBundleRequest{}
		err = proto.Unmarshal(reqBody, req)
		c.So(err, ShouldBeNil)
		variants := []string{
			"linux-amd64",
			"mac-amd64",
		}
		bootstrapFiles := []*cipdpb.DescribeBootstrapBundleResponse_BootstrapFile{}
		for _, variant := range variants {
			pkdName := req.Prefix + "/" + variant
			bootstrapFile := &cipdpb.DescribeBootstrapBundleResponse_BootstrapFile{
				Package: pkdName,
				Size:    100,
				Instance: &cipdpb.ObjectRef{
					HashAlgo:  cipdpb.HashAlgo_SHA256,
					HexDigest: "this_is_a_sha_256_I_swear",
				},
			}
			bootstrapFiles = append(bootstrapFiles, bootstrapFile)
		}
		if hasBadPkgFile {
			bootstrapFiles = append(bootstrapFiles, &cipdpb.DescribeBootstrapBundleResponse_BootstrapFile{
				Package: req.Prefix + "/" + "bad-platform",
				Status: &statuspb.Status{
					Code:    int32(codepb.Code_NOT_FOUND),
					Message: "no such tag",
				},
			})
		}
		res := &cipdpb.DescribeBootstrapBundleResponse{
			Files: bootstrapFiles,
		}
		var buf []byte
		buf, _ = proto.Marshal(res)
		code := codes.OK
		status := http.StatusOK
		w.Header().Set("Content-Type", r.Header.Get("Accept"))
		w.Header().Set(prpc.HeaderGRPCCode, strconv.Itoa(int(code)))
		w.WriteHeader(status)
		_, err = w.Write(buf)
		c.So(err, ShouldBeNil)
	}
}

func helpTestCipdCall(c C, ctx context.Context, infra *pb.BuildInfra, expectedNumCalls int) {
	m, err := extractCipdDetails(ctx, "project", infra)
	c.So(err, ShouldBeNil)
	detail, ok := m["infra/tools/luci/bbagent/linux-amd64"]
	c.So(ok, ShouldBeTrue)
	c.So(detail, ShouldResembleProto, &pb.RunTaskRequest_AgentExecutable_AgentSource{
		Sha256:    "this_is_a_sha_256_I_swear",
		SizeBytes: 100,
		Url:       "https://chrome-infra-packages.appspot.com/bootstrap/infra/tools/luci/bbagent/linux-amd64/+/latest",
	})
	detail, ok = m["infra/tools/luci/bbagent/mac-amd64"]
	c.So(ok, ShouldBeTrue)
	c.So(detail, ShouldResembleProto, &pb.RunTaskRequest_AgentExecutable_AgentSource{
		Sha256:    "this_is_a_sha_256_I_swear",
		SizeBytes: 100,
		Url:       "https://chrome-infra-packages.appspot.com/bootstrap/infra/tools/luci/bbagent/mac-amd64/+/latest",
	})
	c.So(numCipdCalls, ShouldEqual, expectedNumCalls)
}

func TestCipdClient(t *testing.T) {
	t.Parallel()

	Convey("extractCipdDetails", t, func(c C) {
		now := testclock.TestRecentTimeUTC
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = memory.UseWithAppID(ctx, "dev~app-id")
		ctx = txndefer.FilterRDS(ctx)
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx, _ = metrics.WithCustomMetrics(ctx, &pb.SettingsCfg{})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, _ = tq.TestingContext(ctx, nil)
		mockCipdServer := httptest.NewServer(describeBootstrapBundle(c, false))
		defer mockCipdServer.Close()
		mockCipdClient := &prpc.Client{
			Host: strings.TrimPrefix(mockCipdServer.URL, "http://"),
			Options: &prpc.Options{
				Retry: func() retry.Iterator {
					return &retry.Limited{
						Retries: 3,
						Delay:   0,
					}
				},
				Insecure:  true,
				UserAgent: "prpc-test",
			},
		}
		ctx = context.WithValue(ctx, MockCipdClientKey{}, mockCipdClient)

		Convey("ok", func() {
			infra := &pb.BuildInfra{
				Backend: &pb.BuildInfra_Backend{
					Task: &pb.Task{
						Id: &pb.TaskID{
							Id:     "abc123",
							Target: "swarming://mytarget",
						},
					},
				},
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Agent: &pb.BuildInfra_Buildbucket_Agent{
						Source: &pb.BuildInfra_Buildbucket_Agent_Source{
							DataType: &pb.BuildInfra_Buildbucket_Agent_Source_Cipd{
								Cipd: &pb.BuildInfra_Buildbucket_Agent_Source_CIPD{
									Package: "infra/tools/luci/bbagent/${platform}",
									Version: "latest",
									Server:  "https://chrome-infra-packages.appspot.com",
								},
							},
						},
					},
					Hostname: "some unique host name",
				},
			}
			Convey("no non-ok pkg file", func() {
				numCipdCalls = 0
				// call extractCipdDetails function 10 times.
				// The test asserts that numCipdCalls should always be 1
				for i := 0; i < 10; i++ {
					helpTestCipdCall(c, ctx, infra, 1)
				}
			})
			Convey("contain non-ok pkg file", func() {
				mockCipdServer := httptest.NewServer(describeBootstrapBundle(c, true))
				defer mockCipdServer.Close()
				mockCipdClient := &prpc.Client{
					Host: strings.TrimPrefix(mockCipdServer.URL, "http://"),
					Options: &prpc.Options{
						Retry: func() retry.Iterator {
							return &retry.Limited{
								Retries: 3,
								Delay:   0,
							}
						},
						Insecure:  true,
						UserAgent: "prpc-test",
					},
				}
				ctx = context.WithValue(ctx, MockCipdClientKey{}, mockCipdClient)
				numCipdCalls = 0
				helpTestCipdCall(c, ctx, infra, 1)
				// make the cuurent time longer than the cache TTL.
				ctx, _ = testclock.UseTime(ctx, now.Add(5*time.Minute))
				helpTestCipdCall(c, ctx, infra, 2)
			})
		})
	})
}

func TestCreateBackendTask(t *testing.T) {
	Convey("computeBackendNewTaskReq", t, func(c C) {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockTaskCreator := clients.NewMockTaskCreator(ctl)
		now := testclock.TestRecentTimeUTC
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = context.WithValue(ctx, clients.MockTaskCreatorKey, mockTaskCreator)
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = memory.UseWithAppID(ctx, "dev~app-id")
		ctx = txndefer.FilterRDS(ctx)
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx, _ = metrics.WithCustomMetrics(ctx, &pb.SettingsCfg{})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, _ = tq.TestingContext(ctx, nil)
		store := &testsecrets.Store{
			Secrets: map[string]secrets.Secret{
				"key": {Active: []byte("stuff")},
			},
		}
		ctx = secrets.Use(ctx, store)
		ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)

		backendSetting := []*pb.BackendSetting{}
		backendSetting = append(backendSetting, &pb.BackendSetting{
			Target:   "swarming:/chromium-swarm-dev",
			Hostname: "chromium-swarm-dev",
			Mode: &pb.BackendSetting_FullMode_{
				FullMode: &pb.BackendSetting_FullMode{
					PubsubId: "chromium-swarm-dev-backend",
				},
			},
		})
		settingsCfg := &pb.SettingsCfg{Backends: backendSetting}
		server := httptest.NewServer(describeBootstrapBundle(c, false))
		defer server.Close()
		client := &prpc.Client{
			Host: strings.TrimPrefix(server.URL, "http://"),
			Options: &prpc.Options{
				Retry: func() retry.Iterator {
					return &retry.Limited{
						Retries: 3,
						Delay:   0,
					}
				},
				Insecure:  true,
				UserAgent: "prpc-test",
			},
		}
		ctx = context.WithValue(ctx, MockCipdClientKey{}, client)

		Convey("ok", func() {
			build := &model.Build{
				ID:        1,
				BucketID:  "project/bucket",
				Project:   "project",
				BuilderID: "project/bucket",
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Builder: "builder",
						Bucket:  "bucket",
						Project: "project",
					},
					CreateTime: &timestamppb.Timestamp{
						Seconds: 1677511793,
					},
					SchedulingTimeout: &durationpb.Duration{Seconds: 100},
					ExecutionTimeout:  &durationpb.Duration{Seconds: 500},
					Input: &pb.Build_Input{
						Experiments: []string{
							"cow_eggs_experiment",
							"are_cow_eggs_real_experiment",
						},
					},
					GracePeriod: &durationpb.Duration{Seconds: 50},
				},
			}
			key := datastore.KeyForObj(ctx, build)
			infra := &model.BuildInfra{
				Build: key,
				Proto: &pb.BuildInfra{
					Backend: &pb.BuildInfra_Backend{
						Caches: []*pb.CacheEntry{
							{
								Name: "cache_name",
								Path: "cache_value",
							},
						},
						Config: &structpb.Struct{
							Fields: map[string]*structpb.Value{
								"priority": {
									Kind: &structpb.Value_NumberValue{NumberValue: 32},
								},
								"bot_ping_tolerance": {
									Kind: &structpb.Value_NumberValue{NumberValue: 2},
								},
							},
						},
						Task: &pb.Task{
							Id: &pb.TaskID{
								Id:     "",
								Target: "swarming:/chromium-swarm-dev",
							},
						},
						TaskDimensions: []*pb.RequestedDimension{
							{
								Key:   "dim_key_1",
								Value: "dim_val_1",
							},
						},
					},
					Bbagent: &pb.BuildInfra_BBAgent{
						CacheDir: "cache",
					},
					Buildbucket: &pb.BuildInfra_Buildbucket{
						Hostname: "some unique host name",
						Agent: &pb.BuildInfra_Buildbucket_Agent{
							Source: &pb.BuildInfra_Buildbucket_Agent_Source{
								DataType: &pb.BuildInfra_Buildbucket_Agent_Source_Cipd{
									Cipd: &pb.BuildInfra_Buildbucket_Agent_Source_CIPD{
										Package: "infra/tools/luci/bbagent/${platform}",
										Version: "latest",
										Server:  "https://chrome-infra-packages.appspot.com",
									},
								},
							},
							CipdClientCache: &pb.CacheEntry{
								Name: "cipd_client_hash",
								Path: "cipd_client",
							},
							CipdPackagesCache: &pb.CacheEntry{
								Name: "cipd_cache_hash",
								Path: "cipd_cache",
							},
						},
					},
				},
			}
			req, err := computeBackendNewTaskReq(ctx, build, infra, "request_id", settingsCfg)
			So(err, ShouldBeNil)
			So(req.BackendConfig, ShouldResembleProto, &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"priority": {
						Kind: &structpb.Value_NumberValue{NumberValue: 32},
					},
					"bot_ping_tolerance": {
						Kind: &structpb.Value_NumberValue{NumberValue: 2},
					},
					"tags": {
						Kind: &structpb.Value_ListValue{
							ListValue: &structpb.ListValue{
								Values: []*structpb.Value{
									{Kind: &structpb.Value_StringValue{StringValue: "buildbucket_bucket:project/bucket"}},
									{Kind: &structpb.Value_StringValue{StringValue: "buildbucket_build_id:1"}},
									{Kind: &structpb.Value_StringValue{StringValue: "buildbucket_hostname:some unique host name"}},
									{Kind: &structpb.Value_StringValue{StringValue: "buildbucket_template_canary:0"}},
									{Kind: &structpb.Value_StringValue{StringValue: "luci_project:project"}},
								},
							},
						},
					},
					"task_name": &structpb.Value{Kind: &structpb.Value_StringValue{StringValue: "bb-1-project/bucket"}},
				},
			})
			So(req.BuildbucketHost, ShouldEqual, "some unique host name")
			So(req.BuildId, ShouldEqual, "1")
			So(req.Caches, ShouldResembleProto, []*pb.CacheEntry{
				{
					Name: "cache_name",
					Path: "cache_value",
				},
				{
					Name: "cipd_client_hash",
					Path: "cipd_client",
				},
				{
					Name: "cipd_cache_hash",
					Path: "cipd_cache",
				},
			})
			So(req.ExecutionTimeout, ShouldResembleProto, &durationpb.Duration{Seconds: 500})
			So(req.GracePeriod, ShouldResembleProto, &durationpb.Duration{Seconds: 230})
			So(req.Agent.Source["infra/tools/luci/bbagent/linux-amd64"], ShouldResembleProto, &pb.RunTaskRequest_AgentExecutable_AgentSource{
				Sha256:    "this_is_a_sha_256_I_swear",
				SizeBytes: 100,
				Url:       "https://chrome-infra-packages.appspot.com/bootstrap/infra/tools/luci/bbagent/linux-amd64/+/latest",
			})
			So(req.Agent.Source["infra/tools/luci/bbagent/mac-amd64"], ShouldResembleProto, &pb.RunTaskRequest_AgentExecutable_AgentSource{
				Sha256:    "this_is_a_sha_256_I_swear",
				SizeBytes: 100,
				Url:       "https://chrome-infra-packages.appspot.com/bootstrap/infra/tools/luci/bbagent/mac-amd64/+/latest",
			})
			So(req.AgentArgs, ShouldResemble, []string{
				"-build-id", "1",
				"-host", "some unique host name",
				"-cache-base", "cache",
				"-context-file", "${BUILDBUCKET_AGENT_CONTEXT_FILE}",
			})
			So(req.Dimensions, ShouldResembleProto, []*pb.RequestedDimension{
				{
					Key:   "dim_key_1",
					Value: "dim_val_1",
				},
			})
			So(req.StartDeadline.Seconds, ShouldEqual, 1677511893)
			So(req.Experiments, ShouldResemble, []string{
				"cow_eggs_experiment",
				"are_cow_eggs_real_experiment",
			})
			So(req.PubsubTopic, ShouldEqual, "projects/app-id/topics/chromium-swarm-dev-backend")
		})
	})

	Convey("RunTask", t, func(c C) {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockTaskCreator := clients.NewMockTaskCreator(ctl)
		now := testclock.TestRecentTimeUTC
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = context.WithValue(ctx, clients.MockTaskCreatorKey, mockTaskCreator)
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = memory.UseWithAppID(ctx, "dev~app-id")
		ctx = txndefer.FilterRDS(ctx)
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		ctx, _ = metrics.WithCustomMetrics(ctx, &pb.SettingsCfg{})
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, _ = tq.TestingContext(ctx, nil)
		store := &testsecrets.Store{
			Secrets: map[string]secrets.Secret{
				"key": {Active: []byte("stuff")},
			},
		}
		ctx = secrets.Use(ctx, store)
		ctx = secrets.GeneratePrimaryTinkAEADForTest(ctx)
		ctx = memlogger.Use(ctx)
		ctx, sch := tq.TestingContext(txndefer.FilterRDS(ctx), nil)
		logs := logging.Get(ctx).(*memlogger.MemLogger)

		backendSetting := []*pb.BackendSetting{}
		backendSetting = append(backendSetting, &pb.BackendSetting{
			Target:   "fail_me",
			Hostname: "hostname",
		})
		backendSetting = append(backendSetting, &pb.BackendSetting{
			Target:   "swarming://chromium-swarm",
			Hostname: "hostname2",
			Mode: &pb.BackendSetting_FullMode_{
				FullMode: &pb.BackendSetting_FullMode{
					BuildSyncSetting: &pb.BackendSetting_BuildSyncSetting{
						Shards:              5,
						SyncIntervalSeconds: 300,
					},
				},
			},
			TaskCreatingTimeout: durationpb.New(8 * time.Minute),
		})
		backendSetting = append(backendSetting, &pb.BackendSetting{
			Target:   "lite://foo-lite",
			Hostname: "foo-hostname",
			Mode: &pb.BackendSetting_LiteMode_{
				LiteMode: &pb.BackendSetting_LiteMode{},
			},
		})
		settingsCfg := &pb.SettingsCfg{Backends: backendSetting}
		err := config.SetTestSettingsCfg(ctx, settingsCfg)
		So(err, ShouldBeNil)
		server := httptest.NewServer(describeBootstrapBundle(c, false))
		defer server.Close()
		client := &prpc.Client{
			Host: strings.TrimPrefix(server.URL, "http://"),
			Options: &prpc.Options{
				Retry: func() retry.Iterator {
					return &retry.Limited{
						Retries: 3,
						Delay:   0,
					}
				},
				Insecure:  true,
				UserAgent: "prpc-test",
			},
		}
		ctx = context.WithValue(ctx, MockCipdClientKey{}, client)

		build := &model.Build{
			ID: 1,
			Proto: &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Builder: "builder",
					Bucket:  "bucket",
					Project: "project",
				},
				CreateTime:       timestamppb.New(now.Add(-5 * time.Minute)),
				ExecutionTimeout: &durationpb.Duration{Seconds: 500},
				Input: &pb.Build_Input{
					Experiments: []string{
						"cow_eggs_experiment",
						"are_cow_eggs_real_experiment",
					},
				},
				GracePeriod: &durationpb.Duration{Seconds: 50},
			},
		}
		key := datastore.KeyForObj(ctx, build)
		infra := &model.BuildInfra{
			Build: key,
			Proto: &pb.BuildInfra{
				Backend: &pb.BuildInfra_Backend{
					Caches: []*pb.CacheEntry{
						{
							Name: "cache_name",
							Path: "cache_value",
						},
					},
					Config: &structpb.Struct{
						Fields: map[string]*structpb.Value{
							"priority": {
								Kind: &structpb.Value_NumberValue{NumberValue: 32},
							},
							"bot_ping_tolerance": {
								Kind: &structpb.Value_NumberValue{NumberValue: 2},
							},
						},
					},
					Task: &pb.Task{
						Id: &pb.TaskID{
							Id:     "",
							Target: "swarming://chromium-swarm",
						},
					},
					TaskDimensions: []*pb.RequestedDimension{
						{
							Key:   "dim_key_1",
							Value: "dim_val_1",
						},
					},
				},
				Bbagent: &pb.BuildInfra_BBAgent{
					CacheDir: "cache",
				},
				Buildbucket: &pb.BuildInfra_Buildbucket{
					Hostname: "some unique host name",
					Agent: &pb.BuildInfra_Buildbucket_Agent{
						Source: &pb.BuildInfra_Buildbucket_Agent_Source{
							DataType: &pb.BuildInfra_Buildbucket_Agent_Source_Cipd{
								Cipd: &pb.BuildInfra_Buildbucket_Agent_Source_CIPD{
									Package: "infra/tools/luci/bbagent/${platform}",
									Version: "latest",
									Server:  "https://chrome-infra-packages.appspot.com",
								},
							},
						},
					},
				},
			},
		}
		bs := &model.BuildStatus{Build: datastore.KeyForObj(ctx, build)}
		So(datastore.Put(ctx, build, infra, bs), ShouldBeNil)

		Convey("ok", func() {
			mockTaskCreator.EXPECT().RunTask(gomock.Any(), gomock.Any()).Return(&pb.RunTaskResponse{
				Task: &pb.Task{
					Id:       &pb.TaskID{Id: "abc123", Target: "swarming://chromium-swarm"},
					Link:     "this_is_a_url_link",
					UpdateId: 1,
				},
			}, nil)
			err = CreateBackendTask(ctx, 1, "request_id")
			So(err, ShouldBeNil)
			eb := &model.Build{ID: build.ID}
			expectedBuildInfra := &model.BuildInfra{Build: key}
			So(datastore.Get(ctx, eb, expectedBuildInfra), ShouldBeNil)
			updateTime := eb.Proto.UpdateTime.AsTime()
			So(updateTime, ShouldEqual, now)
			So(eb.BackendTarget, ShouldEqual, "swarming://chromium-swarm")
			So(eb.BackendSyncInterval, ShouldEqual, time.Duration(300)*time.Second)
			parts := strings.Split(eb.NextBackendSyncTime, "--")
			So(parts, ShouldHaveLength, 4)
			So(parts[0], ShouldEqual, eb.BackendTarget)
			So(parts[1], ShouldEqual, "project")
			shardID, err := strconv.Atoi(parts[2])
			So(err, ShouldBeNil)
			So(shardID >= 0, ShouldBeTrue)
			So(shardID < 5, ShouldBeTrue)
			So(parts[3], ShouldEqual, fmt.Sprint(updateTime.Round(time.Minute).Add(eb.BackendSyncInterval).Unix()))
			So(expectedBuildInfra.Proto.Backend.Task, ShouldResembleProto, &pb.Task{
				Id: &pb.TaskID{
					Id:     "abc123",
					Target: "swarming://chromium-swarm",
				},
				Link:     "this_is_a_url_link",
				UpdateId: 1,
			})
			So(sch.Tasks(), ShouldBeEmpty)
		})

		Convey("fail", func() {
			mockTaskCreator.EXPECT().RunTask(gomock.Any(), gomock.Any()).Return(nil, status.Errorf(codes.InvalidArgument, "bad request"))
			err = CreateBackendTask(ctx, 1, "request_id")
			expectedBuild := &model.Build{ID: 1}
			So(datastore.Get(ctx, expectedBuild), ShouldBeNil)
			So(err, ShouldErrLike, "failed to create a backend task")
			So(expectedBuild.Proto.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
			So(expectedBuild.Proto.SummaryMarkdown, ShouldContainSubstring, "Backend task creation failure.")
		})

		Convey("bail out if the build has a task associated", func() {
			infra.Proto.Backend.Task.Id.Id = "task"
			So(datastore.Put(ctx, infra), ShouldBeNil)
			err = CreateBackendTask(ctx, 1, "request_id")
			So(err, ShouldBeNil)
			expectedBuildInfra := &model.BuildInfra{Build: key}
			So(datastore.Get(ctx, expectedBuildInfra), ShouldBeNil)
			So(expectedBuildInfra.Proto.Backend.Task.Id.Id, ShouldEqual, "task")
			So(logs, memlogger.ShouldHaveLog,
				logging.Info, "build 1 has associated with task")
		})

		Convey("give up after backend timeout", func() {
			now = now.Add(9 * time.Minute)
			ctx, _ = testclock.UseTime(ctx, now)
			err = CreateBackendTask(ctx, 1, "request_id")
			expectedBuild := &model.Build{ID: 1}
			So(datastore.Get(ctx, expectedBuild), ShouldBeNil)
			So(err, ShouldErrLike, "creating backend task for build 1 with requestID request_id has expired after 8m0s")
			So(expectedBuild.Proto.Status, ShouldEqual, pb.Status_INFRA_FAILURE)
			So(expectedBuild.Proto.SummaryMarkdown, ShouldContainSubstring, "Backend task creation failure.")
		})

		Convey("Lite backend", func() {
			bkt := &model.Bucket{
				ID:     "bucket",
				Parent: model.ProjectKey(ctx, "project"),
			}
			bldr := &model.Builder{
				ID:     "builder",
				Parent: datastore.KeyForObj(ctx, bkt),
				Config: &pb.BuilderConfig{
					Name:                 "builder",
					HeartbeatTimeoutSecs: 5,
				},
			}
			build.Proto.SchedulingTimeout = durationpb.New(1 * time.Minute)
			infra.Proto.Backend.Task.Id.Target = "lite://foo-lite"
			So(datastore.Put(ctx, build, infra, bkt, bldr), ShouldBeNil)

			mockTaskCreator.EXPECT().RunTask(gomock.Any(), gomock.Any()).Return(&pb.RunTaskResponse{
				Task: &pb.Task{
					Id:       &pb.TaskID{Id: "abc123", Target: "lite://foo-lite"},
					Link:     "this_is_a_url_link",
					UpdateId: 1,
				},
			}, nil)

			Convey("ok", func() {
				err = CreateBackendTask(ctx, 1, "request_id")
				So(err, ShouldBeNil)
				eb := &model.Build{ID: build.ID}
				expectedBuildInfra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, build)}
				So(datastore.Get(ctx, eb, expectedBuildInfra), ShouldBeNil)
				updateTime := eb.Proto.UpdateTime.AsTime()
				So(updateTime, ShouldEqual, now)
				So(expectedBuildInfra.Proto.Backend.Task, ShouldResembleProto, &pb.Task{
					Id: &pb.TaskID{
						Id:     "abc123",
						Target: "lite://foo-lite",
					},
					Link:     "this_is_a_url_link",
					UpdateId: 1,
				})
				tasks := sch.Tasks()
				So(tasks, ShouldHaveLength, 1)
				So(tasks[0].Payload.(*taskdefs.CheckBuildLiveness).GetBuildId(), ShouldEqual, build.ID)
				So(tasks[0].Payload.(*taskdefs.CheckBuildLiveness).GetHeartbeatTimeout(), ShouldEqual, 5)
				So(tasks[0].ETA, ShouldEqual, now.Add(5*time.Second))
			})

			Convey("SchedulingTimeout shorter than heartbeat timeout", func() {
				bldr.Config.HeartbeatTimeoutSecs = 60
				build.Proto.SchedulingTimeout = durationpb.New(10 * time.Second)
				So(datastore.Put(ctx, build, bldr), ShouldBeNil)

				err = CreateBackendTask(ctx, 1, "request_id")
				So(err, ShouldBeNil)
				tasks := sch.Tasks()
				So(tasks, ShouldHaveLength, 1)
				So(tasks[0].Payload.(*taskdefs.CheckBuildLiveness).GetBuildId(), ShouldEqual, build.ID)
				So(tasks[0].Payload.(*taskdefs.CheckBuildLiveness).GetHeartbeatTimeout(), ShouldEqual, 60)
				So(tasks[0].ETA, ShouldEqual, now.Add(10*time.Second))
			})

			Convey("no heartbeat_timeout_secs field set", func() {
				bldr.Config.HeartbeatTimeoutSecs = 0
				build.Proto.SchedulingTimeout = durationpb.New(10 * time.Second)
				So(datastore.Put(ctx, build, bldr), ShouldBeNil)

				err = CreateBackendTask(ctx, 1, "request_id")
				So(err, ShouldBeNil)
				tasks := sch.Tasks()
				So(tasks, ShouldHaveLength, 1)
				So(tasks[0].Payload.(*taskdefs.CheckBuildLiveness).GetBuildId(), ShouldEqual, build.ID)
				So(tasks[0].Payload.(*taskdefs.CheckBuildLiveness).GetHeartbeatTimeout(), ShouldEqual, 0)
				So(tasks[0].ETA, ShouldEqual, now.Add(10*time.Second))
			})

			Convey("builder not found", func() {
				So(datastore.Delete(ctx, bldr), ShouldBeNil)
				err = CreateBackendTask(ctx, 1, "request_id")
				So(err, ShouldErrLike, "failed to fetch builder project/bucket/builder: datastore: no such entity")

			})

			Convey("in dynamic bucket", func() {
				So(datastore.Delete(ctx, bldr, bkt), ShouldBeNil)
				bkt.Proto = &pb.Bucket{
					Name: "bucket",
					DynamicBuilderTemplate: &pb.Bucket_DynamicBuilderTemplate{
						Template: &pb.BuilderConfig{},
					},
				}
				So(datastore.Put(ctx, bkt), ShouldBeNil)
				err = CreateBackendTask(ctx, 1, "request_id")
				So(err, ShouldBeNil)
				tasks := sch.Tasks()
				So(tasks, ShouldHaveLength, 1)
				So(tasks[0].Payload.(*taskdefs.CheckBuildLiveness).GetBuildId(), ShouldEqual, build.ID)
				So(tasks[0].Payload.(*taskdefs.CheckBuildLiveness).GetHeartbeatTimeout(), ShouldEqual, 0)
				So(tasks[0].ETA, ShouldEqual, now.Add(1*time.Minute))
			})
		})
	})
}
