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

package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/rpcpb"
)

func testReplicaState(ctx context.Context, appID string, authDBRev int64) *AuthReplicaState {
	return &AuthReplicaState{
		Kind:      "AuthReplicaState",
		ID:        appID,
		Parent:    ReplicasRootKey(ctx),
		AuthDBRev: authDBRev,
	}
}

func TestReplicaUpdateError(t *testing.T) {
	t.Parallel()

	ftt.Run("custom ReplicaUpdateError works", t, func(t *ftt.Test) {
		t.Run("unwrapping works", func(t *ftt.Test) {
			replicaErr := &ReplicaUpdateError{
				RootErr: errors.Annotate(datastore.ErrNoSuchEntity, "annotated test error").Err(),
				IsFatal: false,
			}
			// Check the root error can be identified when it is a non-fatal error.
			assert.Loosely(t, errors.Is(replicaErr, FatalReplicaUpdateError), should.BeFalse)
			assert.Loosely(t, errors.Is(replicaErr, datastore.ErrNoSuchEntity), should.BeTrue)

			// Check the root error can be identified when it is a fatal error.
			replicaErr.IsFatal = true
			assert.Loosely(t, errors.Is(replicaErr, FatalReplicaUpdateError), should.BeTrue)
			assert.Loosely(t, errors.Is(replicaErr, datastore.ErrNoSuchEntity), should.BeTrue)
		})

		t.Run("returns error message", func(t *ftt.Test) {
			replicaErr := &ReplicaUpdateError{
				RootErr: fmt.Errorf("custom test error"),
				IsFatal: true,
			}
			assert.Loosely(t, replicaErr.Error(), should.Equal("custom test error"))
		})
	})
}

func TestAuthReplicaStateToProto(t *testing.T) {
	t.Parallel()

	ftt.Run("AuthReplicaState ToProto works", t, func(t *ftt.Test) {
		testPushStart := time.Date(2022, time.July, 4, 16, 32, 1, 0, time.UTC)
		testPushEnd := time.Date(2022, time.July, 4, 16, 32, 9, 0, time.UTC)
		replica := &AuthReplicaState{
			Kind:            "AuthReplicaState",
			ID:              "dev~appA",
			ReplicaURL:      "https://test.dev~appA.appspot.com",
			AuthDBRev:       4,
			RevModifiedTS:   testModifiedTS,
			AuthCodeVersion: "1.2.3",
			PushStartedTS:   testPushStart,
			PushFinishedTS:  testPushEnd,
			PushStatus:      ReplicaPushStatusFatalError,
			PushError:       "some sort of fatal error",
		}
		assert.Loosely(t, replica.ToProto(), should.Resemble(&rpcpb.ReplicaState{
			AppId:           "dev~appA",
			BaseUrl:         "https://test.dev~appA.appspot.com",
			AuthDbRev:       4,
			RevModified:     timestamppb.New(testModifiedTS),
			AuthCodeVersion: "1.2.3",
			PushStarted:     timestamppb.New(testPushStart),
			PushFinished:    timestamppb.New(testPushEnd),
			PushStatus:      rpcpb.ReplicaPushStatus_FATAL_ERROR,
			PushError:       "some sort of fatal error",
		}))
	})
}

