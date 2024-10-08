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
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
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
)

// This will help track the number of times the cipd server is called to test if the cache is working as intended.
var numCipdCalls int

func describeBootstrapBundle(t testing.TB, hasBadPkgFile bool) http.HandlerFunc {
	// no t.Helper or LineContext because it just ends up indicating a function
	// deep inside of the http stack.
	return func(w http.ResponseWriter, r *http.Request) {
		assert.Loosely(t, r.URL.Path, should.Equal("/prpc/cipd.Repository/DescribeBootstrapBundle"))
		numCipdCalls++
		reqBody, err := io.ReadAll(r.Body)
		assert.Loosely(t, err, should.BeNil)
		req := &cipdpb.DescribeBootstrapBundleRequest{}
		err = proto.Unmarshal(reqBody, req)
		assert.Loosely(t, err, should.BeNil)
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
		assert.Loosely(t, err, should.BeNil)
	}
}

func helpTestCipdCall(t testing.TB, ctx context.Context, infra *pb.BuildInfra, expectedNumCalls int) {
	t.Helper()
	m, err := extractCipdDetails(ctx, "project", infra)
	assert.Loosely(t, err, should.BeNil, truth.LineContext())
	detail, ok := m["infra/tools/luci/bbagent/linux-amd64"]
	assert.Loosely(t, ok, should.BeTrue, truth.LineContext())
	assert.Loosely(t, detail, should.Resemble(&pb.RunTaskRequest_AgentExecutable_AgentSource{
		Sha256:    "this_is_a_sha_256_I_swear",
		SizeBytes: 100,
		Url:       "https://chrome-infra-packages.appspot.com/bootstrap/infra/tools/luci/bbagent/linux-amd64/+/latest",
	}), truth.LineContext())
	detail, ok = m["infra/tools/luci/bbagent/mac-amd64"]
	assert.Loosely(t, ok, should.BeTrue, truth.LineContext())
	assert.Loosely(t, detail, should.Resemble(&pb.RunTaskRequest_AgentExecutable_AgentSource{
		Sha256:    "this_is_a_sha_256_I_swear",
		SizeBytes: 100,
		Url:       "https://chrome-infra-packages.appspot.com/bootstrap/infra/tools/luci/bbagent/mac-amd64/+/latest",
	}), truth.LineContext())
	assert.Loosely(t, numCipdCalls, should.Equal(expectedNumCalls), truth.LineContext())
}

