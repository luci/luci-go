// Copyright 2023 The LUCI Authors.
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

package rdb

import (
	"context"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"google.golang.org/grpc/codes"
	"google.golang.org/protobuf/types/known/emptypb"

	bpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/proto"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/grpc/appstatus"
	rdbpb "go.chromium.org/luci/resultdb/proto/v1"
	"go.chromium.org/luci/server/tq/tqtesting"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestNotifier(t *testing.T) {

	ftt.Run(`MarkInvocationSubmitted`, t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		mcf := NewMockRecorderClientFactory(ct.GoMockCtl)
		notifier := NewNotifier(ct.TQDispatcher, mcf)

		epoch := ct.Clock.Now().UTC()
		runID := common.MakeRunID("infra", epoch, 1, []byte("aaa"))

		r := &run.Run{
			ID:         common.RunID(runID),
			Status:     run.Status_SUCCEEDED,
			CreateTime: epoch,
			StartTime:  epoch.Add(time.Minute * 2),
			EndTime:    epoch.Add(time.Minute * 25),
			Mode:       run.FullRun,
			Tryjobs: &run.Tryjobs{
				State: &tryjob.ExecutionState{
					Executions: []*tryjob.ExecutionState_Execution{
						{
							Attempts: []*tryjob.ExecutionState_Execution_Attempt{
								{
									ExternalId: string(tryjob.MustBuildbucketID("cr-buildbucket.appspot.com", 100004)),
									Status:     tryjob.Status_ENDED,
									Reused:     true,
									Result: &tryjob.Result{
										Backend: &tryjob.Result_Buildbucket_{
											Buildbucket: &tryjob.Result_Buildbucket{
												Infra: &bpb.BuildInfra{
													Resultdb: &bpb.BuildInfra_ResultDB{
														Hostname:   "resultdb.example.com",
														Invocation: "invocations/build:12345",
													},
												},
											},
										},
									},
								},
								{
									ExternalId: string(tryjob.MustBuildbucketID("cr-buildbucket.appspot.com", 100005)),
									Status:     tryjob.Status_ENDED,
									Result: &tryjob.Result{
										Backend: &tryjob.Result_Buildbucket_{
											Buildbucket: &tryjob.Result_Buildbucket{
												Infra: &bpb.BuildInfra{
													Resultdb: &bpb.BuildInfra_ResultDB{
														Hostname:   "resultdb.example.com",
														Invocation: "invocations/build:67890",
													},
												},
											},
										},
									},
								},
							},
						},
					},
					Status: tryjob.ExecutionState_SUCCEEDED,
				},
			},
		}
		assert.Loosely(t, datastore.Put(ctx, r), should.BeNil)

		t.Run(`Permission Denied`, func(t *ftt.Test) {
			mcf.mc.EXPECT().MarkInvocationSubmitted(gomock.Any(), proto.MatcherEqual(&rdbpb.MarkInvocationSubmittedRequest{
				Invocation: "invocations/build:12345",
			})).Return(&emptypb.Empty{}, appstatus.Error(codes.PermissionDenied, "permission denied"))
			mcf.mc.EXPECT().MarkInvocationSubmitted(gomock.Any(), proto.MatcherEqual(&rdbpb.MarkInvocationSubmittedRequest{
				Invocation: "invocations/build:67890",
			})).Return(&emptypb.Empty{}, appstatus.Error(codes.PermissionDenied, "permission denied"))

			err := MarkInvocationSubmitted(ctx, mcf, runID)
			assert.Loosely(t, err, should.ErrLike("failed to mark invocation submitted"))
		})

		t.Run(`Valid`, func(t *ftt.Test) {
			mcf.mc.EXPECT().MarkInvocationSubmitted(gomock.Any(), proto.MatcherEqual(&rdbpb.MarkInvocationSubmittedRequest{
				Invocation: "invocations/build:12345",
			})).Return(&emptypb.Empty{}, nil)
			mcf.mc.EXPECT().MarkInvocationSubmitted(gomock.Any(), proto.MatcherEqual(&rdbpb.MarkInvocationSubmittedRequest{
				Invocation: "invocations/build:67890",
			})).Return(&emptypb.Empty{}, nil)

			err := datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				return notifier.Schedule(ctx, runID)
			}, nil)
			assert.NoErr(t, err)

			assert.Loosely(t, ct.TQ.Tasks(), should.HaveLength(1))
			ct.TQ.Run(ctx, tqtesting.StopAfterTask(notifierTaskClass))
		})
	})
}
