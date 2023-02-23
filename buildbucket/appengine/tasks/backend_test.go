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
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"

	cipdpb "go.chromium.org/luci/cipd/api/cipd/v1"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/gae/filter/txndefer"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/prpc"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/internal/config"
	"go.chromium.org/luci/buildbucket/appengine/internal/metrics"
	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type MockedClient struct {
	Client *MockTaskBackendClient
	Ctx    context.Context
}

// MockTaskBackendClient is a mock of TaskBackendClient interface.
type MockTaskBackendClient struct {
	ctrl     *gomock.Controller
	recorder *MockTaskBackendClientMockRecorder
}

// MockTaskBackendClientMockRecorder is the mock recorder for MockTaskBackendClient.
type MockTaskBackendClientMockRecorder struct {
	mock *MockTaskBackendClient
}

// NewMockTaskBackendClient creates a new mock instance.
func NewMockTaskBackendClient(ctrl *gomock.Controller) *MockTaskBackendClient {
	mock := &MockTaskBackendClient{ctrl: ctrl}
	mock.recorder = &MockTaskBackendClientMockRecorder{mock}
	return mock
}

// NewMockedClient creates a MockedClient for testing.
func NewMockedClient(ctx context.Context, ctl *gomock.Controller) *MockedClient {
	mockClient := NewMockTaskBackendClient(ctl)
	return &MockedClient{
		Client: mockClient,
		Ctx:    useTaskBackendClientForTesting(ctx, mockClient),
	}
}

// RunTask Mocks the RunTask RPC.
func (mc *MockedClient) RunTask(ctx context.Context, taskReq *pb.RunTaskRequest, opts ...grpc.CallOption) (*emptypb.Empty, error) {
	return new(emptypb.Empty), nil
}

// useTaskBackendClientForTesting specifies that the given test double shall be used
// instead of making calls to TaskBackend.
func useTaskBackendClientForTesting(ctx context.Context, client *MockTaskBackendClient) context.Context {
	return context.WithValue(ctx, MockTaskBackendClientKey{}, client)
}

func describeBootstrapBundle(c C) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c.So(r.URL.Path, ShouldEqual, "/prpc/cipd.Repository/DescribeBootstrapBundle")
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

func TestBackendTaskClient(t *testing.T) {
	t.Parallel()

	Convey("assert NewBackendClient", t, func() {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mc := NewMockedClient(context.Background(), ctl)
		now := testclock.TestRecentTimeUTC
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = context.WithValue(ctx, MockTaskBackendClientKey{}, mc)
		ctx = memory.UseWithAppID(ctx, "dev~app-id")
		ctx = txndefer.FilterRDS(ctx)
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, _ = tq.TestingContext(ctx, nil)

		build := &pb.Build{
			Builder: &pb.BuilderID{
				Builder: "builder",
				Bucket:  "bucket",
				Project: "project",
			},
			Id: 1,
		}

		infra := &pb.BuildInfra{
			Backend: &pb.BuildInfra_Backend{
				Task: &pb.Task{
					Id: &pb.TaskID{
						Id:     "1",
						Target: "swarming://mytarget",
					},
				},
			},
			Buildbucket: &pb.BuildInfra_Buildbucket{
				Hostname: "some unique host name",
			},
		}

		Convey("global settings not defined", func() {
			_, err := NewBackendClient(ctx, build, infra)
			So(err, ShouldErrLike, "could not get global settings config")
		})

		Convey("target not in global config", func() {
			backendSetting := []*pb.BackendSetting{}
			settingsCfg := &pb.SettingsCfg{Backends: backendSetting}
			err := config.SetTestSettingsCfg(ctx, settingsCfg)
			So(err, ShouldBeNil)
			_, err = NewBackendClient(ctx, build, infra)
			So(err, ShouldErrLike, "could not find target in global config settings")
		})

		Convey("target is in global config", func() {
			backendSetting := []*pb.BackendSetting{}
			backendSetting = append(backendSetting, &pb.BackendSetting{
				Target:   "swarming://mytarget",
				Hostname: "hostname",
			})
			settingsCfg := &pb.SettingsCfg{Backends: backendSetting}
			err := config.SetTestSettingsCfg(ctx, settingsCfg)
			So(err, ShouldBeNil)
			_, err = NewBackendClient(ctx, build, infra)
			So(err, ShouldBeNil)
		})
	})
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
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, _ = tq.TestingContext(ctx, nil)
		mockCipdServer := httptest.NewServer(describeBootstrapBundle(c))
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
							Id:     "1",
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
			m, err := extractCipdDetails(ctx, "project", infra)
			So(err, ShouldBeNil)
			detail, ok := m["infra/tools/luci/bbagent/linux-amd64"]
			So(ok, ShouldBeTrue)
			So(detail, ShouldResembleProto, &pb.RunTaskRequest_AgentExecutable_AgentSource{
				Sha256:    "this_is_a_sha_256_I_swear",
				SizeBytes: 100,
				Url:       "https://chrome-infra-packages.appspot.com/bootstrap/infra/tools/luci/bbagent/linux-amd64/+/latest",
			})
			detail, ok = m["infra/tools/luci/bbagent/mac-amd64"]
			So(ok, ShouldBeTrue)
			So(detail, ShouldResembleProto, &pb.RunTaskRequest_AgentExecutable_AgentSource{
				Sha256:    "this_is_a_sha_256_I_swear",
				SizeBytes: 100,
				Url:       "https://chrome-infra-packages.appspot.com/bootstrap/infra/tools/luci/bbagent/mac-amd64/+/latest",
			})
		})
	})
}