func TestCipdClient(t *testing.T) {
	t.Parallel()

	ftt.Run("extractCipdDetails", t, func(c *ftt.Test) {
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
		mockCipdServer := httptest.NewServer(describeBootstrapBundle(t, false))
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

		c.Run("ok", func(c *ftt.Test) {
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
			c.Run("no non-ok pkg file", func(c *ftt.Test) {
				numCipdCalls = 0
				// call extractCipdDetails function 10 times.
				// The test asserts that numCipdCalls should always be 1
				for i := 0; i < 10; i++ {
					helpTestCipdCall(c, ctx, infra, 1)
				}
			})
			c.Run("contain non-ok pkg file", func(c *ftt.Test) {
				mockCipdServer := httptest.NewServer(describeBootstrapBundle(t, true))
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
	ftt.Run("computeBackendNewTaskReq", t, func(c *ftt.Test) {
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
		server := httptest.NewServer(describeBootstrapBundle(t, false))
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

		c.Run("ok", func(c *ftt.Test) {
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
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, req.BackendConfig, should.Resemble(&structpb.Struct{
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
			}))
			assert.Loosely(c, req.BuildbucketHost, should.Equal("some unique host name"))
			assert.Loosely(c, req.BuildId, should.Equal("1"))
			assert.Loosely(c, req.Caches, should.Resemble([]*pb.CacheEntry{
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
			}))
			assert.Loosely(c, req.ExecutionTimeout, should.Resemble(&durationpb.Duration{Seconds: 500}))
			assert.Loosely(c, req.GracePeriod, should.Resemble(&durationpb.Duration{Seconds: 230}))
			assert.Loosely(c, req.Agent.Source["infra/tools/luci/bbagent/linux-amd64"], should.Resemble(&pb.RunTaskRequest_AgentExecutable_AgentSource{
				Sha256:    "this_is_a_sha_256_I_swear",
				SizeBytes: 100,
				Url:       "https://chrome-infra-packages.appspot.com/bootstrap/infra/tools/luci/bbagent/linux-amd64/+/latest",
			}))
			assert.Loosely(c, req.Agent.Source["infra/tools/luci/bbagent/mac-amd64"], should.Resemble(&pb.RunTaskRequest_AgentExecutable_AgentSource{
				Sha256:    "this_is_a_sha_256_I_swear",
				SizeBytes: 100,
				Url:       "https://chrome-infra-packages.appspot.com/bootstrap/infra/tools/luci/bbagent/mac-amd64/+/latest",
			}))
			assert.Loosely(c, req.AgentArgs, should.Resemble([]string{
				"-build-id", "1",
				"-host", "some unique host name",
				"-cache-base", "cache",
				"-context-file", "${BUILDBUCKET_AGENT_CONTEXT_FILE}",
			}))
			assert.Loosely(c, req.Dimensions, should.Resemble([]*pb.RequestedDimension{
				{
					Key:   "dim_key_1",
					Value: "dim_val_1",
				},
			}))
			assert.Loosely(c, req.StartDeadline.Seconds, should.Equal(1677511893))
			assert.Loosely(c, req.Experiments, should.Resemble([]string{
				"cow_eggs_experiment",
				"are_cow_eggs_real_experiment",
			}))
			assert.Loosely(c, req.PubsubTopic, should.Equal("projects/app-id/topics/chromium-swarm-dev-backend"))
		})
	})

	ftt.Run("RunTask", t, func(c *ftt.Test) {
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
		assert.Loosely(c, err, should.BeNil)
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
		assert.Loosely(c, datastore.Put(ctx, build, infra, bs), should.BeNil)

		c.Run("ok", func(c *ftt.Test) {
			mockTaskCreator.EXPECT().RunTask(gomock.Any(), gomock.Any()).Return(&pb.RunTaskResponse{
				Task: &pb.Task{
					Id:       &pb.TaskID{Id: "abc123", Target: "swarming://chromium-swarm"},
					Link:     "this_is_a_url_link",
					UpdateId: 1,
				},
			}, nil)
			err = CreateBackendTask(ctx, 1, "request_id")
			assert.Loosely(c, err, should.BeNil)
			eb := &model.Build{ID: build.ID}
			expectedBuildInfra := &model.BuildInfra{Build: key}
			assert.Loosely(c, datastore.Get(ctx, eb, expectedBuildInfra), should.BeNil)
			updateTime := eb.Proto.UpdateTime.AsTime()
			assert.Loosely(c, updateTime, should.Match(now))
			assert.Loosely(c, eb.BackendTarget, should.Equal("swarming://chromium-swarm"))
			assert.Loosely(c, eb.BackendSyncInterval, should.Equal(time.Duration(300)*time.Second))
			parts := strings.Split(eb.NextBackendSyncTime, "--")
			assert.Loosely(c, parts, should.HaveLength(4))
			assert.Loosely(c, parts[0], should.Equal(eb.BackendTarget))
			assert.Loosely(c, parts[1], should.Equal("project"))
			shardID, err := strconv.Atoi(parts[2])
			assert.Loosely(c, err, should.BeNil)
			assert.Loosely(c, shardID >= 0, should.BeTrue)
			assert.Loosely(c, shardID < 5, should.BeTrue)
			assert.Loosely(c, parts[3], should.Equal(fmt.Sprint(updateTime.Round(time.Minute).Add(eb.BackendSyncInterval).Unix())))
			assert.Loosely(c, expectedBuildInfra.Proto.Backend.Task, should.Resemble(&pb.Task{
				Id: &pb.TaskID{
					Id:     "abc123",
					Target: "swarming://chromium-swarm",
				},
				Link:     "this_is_a_url_link",
				UpdateId: 1,
			}))
			assert.Loosely(c, sch.Tasks(), should.BeEmpty)
		})

		c.Run("fail", func(c *ftt.Test) {
			mockTaskCreator.EXPECT().RunTask(gomock.Any(), gomock.Any()).Return(nil, status.Errorf(codes.InvalidArgument, "bad request"))
			bldr := &model.Builder{
				ID:     "builder",
				Parent: model.BucketKey(ctx, "project", "bucket"),
			}
			assert.Loosely(c, datastore.Put(ctx, bldr), should.BeNil)
			err = CreateBackendTask(ctx, 1, "request_id")
			expectedBuild := &model.Build{ID: 1}
			assert.Loosely(c, datastore.Get(ctx, expectedBuild), should.BeNil)
			assert.Loosely(c, err, should.ErrLike("failed to create a backend task"))
			assert.Loosely(c, expectedBuild.Proto.Status, should.Equal(pb.Status_INFRA_FAILURE))
			assert.Loosely(c, expectedBuild.Proto.SummaryMarkdown, should.ContainSubstring("Backend task creation failure."))
		})

		c.Run("bail out if the build has a task associated", func(c *ftt.Test) {
			infra.Proto.Backend.Task.Id.Id = "task"
			assert.Loosely(c, datastore.Put(ctx, infra), should.BeNil)
			err = CreateBackendTask(ctx, 1, "request_id")
			assert.Loosely(c, err, should.BeNil)
			expectedBuildInfra := &model.BuildInfra{Build: key}
			assert.Loosely(c, datastore.Get(ctx, expectedBuildInfra), should.BeNil)
			assert.Loosely(c, expectedBuildInfra.Proto.Backend.Task.Id.Id, should.Equal("task"))
			assert.Loosely(c, logs, convey.Adapt(memlogger.ShouldHaveLog)(
				logging.Info, "build 1 has associated with task"))
		})

		c.Run("give up after backend timeout", func(c *ftt.Test) {
			bldr := &model.Builder{
				ID:     "builder",
				Parent: model.BucketKey(ctx, "project", "bucket"),
			}
			assert.Loosely(c, datastore.Put(ctx, bldr), should.BeNil)
			now = now.Add(9 * time.Minute)
			ctx, _ = testclock.UseTime(ctx, now)
			err = CreateBackendTask(ctx, 1, "request_id")
			expectedBuild := &model.Build{ID: 1}
			assert.Loosely(c, datastore.Get(ctx, expectedBuild), should.BeNil)
			assert.Loosely(c, err, should.ErrLike("creating backend task for build 1 with requestID request_id has expired after 8m0s"))
			assert.Loosely(c, expectedBuild.Proto.Status, should.Equal(pb.Status_INFRA_FAILURE))
			assert.Loosely(c, expectedBuild.Proto.SummaryMarkdown, should.ContainSubstring("Backend task creation failure."))
		})

		c.Run("Lite backend", func(c *ftt.Test) {
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
			assert.Loosely(c, datastore.Put(ctx, build, infra, bkt, bldr), should.BeNil)

			boilerplate := func() {
				mockTaskCreator.EXPECT().RunTask(gomock.Any(), gomock.Any()).Return(&pb.RunTaskResponse{
					Task: &pb.Task{
						Id:       &pb.TaskID{Id: "abc123", Target: "lite://foo-lite"},
						Link:     "this_is_a_url_link",
						UpdateId: 1,
					},
				}, nil)
			}

			c.Run("ok", func(c *ftt.Test) {
				boilerplate()
				err = CreateBackendTask(ctx, 1, "request_id")
				assert.Loosely(c, err, should.BeNil)
				eb := &model.Build{ID: build.ID}
				expectedBuildInfra := &model.BuildInfra{Build: datastore.KeyForObj(ctx, build)}
				assert.Loosely(c, datastore.Get(ctx, eb, expectedBuildInfra), should.BeNil)
				updateTime := eb.Proto.UpdateTime.AsTime()
				assert.Loosely(c, updateTime, should.Match(now))
				assert.Loosely(c, expectedBuildInfra.Proto.Backend.Task, should.Resemble(&pb.Task{
					Id: &pb.TaskID{
						Id:     "abc123",
						Target: "lite://foo-lite",
					},
					Link:     "this_is_a_url_link",
					UpdateId: 1,
				}))
				tasks := sch.Tasks()
				assert.Loosely(c, tasks, should.HaveLength(1))
				assert.Loosely(c, tasks[0].Payload.(*taskdefs.CheckBuildLiveness).GetBuildId(), should.Equal(build.ID))
				assert.Loosely(c, tasks[0].Payload.(*taskdefs.CheckBuildLiveness).GetHeartbeatTimeout(), should.Equal(5))
				assert.Loosely(c, tasks[0].ETA, should.Match(now.Add(5*time.Second)))
			})

			c.Run("SchedulingTimeout shorter than heartbeat timeout", func(c *ftt.Test) {
				boilerplate()
				bldr.Config.HeartbeatTimeoutSecs = 60
				build.Proto.SchedulingTimeout = durationpb.New(10 * time.Second)
				assert.Loosely(c, datastore.Put(ctx, build, bldr), should.BeNil)

				err = CreateBackendTask(ctx, 1, "request_id")
				assert.Loosely(c, err, should.BeNil)
				tasks := sch.Tasks()
				assert.Loosely(c, tasks, should.HaveLength(1))
				assert.Loosely(c, tasks[0].Payload.(*taskdefs.CheckBuildLiveness).GetBuildId(), should.Equal(build.ID))
				assert.Loosely(c, tasks[0].Payload.(*taskdefs.CheckBuildLiveness).GetHeartbeatTimeout(), should.Equal(60))
				assert.Loosely(c, tasks[0].ETA, should.Match(now.Add(10*time.Second)))
			})

			c.Run("no heartbeat_timeout_secs field set", func(c *ftt.Test) {
				boilerplate()
				bldr.Config.HeartbeatTimeoutSecs = 0
				build.Proto.SchedulingTimeout = durationpb.New(10 * time.Second)
				assert.Loosely(c, datastore.Put(ctx, build, bldr), should.BeNil)

				err = CreateBackendTask(ctx, 1, "request_id")
				assert.Loosely(c, err, should.BeNil)
				tasks := sch.Tasks()
				assert.Loosely(c, tasks, should.HaveLength(1))
				assert.Loosely(c, tasks[0].Payload.(*taskdefs.CheckBuildLiveness).GetBuildId(), should.Equal(build.ID))
				assert.Loosely(c, tasks[0].Payload.(*taskdefs.CheckBuildLiveness).GetHeartbeatTimeout(), should.BeZero)
				assert.Loosely(c, tasks[0].ETA, should.Match(now.Add(10*time.Second)))
			})

			c.Run("builder not found", func(c *ftt.Test) {
				boilerplate()
				assert.Loosely(c, datastore.Delete(ctx, bldr), should.BeNil)
				err = CreateBackendTask(ctx, 1, "request_id")
				assert.Loosely(c, err, should.ErrLike("failed to fetch builder project/bucket/builder: datastore: no such entity"))
			})

			c.Run("in dynamic bucket", func(c *ftt.Test) {
				boilerplate()
				assert.Loosely(c, datastore.Delete(ctx, bldr, bkt), should.BeNil)
				bkt.Proto = &pb.Bucket{
					Name: "bucket",
					DynamicBuilderTemplate: &pb.Bucket_DynamicBuilderTemplate{
						Template: &pb.BuilderConfig{},
					},
				}
				assert.Loosely(c, datastore.Put(ctx, bkt), should.BeNil)
				err = CreateBackendTask(ctx, 1, "request_id")
				assert.Loosely(c, err, should.BeNil)
				tasks := sch.Tasks()
				assert.Loosely(c, tasks, should.HaveLength(1))
				assert.Loosely(c, tasks[0].Payload.(*taskdefs.CheckBuildLiveness).GetBuildId(), should.Equal(build.ID))
				assert.Loosely(c, tasks[0].Payload.(*taskdefs.CheckBuildLiveness).GetHeartbeatTimeout(), should.BeZero)
				assert.Loosely(c, tasks[0].ETA, should.Match(now.Add(1*time.Minute)))
			})
		})
	})
}
