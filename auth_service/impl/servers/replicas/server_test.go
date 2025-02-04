// Copyright 2024 The LUCI Authors.
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

package replicas

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/gae/service/info"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/rpcpb"
	"go.chromium.org/luci/auth_service/impl/model"
)

func TestReplicasServer(t *testing.T) {
	t.Parallel()

	ftt.Run("ListReplicas RPC call", t, func(t *ftt.Test) {
		createdTime := time.Date(2021, time.August, 16, 15, 20, 0, 0, time.UTC)
		modifiedTime := time.Date(2022, time.July, 4, 15, 45, 0, 0, time.UTC)
		pushStartedTime := time.Date(2022, time.July, 4, 16, 32, 1, 0, time.UTC)
		pushFinishedTime := time.Date(2022, time.July, 4, 16, 32, 9, 0, time.UTC)
		testNow := time.Date(2022, time.July, 4, 16, 45, 0, 0, time.UTC)

		ctx := clock.Set(context.Background(), testclock.New(testNow))
		ctx = memory.Use(ctx)

		// Set up replica states and AuthDB replication state into Datastore.
		assert.Loosely(t, datastore.Put(ctx,
			&model.AuthReplicaState{
				Kind:            "AuthReplicaState",
				ID:              "dev~appB",
				Parent:          model.ReplicasRootKey(ctx),
				ReplicaURL:      "https://test.dev~appB.appspot.com",
				AuthDBRev:       5,
				RevModifiedTS:   modifiedTime,
				AuthCodeVersion: "1.2.3",
				PushStartedTS:   pushStartedTime,
				PushFinishedTS:  pushFinishedTime,
				PushStatus:      model.ReplicaPushStatusSuccess,
			},
			&model.AuthReplicaState{
				Kind:            "AuthReplicaState",
				ID:              "dev~appA",
				Parent:          model.ReplicasRootKey(ctx),
				ReplicaURL:      "https://test.dev~appA.appspot.com",
				AuthDBRev:       4,
				RevModifiedTS:   createdTime,
				AuthCodeVersion: "1.2.4",
				PushStartedTS:   pushStartedTime,
				PushFinishedTS:  pushFinishedTime,
				PushStatus:      model.ReplicaPushStatusFatalError,
				PushError:       "some sort of fatal error",
			},
			&model.AuthReplicationState{
				Kind:       "AuthReplicationState",
				Parent:     model.RootKey(ctx),
				ModifiedTS: modifiedTime,
				AuthDBRev:  5,
			}), should.BeNil)

		expectedResp := &rpcpb.ListReplicasResponse{
			PrimaryRevision: &protocol.AuthDBRevision{
				PrimaryId:  info.AppID(ctx),
				AuthDbRev:  5,
				ModifiedTs: modifiedTime.UnixMicro(),
			},
			AuthCodeVersion: model.AuthAPIVersion,
			Replicas: []*rpcpb.ReplicaState{
				{
					AppId:           "dev~appA",
					BaseUrl:         "https://test.dev~appA.appspot.com",
					AuthDbRev:       4,
					RevModified:     timestamppb.New(createdTime),
					AuthCodeVersion: "1.2.4",
					PushStarted:     timestamppb.New(pushStartedTime),
					PushFinished:    timestamppb.New(pushFinishedTime),
					PushStatus:      rpcpb.ReplicaPushStatus_FATAL_ERROR,
					PushError:       "some sort of fatal error",
				},
				{
					AppId:           "dev~appB",
					BaseUrl:         "https://test.dev~appB.appspot.com",
					AuthDbRev:       5,
					RevModified:     timestamppb.New(modifiedTime),
					AuthCodeVersion: "1.2.3",
					PushStarted:     timestamppb.New(pushStartedTime),
					PushFinished:    timestamppb.New(pushFinishedTime),
					PushStatus:      rpcpb.ReplicaPushStatus_SUCCESS,
				},
			},
			ProcessedAt: timestamppb.New(testNow),
		}

		srv := Server{}
		resp, err := srv.ListReplicas(ctx, &emptypb.Empty{})
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, resp, should.Match(expectedResp))
	})
}