func TestCreateBackendTask(t *testing.T) {
	Convey("computeBackendNewTaskReq", t, func(c C) {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mc := NewMockedClient(context.Background(), ctl)
		now := testclock.TestRecentTimeUTC
		ctx, _ := testclock.UseTime(context.Background(), now)
		ctx = context.WithValue(ctx, MockTaskBackendClientKey{}, mc)
		ctx = caching.WithEmptyProcessCache(ctx)
		ctx = memory.UseWithAppID(ctx, "dev~app-id")
		ctx = txndefer.FilterRDS(ctx)
		ctx = metrics.WithServiceInfo(ctx, "svc", "job", "ins")
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)
		ctx, _ = tq.TestingContext(ctx, nil)
		backendSetting := []*pb.BackendSetting{}
		backendSetting = append(backendSetting, &pb.BackendSetting{
			Target:   "swarming://mytarget",
			Hostname: "hostname",
		})
		settingsCfg := &pb.SettingsCfg{Backends: backendSetting}
		err := config.SetTestSettingsCfg(ctx, settingsCfg)
		So(err, ShouldBeNil)
		server := httptest.NewServer(describeBootstrapBundle(c))
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
				ID: 1,
				Proto: &pb.Build{
					Id: 1,
					Builder: &pb.BuilderID{
						Builder: "builder",
						Bucket:  "bucket",
						Project: "project",
					},
					ExecutionTimeout: &durationpb.Duration{Seconds: 500},
					GracePeriod:      &durationpb.Duration{Seconds: 50},
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
			req, err := computeBackendNewTaskReq(ctx, build, infra)
			So(err, ShouldBeNil)
			So(req.BackendConfig, ShouldResembleProto, &structpb.Struct{
				Fields: map[string]*structpb.Value{
					"priority": {
						Kind: &structpb.Value_NumberValue{NumberValue: 32},
					},
					"bot_ping_tolerance": {
						Kind: &structpb.Value_NumberValue{NumberValue: 2},
					},
				},
			})
			So(req.BuildbucketHost, ShouldEqual, "some unique host name")
			So(req.BuildId, ShouldEqual, "1")
			So(req.Caches, ShouldResembleProto, []*pb.CacheEntry{
				{
					Name: "cache_name",
					Path: "cache_value",
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
				"-cache-base", "cache"})
		})
	})
}
