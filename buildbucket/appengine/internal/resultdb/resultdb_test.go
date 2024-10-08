// Copyright 2021 The LUCI Authors.
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

package resultdb

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"

	"github.com/golang/mock/gomock"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	grpcStatus "google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	rdbPb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/tq"

	"go.chromium.org/luci/buildbucket/appengine/model"
	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestCreateInvocations(t *testing.T) {
	t.Parallel()

	ftt.Run("create invocations", t, func(t *ftt.Test) {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := rdbPb.NewMockRecorderClient(ctl)
		ctx := SetMockRecorder(context.Background(), mockClient)
		ctx, _ = testclock.UseTime(ctx, testclock.TestRecentTimeUTC)
		ctx = memory.UseInfo(ctx, "cr-buildbucket-dev")

		bqExports := []*rdbPb.BigQueryExport{}

		t.Run("builds without number", func(t *ftt.Test) {
			builds := []*model.Build{
				{
					ID: 1,
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "proj1",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Infra: &pb.BuildInfra{
							Resultdb: &pb.BuildInfra_ResultDB{
								Hostname:  "host",
								Enable:    true,
								BqExports: bqExports,
							},
						},
					},
				},
				{
					ID: 2,
					Proto: &pb.Build{
						Id: 2,
						Builder: &pb.BuilderID{
							Project: "proj1",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Infra: &pb.BuildInfra{
							Resultdb: &pb.BuildInfra_ResultDB{
								Enable:    true,
								BqExports: bqExports,
							},
						},
					},
				},
			}
			opts := []CreateOptions{
				{
					IsExportRoot: true,
				},
				{
					IsExportRoot: false,
				},
			}

			mockClient.EXPECT().CreateInvocation(gomock.Any(), proto.MatcherEqual(
				&rdbPb.CreateInvocationRequest{
					InvocationId: "build-1",
					Invocation: &rdbPb.Invocation{
						Deadline:         timestamppb.New(testclock.TestRecentTimeUTC),
						BigqueryExports:  bqExports,
						ProducerResource: "//cr-buildbucket-dev.appspot.com/builds/1",
						Realm:            "proj1:bucket",
						IsExportRoot:     true,
					},
					RequestId: "build-1",
				}), gomock.Any()).DoAndReturn(func(ctx context.Context, in *rdbPb.CreateInvocationRequest, opt grpc.CallOption) (*rdbPb.Invocation, error) {
				h, _ := opt.(grpc.HeaderCallOption)
				h.HeaderAddr.Set("update-token", "token for build-1")
				return &rdbPb.Invocation{}, nil
			})
			mockClient.EXPECT().CreateInvocation(gomock.Any(), proto.MatcherEqual(
				&rdbPb.CreateInvocationRequest{
					InvocationId: "build-2",
					Invocation: &rdbPb.Invocation{
						Deadline:         timestamppb.New(testclock.TestRecentTimeUTC),
						BigqueryExports:  bqExports,
						ProducerResource: "//cr-buildbucket-dev.appspot.com/builds/2",
						Realm:            "proj1:bucket",
					},
					RequestId: "build-2",
				}), gomock.Any()).DoAndReturn(func(ctx context.Context, in *rdbPb.CreateInvocationRequest, opt grpc.CallOption) (*rdbPb.Invocation, error) {
				h, _ := opt.(grpc.HeaderCallOption)
				h.HeaderAddr.Set("update-token", "token for build-2")
				return &rdbPb.Invocation{}, nil
			})

			err := CreateInvocations(ctx, builds, opts)
			assert.Loosely(t, err, should.BeEmpty)
			assert.Loosely(t, builds[0].ResultDBUpdateToken, should.Equal("token for build-1"))
			assert.Loosely(t, builds[0].Proto.Infra.Resultdb.Invocation, should.Equal("invocations/build-1"))
			assert.Loosely(t, builds[1].ResultDBUpdateToken, should.Equal("token for build-2"))
			assert.Loosely(t, builds[1].Proto.Infra.Resultdb.Invocation, should.Equal("invocations/build-2"))
		})

		t.Run("build with number and expirations", func(t *ftt.Test) {
			builds := []*model.Build{
				{
					ID: 1,
					Proto: &pb.Build{
						Id:     1,
						Number: 123,
						Builder: &pb.BuilderID{
							Project: "proj1",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Infra: &pb.BuildInfra{
							Resultdb: &pb.BuildInfra_ResultDB{
								Hostname:  "host",
								Enable:    true,
								BqExports: bqExports,
							},
						},
						ExecutionTimeout:  durationpb.New(1000),
						SchedulingTimeout: durationpb.New(1000),
					},
				},
			}
			opts := []CreateOptions{{
				IsExportRoot: true,
			}}

			deadline := testclock.TestRecentTimeUTC.Add(2000)
			sha256Bldr := sha256.Sum256([]byte("proj1/bucket/builder"))
			mockClient.EXPECT().CreateInvocation(gomock.Any(), proto.MatcherEqual(
				&rdbPb.CreateInvocationRequest{
					InvocationId: "build-1",
					Invocation: &rdbPb.Invocation{
						Deadline:         timestamppb.New(deadline),
						BigqueryExports:  bqExports,
						ProducerResource: "//cr-buildbucket-dev.appspot.com/builds/1",
						Realm:            "proj1:bucket",
						IsExportRoot:     true,
					},
					RequestId: "build-1",
				}), gomock.Any()).DoAndReturn(func(ctx context.Context, in *rdbPb.CreateInvocationRequest, opt grpc.CallOption) (*rdbPb.Invocation, error) {
				h, _ := opt.(grpc.HeaderCallOption)
				h.HeaderAddr.Set("update-token", "token for build id 1")
				return &rdbPb.Invocation{}, nil
			})
			mockClient.EXPECT().CreateInvocation(gomock.Any(), proto.MatcherEqual(
				&rdbPb.CreateInvocationRequest{
					InvocationId: fmt.Sprintf("build-%s-123", hex.EncodeToString(sha256Bldr[:])),
					Invocation: &rdbPb.Invocation{
						IncludedInvocations: []string{"invocations/build-1"},
						ProducerResource:    "//cr-buildbucket-dev.appspot.com/builds/1",
						State:               rdbPb.Invocation_FINALIZING,
						Realm:               "proj1:bucket",
						// Should NOT be marked export root.
					},
					RequestId: "build-1-123",
				})).Return(&rdbPb.Invocation{}, nil)

			err := CreateInvocations(ctx, builds, opts)
			assert.Loosely(t, err, should.BeEmpty)
			assert.Loosely(t, len(builds), should.Equal(1))
			assert.Loosely(t, builds[0].ResultDBUpdateToken, should.Equal("token for build id 1"))
			assert.Loosely(t, builds[0].Proto.Infra.Resultdb.Invocation, should.Equal("invocations/build-1"))
		})

		t.Run("already exists error", func(t *ftt.Test) {
			builds := []*model.Build{
				{
					ID: 1,
					Proto: &pb.Build{
						Id:     1,
						Number: 123,
						Builder: &pb.BuilderID{
							Project: "proj1",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Infra: &pb.BuildInfra{
							Resultdb: &pb.BuildInfra_ResultDB{
								Hostname:  "host",
								Enable:    true,
								BqExports: bqExports,
							},
						},
					},
				},
			}
			opts := []CreateOptions{{
				IsExportRoot: true,
			}}

			mockClient.EXPECT().CreateInvocation(gomock.Any(), proto.MatcherEqual(
				&rdbPb.CreateInvocationRequest{
					InvocationId: "build-1",
					Invocation: &rdbPb.Invocation{
						Deadline:         timestamppb.New(testclock.TestRecentTimeUTC),
						BigqueryExports:  bqExports,
						ProducerResource: "//cr-buildbucket-dev.appspot.com/builds/1",
						Realm:            "proj1:bucket",
						IsExportRoot:     true,
					},
					RequestId: "build-1",
				}), gomock.Any()).Return(nil, grpcStatus.Error(codes.AlreadyExists, "already exists"))

			err := CreateInvocations(ctx, builds, opts)
			assert.Loosely(t, err, should.ErrLike("failed to create the invocation for build id: 1: rpc error: code = AlreadyExists desc = already exists"))
		})

		t.Run("resultDB throws err", func(t *ftt.Test) {
			builds := []*model.Build{
				{
					ID: 1,
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "proj1",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Infra: &pb.BuildInfra{
							Resultdb: &pb.BuildInfra_ResultDB{
								Hostname:  "host",
								Enable:    true,
								BqExports: bqExports,
							},
						},
					},
				},
			}
			opts := []CreateOptions{{
				IsExportRoot: true,
			}}

			mockClient.EXPECT().CreateInvocation(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil, grpcStatus.Error(codes.DeadlineExceeded, "timeout"))

			err := CreateInvocations(ctx, builds, opts)
			assert.Loosely(t, err, should.ErrLike("failed to create the invocation for build id: 1: rpc error: code = DeadlineExceeded desc = timeout"))
		})

		t.Run("partial success", func(t *ftt.Test) {
			builds := []*model.Build{
				{
					ID: 1,
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "proj1",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Infra: &pb.BuildInfra{
							Resultdb: &pb.BuildInfra_ResultDB{
								Hostname:  "host",
								Enable:    true,
								BqExports: bqExports,
							},
						},
					},
				},
				{
					ID: 2,
					Proto: &pb.Build{
						Id: 2,
						Builder: &pb.BuilderID{
							Project: "proj1",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Infra: &pb.BuildInfra{
							Resultdb: &pb.BuildInfra_ResultDB{
								Hostname:  "host",
								Enable:    true,
								BqExports: bqExports,
							},
						},
					},
				},
			}
			opts := []CreateOptions{{
				IsExportRoot: true,
			}, {
				IsExportRoot: false,
			}}

			mockClient.EXPECT().CreateInvocation(gomock.Any(), proto.MatcherEqual(
				&rdbPb.CreateInvocationRequest{
					InvocationId: "build-1",
					Invocation: &rdbPb.Invocation{
						Deadline:         timestamppb.New(testclock.TestRecentTimeUTC),
						BigqueryExports:  bqExports,
						ProducerResource: "//cr-buildbucket-dev.appspot.com/builds/1",
						Realm:            "proj1:bucket",
						IsExportRoot:     true,
					},
					RequestId: "build-1",
				}), gomock.Any()).Return(nil, grpcStatus.Error(codes.Internal, "error"))
			mockClient.EXPECT().CreateInvocation(gomock.Any(), proto.MatcherEqual(
				&rdbPb.CreateInvocationRequest{
					InvocationId: "build-2",
					Invocation: &rdbPb.Invocation{
						Deadline:         timestamppb.New(testclock.TestRecentTimeUTC),
						BigqueryExports:  bqExports,
						ProducerResource: "//cr-buildbucket-dev.appspot.com/builds/2",
						Realm:            "proj1:bucket",
					},
					RequestId: "build-2",
				}), gomock.Any()).DoAndReturn(func(ctx context.Context, in *rdbPb.CreateInvocationRequest, opt grpc.CallOption) (*rdbPb.Invocation, error) {
				h, _ := opt.(grpc.HeaderCallOption)
				h.HeaderAddr.Set("update-token", "update token")
				return &rdbPb.Invocation{}, nil
			})

			err := CreateInvocations(ctx, builds, opts)
			assert.Loosely(t, err[0], should.ErrLike("failed to create the invocation for build id: 1: rpc error: code = Internal desc = error"))
			assert.Loosely(t, err[1], should.BeNil)
			assert.Loosely(t, builds[0].ResultDBUpdateToken, should.BeEmpty)
			assert.Loosely(t, builds[0].Proto.Infra.Resultdb.Invocation, should.BeEmpty)
			assert.Loosely(t, builds[1].ResultDBUpdateToken, should.Equal("update token"))
			assert.Loosely(t, builds[1].Proto.Infra.Resultdb.Invocation, should.Equal("invocations/build-2"))
		})

		t.Run("resultDB not enabled", func(t *ftt.Test) {
			builds := []*model.Build{
				{
					ID: 1,
					Proto: &pb.Build{
						Id: 1,
						Builder: &pb.BuilderID{
							Project: "proj1",
							Bucket:  "bucket",
							Builder: "builder",
						},
						Infra: &pb.BuildInfra{Resultdb: &pb.BuildInfra_ResultDB{
							Hostname: "host",
							Enable:   false,
						}},
					},
				},
			}
			opts := []CreateOptions{{}}

			err := CreateInvocations(ctx, builds, opts)
			assert.Loosely(t, err, should.BeEmpty)
			assert.Loosely(t, builds[0].Proto.Infra.Resultdb.Invocation, should.BeEmpty)
		})
	})
}

func TestFinalizeInvocation(t *testing.T) {
	t.Parallel()

	ftt.Run("finalize invocations", t, func(t *ftt.Test) {
		ctl := gomock.NewController(t)
		defer ctl.Finish()
		mockClient := rdbPb.NewMockRecorderClient(ctl)
		ctx := memory.Use(context.Background())
		ctx = SetMockRecorder(ctx, mockClient)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		assert.Loosely(t, datastore.Put(ctx, &model.Build{
			ID:                  1,
			Project:             "project",
			BucketID:            "bucket",
			BuilderID:           "builder",
			ResultDBUpdateToken: "token",
			Proto: &pb.Build{
				Id: 1,
				Builder: &pb.BuilderID{
					Project: "project",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status: pb.Status_SUCCESS,
			},
		}), should.BeNil)

		t.Run("no exists", func(t *ftt.Test) {
			assert.Loosely(t, FinalizeInvocation(ctx, 1), should.ErrLike("build 1 or buildInfra not found"))
		})

		t.Run("no resultdb hostname", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.BuildInfra{
				ID:    1,
				Build: datastore.KeyForObj(ctx, &model.Build{ID: 1}),
				Proto: &pb.BuildInfra{
					Resultdb: &pb.BuildInfra_ResultDB{
						Invocation: "invocation",
					},
				},
			}), should.BeNil)

			mockClient.EXPECT().FinalizeInvocation(gomock.Any(), gomock.Any()).Times(0)
			assert.Loosely(t, FinalizeInvocation(ctx, 1), should.BeNil)
		})

		t.Run("no invocation", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.BuildInfra{
				ID:    1,
				Build: datastore.KeyForObj(ctx, &model.Build{ID: 1}),
				Proto: &pb.BuildInfra{
					Resultdb: &pb.BuildInfra_ResultDB{
						Hostname: "hostname",
					},
				},
			}), should.BeNil)

			mockClient.EXPECT().FinalizeInvocation(gomock.Any(), gomock.Any()).Times(0)
			assert.Loosely(t, FinalizeInvocation(ctx, 1), should.BeNil)
		})

		t.Run("success", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.BuildInfra{
				ID:    1,
				Build: datastore.KeyForObj(ctx, &model.Build{ID: 1}),
				Proto: &pb.BuildInfra{
					Resultdb: &pb.BuildInfra_ResultDB{
						Hostname:   "hostname",
						Invocation: "invocation",
					},
				},
			}), should.BeNil)

			expectedCtx := metadata.AppendToOutgoingContext(ctx, "update-token", "token")
			mockClient.EXPECT().FinalizeInvocation(expectedCtx, proto.MatcherEqual(&rdbPb.FinalizeInvocationRequest{
				Name: "invocation",
			})).Return(&rdbPb.Invocation{}, nil)

			assert.Loosely(t, FinalizeInvocation(ctx, 1), should.BeNil)
		})

		t.Run("resultDB server fatal err", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.BuildInfra{
				ID:    1,
				Build: datastore.KeyForObj(ctx, &model.Build{ID: 1}),
				Proto: &pb.BuildInfra{
					Resultdb: &pb.BuildInfra_ResultDB{
						Hostname:   "hostname",
						Invocation: "invocation",
					},
				},
			}), should.BeNil)

			mockClient.EXPECT().FinalizeInvocation(gomock.Any(), proto.MatcherEqual(&rdbPb.FinalizeInvocationRequest{
				Name: "invocation",
			})).Return(nil, grpcStatus.Error(codes.PermissionDenied, "permission denied"))

			err := FinalizeInvocation(ctx, 1)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, tq.Fatal.In(err), should.BeTrue)
		})

		t.Run("resultDB server retryable err", func(t *ftt.Test) {
			assert.Loosely(t, datastore.Put(ctx, &model.BuildInfra{
				ID:    1,
				Build: datastore.KeyForObj(ctx, &model.Build{ID: 1}),
				Proto: &pb.BuildInfra{
					Resultdb: &pb.BuildInfra_ResultDB{
						Hostname:   "hostname",
						Invocation: "invocation",
					},
				},
			}), should.BeNil)

			mockClient.EXPECT().FinalizeInvocation(gomock.Any(), proto.MatcherEqual(&rdbPb.FinalizeInvocationRequest{
				Name: "invocation",
			})).Return(nil, grpcStatus.Error(codes.Internal, "internal error"))

			err := FinalizeInvocation(ctx, 1)
			assert.Loosely(t, err, should.NotBeNil)
			assert.Loosely(t, transient.Tag.In(err), should.BeTrue)
		})
	})
}