func TestGetAllReplicas(t *testing.T) {
	t.Parallel()

	ftt.Run("GetAllReplicas works", t, func(t *ftt.Test) {
		t.Run("succeeds even when empty", func(t *ftt.Test) {
			ctx := memory.Use(context.Background())

			replicas, err := GetAllReplicas(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, replicas, should.BeEmpty)
		})

		t.Run("returns all replicas", func(t *ftt.Test) {
			// Set up replica states in the datastore.
			ctx := memory.Use(context.Background())
			assert.Loosely(t, datastore.Put(ctx,
				testReplicaState(ctx, "dev~appG", 1),
				testReplicaState(ctx, "dev~appF", 2),
				testReplicaState(ctx, "dev~appE", 3),
				testReplicaState(ctx, "dev~appA", 4),
				testReplicaState(ctx, "dev~appB", 5),
				testReplicaState(ctx, "dev~appC", 3),
				testReplicaState(ctx, "dev~appD", 2),
			), should.BeNil)

			// Check all the replica states were returned, sorted by App ID.
			replicas, err := GetAllReplicas(ctx)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, replicas, should.Resemble([]*AuthReplicaState{
				testReplicaState(ctx, "dev~appA", 4),
				testReplicaState(ctx, "dev~appB", 5),
				testReplicaState(ctx, "dev~appC", 3),
				testReplicaState(ctx, "dev~appD", 2),
				testReplicaState(ctx, "dev~appE", 3),
				testReplicaState(ctx, "dev~appF", 2),
				testReplicaState(ctx, "dev~appG", 1),
			}))
		})
	})
}

func TestGetAllStaleReplicas(t *testing.T) {
	t.Parallel()

	ftt.Run("GetAllStaleReplicas works", t, func(t *ftt.Test) {
		t.Run("succeeds even when empty", func(t *ftt.Test) {
			ctx := memory.Use(context.Background())

			replicas, err := GetAllStaleReplicas(ctx, 1000)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, replicas, should.BeEmpty)
		})

		t.Run("returns only stale replicas", func(t *ftt.Test) {
			// Set up replica states in the datastore.
			ctx := memory.Use(context.Background())
			assert.Loosely(t, datastore.Put(ctx,
				testReplicaState(ctx, "dev~appG", 1),
				testReplicaState(ctx, "dev~appF", 2),
				testReplicaState(ctx, "dev~appE", 3),
				testReplicaState(ctx, "dev~appA", 4),
				testReplicaState(ctx, "dev~appB", 5),
				testReplicaState(ctx, "dev~appC", 3),
				testReplicaState(ctx, "dev~appD", 2),
			), should.BeNil)

			// Check the replicas returned were limited to stale ones, and are
			// in ascending order of the app ID.
			replicas, err := GetAllStaleReplicas(ctx, 4)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, replicas, should.Resemble([]*AuthReplicaState{
				testReplicaState(ctx, "dev~appC", 3),
				testReplicaState(ctx, "dev~appD", 2),
				testReplicaState(ctx, "dev~appE", 3),
				testReplicaState(ctx, "dev~appF", 2),
				testReplicaState(ctx, "dev~appG", 1),
			}))
		})
	})
}

func TestGetAuthReplicaState(t *testing.T) {
	t.Parallel()

	ftt.Run("getAuthReplicaState works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		r := testReplicaState(ctx, "dev~appA", 123)

		_, err := getAuthReplicaState(ctx, replicaStateKey(ctx, "dev~appA"))
		assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))

		// Now set up the replica state.
		assert.Loosely(t, datastore.Put(ctx, r), should.BeNil)

		actual, err := getAuthReplicaState(ctx, replicaStateKey(ctx, "dev~appA"))
		assert.Loosely(t, err, should.BeNil)
		assert.Loosely(t, actual, should.Resemble(r))
	})
}

func TestUpdateReplicaStateOnSuccess(t *testing.T) {
	t.Parallel()

	ftt.Run("updateReplicaStateOnSuccess works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		started := testCreatedTS
		finished := testModifiedTS
		currentRevision := &protocol.AuthDBRevision{
			PrimaryId:  "chrome-infra-auth-test",
			AuthDbRev:  1001,
			ModifiedTs: testModifiedTS.UnixMicro(),
		}
		authCodeVersion := "2.0.test"

		t.Run("returns error for unknown replica", func(t *ftt.Test) {
			storedRev, err := updateReplicaStateOnSuccess(ctx, "dev~unknownApp",
				started, finished, currentRevision, authCodeVersion)
			assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))
			assert.Loosely(t, storedRev, should.BeZero)
		})

		t.Run("updates last known state", func(t *ftt.Test) {
			testAppID := "dev~appA"
			initialState := testReplicaState(ctx, testAppID, 1000)
			assert.Loosely(t, datastore.Put(ctx, initialState), should.BeNil)

			storedRev, err := updateReplicaStateOnSuccess(ctx, testAppID,
				started, finished, currentRevision, authCodeVersion)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, storedRev, should.Equal(1001))

			// Check the replica state was updated.
			actual := &AuthReplicaState{
				Kind:   "AuthReplicaState",
				ID:     testAppID,
				Parent: ReplicasRootKey(ctx),
			}
			assert.Loosely(t, datastore.Get(ctx, actual), should.BeNil)
			assert.Loosely(t, actual, should.Resemble(&AuthReplicaState{
				Kind:            "AuthReplicaState",
				ID:              testAppID,
				Parent:          ReplicasRootKey(ctx),
				ReplicaURL:      initialState.ReplicaURL,
				AuthDBRev:       1001,
				RevModifiedTS:   testModifiedTS,
				AuthCodeVersion: authCodeVersion,
				PushStartedTS:   started,
				PushFinishedTS:  finished,
				PushStatus:      ReplicaPushStatusSuccess,
				PushError:       "",
			}))
		})
	})
}

func TestUpdateReplicaStateOnFail(t *testing.T) {
	t.Parallel()

	ftt.Run("updateReplicaStateOnSuccess works", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())

		started := testCreatedTS
		finished := testModifiedTS
		oldReplicaRev := int64(1000)
		errMsg := "replica returned transient error"
		pushErr := &ReplicaUpdateError{
			RootErr: fmt.Errorf(errMsg),
			IsFatal: false,
		}

		t.Run("returns error for unknown replica", func(t *ftt.Test) {
			storedRev, err := updateReplicaStateOnFail(ctx, "dev~unknownApp",
				oldReplicaRev, started, finished, pushErr)
			assert.Loosely(t, err, should.Equal(datastore.ErrNoSuchEntity))
			assert.Loosely(t, storedRev, should.BeZero)
		})

		t.Run("exits early if the replica has a newer revision", func(t *ftt.Test) {
			testAppID := "dev~appA"
			advancedRev := oldReplicaRev + 1
			initialState := testReplicaState(ctx, testAppID, advancedRev)
			assert.Loosely(t, datastore.Put(ctx, initialState), should.BeNil)

			storedRev, err := updateReplicaStateOnFail(ctx, testAppID,
				oldReplicaRev, started, finished, pushErr)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, storedRev, should.Equal(advancedRev))

			// Check the replica state was unchanged.
			actual := &AuthReplicaState{
				Kind:   "AuthReplicaState",
				ID:     testAppID,
				Parent: ReplicasRootKey(ctx),
			}
			assert.Loosely(t, datastore.Get(ctx, actual), should.BeNil)
			assert.Loosely(t, actual, should.Resemble(initialState))
		})

		t.Run("updates last known state with error details", func(t *ftt.Test) {
			testAppID := "dev~appA"
			initialState := testReplicaState(ctx, testAppID, oldReplicaRev)
			assert.Loosely(t, datastore.Put(ctx, initialState), should.BeNil)

			storedRev, err := updateReplicaStateOnFail(ctx, testAppID,
				oldReplicaRev, started, finished, pushErr)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, storedRev, should.Equal(oldReplicaRev))

			// Check the replica state was updated.
			actual := &AuthReplicaState{
				Kind:   "AuthReplicaState",
				ID:     testAppID,
				Parent: ReplicasRootKey(ctx),
			}
			assert.Loosely(t, datastore.Get(ctx, actual), should.BeNil)
			assert.Loosely(t, actual, should.Resemble(&AuthReplicaState{
				Kind:           "AuthReplicaState",
				ID:             testAppID,
				Parent:         ReplicasRootKey(ctx),
				ReplicaURL:     initialState.ReplicaURL,
				AuthDBRev:      oldReplicaRev,
				PushStartedTS:  started,
				PushFinishedTS: finished,
				PushStatus:     ReplicaPushStatusTransientError,
				PushError:      errMsg,
			}))
		})
	})
}
