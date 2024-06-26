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
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/service/protocol"

	"go.chromium.org/luci/auth_service/api/rpcpb"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
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

	Convey("custom ReplicaUpdateError works", t, func() {
		Convey("unwrapping works", func() {
			replicaErr := &ReplicaUpdateError{
				RootErr: errors.Annotate(datastore.ErrNoSuchEntity, "annotated test error").Err(),
				IsFatal: false,
			}
			// Check the root error can be identified when it is a non-fatal error.
			So(errors.Is(replicaErr, FatalReplicaUpdateError), ShouldBeFalse)
			So(errors.Is(replicaErr, datastore.ErrNoSuchEntity), ShouldBeTrue)

			// Check the root error can be identified when it is a fatal error.
			replicaErr.IsFatal = true
			So(errors.Is(replicaErr, FatalReplicaUpdateError), ShouldBeTrue)
			So(errors.Is(replicaErr, datastore.ErrNoSuchEntity), ShouldBeTrue)
		})

		Convey("returns error message", func() {
			replicaErr := &ReplicaUpdateError{
				RootErr: fmt.Errorf("custom test error"),
				IsFatal: true,
			}
			So(replicaErr.Error(), ShouldEqual, "custom test error")
		})
	})
}

func TestAuthReplicaStateToProto(t *testing.T) {
	t.Parallel()

	Convey("AuthReplicaState ToProto works", t, func() {
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
		So(replica.ToProto(), ShouldResembleProto, &rpcpb.ReplicaState{
			AppId:           "dev~appA",
			BaseUrl:         "https://test.dev~appA.appspot.com",
			AuthDbRev:       4,
			RevModified:     timestamppb.New(testModifiedTS),
			AuthCodeVersion: "1.2.3",
			PushStarted:     timestamppb.New(testPushStart),
			PushFinished:    timestamppb.New(testPushEnd),
			PushStatus:      rpcpb.ReplicaPushStatus_FATAL_ERROR,
			PushError:       "some sort of fatal error",
		})
	})
}

func TestGetAllReplicas(t *testing.T) {
	t.Parallel()

	Convey("GetAllReplicas works", t, func() {
		Convey("succeeds even when empty", func() {
			ctx := memory.Use(context.Background())

			replicas, err := GetAllReplicas(ctx)
			So(err, ShouldBeNil)
			So(replicas, ShouldBeEmpty)
		})

		Convey("returns all replicas", func() {
			// Set up replica states in the datastore.
			ctx := memory.Use(context.Background())
			So(datastore.Put(ctx,
				testReplicaState(ctx, "dev~appG", 1),
				testReplicaState(ctx, "dev~appF", 2),
				testReplicaState(ctx, "dev~appE", 3),
				testReplicaState(ctx, "dev~appA", 4),
				testReplicaState(ctx, "dev~appB", 5),
				testReplicaState(ctx, "dev~appC", 3),
				testReplicaState(ctx, "dev~appD", 2),
			), ShouldBeNil)

			// Check all the replica states were returned, sorted by App ID.
			replicas, err := GetAllReplicas(ctx)
			So(err, ShouldBeNil)
			So(replicas, ShouldResembleProto, []*AuthReplicaState{
				testReplicaState(ctx, "dev~appA", 4),
				testReplicaState(ctx, "dev~appB", 5),
				testReplicaState(ctx, "dev~appC", 3),
				testReplicaState(ctx, "dev~appD", 2),
				testReplicaState(ctx, "dev~appE", 3),
				testReplicaState(ctx, "dev~appF", 2),
				testReplicaState(ctx, "dev~appG", 1),
			})
		})
	})
}

func TestGetAllStaleReplicas(t *testing.T) {
	t.Parallel()

	Convey("GetAllStaleReplicas works", t, func() {
		Convey("succeeds even when empty", func() {
			ctx := memory.Use(context.Background())

			replicas, err := GetAllStaleReplicas(ctx, 1000)
			So(err, ShouldBeNil)
			So(replicas, ShouldBeEmpty)
		})

		Convey("returns only stale replicas", func() {
			// Set up replica states in the datastore.
			ctx := memory.Use(context.Background())
			So(datastore.Put(ctx,
				testReplicaState(ctx, "dev~appG", 1),
				testReplicaState(ctx, "dev~appF", 2),
				testReplicaState(ctx, "dev~appE", 3),
				testReplicaState(ctx, "dev~appA", 4),
				testReplicaState(ctx, "dev~appB", 5),
				testReplicaState(ctx, "dev~appC", 3),
				testReplicaState(ctx, "dev~appD", 2),
			), ShouldBeNil)

			// Check the replicas returned were limited to stale ones, and are
			// in ascending order of the app ID.
			replicas, err := GetAllStaleReplicas(ctx, 4)
			So(err, ShouldBeNil)
			So(replicas, ShouldResembleProto, []*AuthReplicaState{
				testReplicaState(ctx, "dev~appC", 3),
				testReplicaState(ctx, "dev~appD", 2),
				testReplicaState(ctx, "dev~appE", 3),
				testReplicaState(ctx, "dev~appF", 2),
				testReplicaState(ctx, "dev~appG", 1),
			})
		})
	})
}

func TestGetAuthReplicaState(t *testing.T) {
	t.Parallel()

	Convey("getAuthReplicaState works", t, func() {
		ctx := memory.Use(context.Background())

		r := testReplicaState(ctx, "dev~appA", 123)

		_, err := getAuthReplicaState(ctx, replicaStateKey(ctx, "dev~appA"))
		So(err, ShouldEqual, datastore.ErrNoSuchEntity)

		// Now set up the replica state.
		So(datastore.Put(ctx, r), ShouldBeNil)

		actual, err := getAuthReplicaState(ctx, replicaStateKey(ctx, "dev~appA"))
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, r)
	})
}

func TestUpdateReplicaStateOnSuccess(t *testing.T) {
	t.Parallel()

	Convey("updateReplicaStateOnSuccess works", t, func() {
		ctx := memory.Use(context.Background())

		started := testCreatedTS
		finished := testModifiedTS
		currentRevision := &protocol.AuthDBRevision{
			PrimaryId:  "chrome-infra-auth-test",
			AuthDbRev:  1001,
			ModifiedTs: testModifiedTS.UnixMicro(),
		}
		authCodeVersion := "2.0.test"

		Convey("returns error for unknown replica", func() {
			storedRev, err := updateReplicaStateOnSuccess(ctx, "dev~unknownApp",
				started, finished, currentRevision, authCodeVersion)
			So(err, ShouldEqual, datastore.ErrNoSuchEntity)
			So(storedRev, ShouldEqual, 0)
		})

		Convey("updates last known state", func() {
			testAppID := "dev~appA"
			initialState := testReplicaState(ctx, testAppID, 1000)
			So(datastore.Put(ctx, initialState), ShouldBeNil)

			storedRev, err := updateReplicaStateOnSuccess(ctx, testAppID,
				started, finished, currentRevision, authCodeVersion)
			So(err, ShouldBeNil)
			So(storedRev, ShouldEqual, 1001)

			// Check the replica state was updated.
			actual := &AuthReplicaState{
				Kind:   "AuthReplicaState",
				ID:     testAppID,
				Parent: ReplicasRootKey(ctx),
			}
			So(datastore.Get(ctx, actual), ShouldBeNil)
			So(actual, ShouldResemble, &AuthReplicaState{
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
			})
		})
	})
}

func TestUpdateReplicaStateOnFail(t *testing.T) {
	t.Parallel()

	Convey("updateReplicaStateOnSuccess works", t, func() {
		ctx := memory.Use(context.Background())

		started := testCreatedTS
		finished := testModifiedTS
		oldReplicaRev := int64(1000)
		errMsg := "replica returned transient error"
		pushErr := &ReplicaUpdateError{
			RootErr: fmt.Errorf(errMsg),
			IsFatal: false,
		}

		Convey("returns error for unknown replica", func() {
			storedRev, err := updateReplicaStateOnFail(ctx, "dev~unknownApp",
				oldReplicaRev, started, finished, pushErr)
			So(err, ShouldEqual, datastore.ErrNoSuchEntity)
			So(storedRev, ShouldEqual, 0)
		})

		Convey("exits early if the replica has a newer revision", func() {
			testAppID := "dev~appA"
			advancedRev := oldReplicaRev + 1
			initialState := testReplicaState(ctx, testAppID, advancedRev)
			So(datastore.Put(ctx, initialState), ShouldBeNil)

			storedRev, err := updateReplicaStateOnFail(ctx, testAppID,
				oldReplicaRev, started, finished, pushErr)
			So(err, ShouldBeNil)
			So(storedRev, ShouldEqual, advancedRev)

			// Check the replica state was unchanged.
			actual := &AuthReplicaState{
				Kind:   "AuthReplicaState",
				ID:     testAppID,
				Parent: ReplicasRootKey(ctx),
			}
			So(datastore.Get(ctx, actual), ShouldBeNil)
			So(actual, ShouldResemble, initialState)
		})

		Convey("updates last known state with error details", func() {
			testAppID := "dev~appA"
			initialState := testReplicaState(ctx, testAppID, oldReplicaRev)
			So(datastore.Put(ctx, initialState), ShouldBeNil)

			storedRev, err := updateReplicaStateOnFail(ctx, testAppID,
				oldReplicaRev, started, finished, pushErr)
			So(err, ShouldBeNil)
			So(storedRev, ShouldEqual, oldReplicaRev)

			// Check the replica state was updated.
			actual := &AuthReplicaState{
				Kind:   "AuthReplicaState",
				ID:     testAppID,
				Parent: ReplicasRootKey(ctx),
			}
			So(datastore.Get(ctx, actual), ShouldBeNil)
			So(actual, ShouldResemble, &AuthReplicaState{
				Kind:           "AuthReplicaState",
				ID:             testAppID,
				Parent:         ReplicasRootKey(ctx),
				ReplicaURL:     initialState.ReplicaURL,
				AuthDBRev:      oldReplicaRev,
				PushStartedTS:  started,
				PushFinishedTS: finished,
				PushStatus:     ReplicaPushStatusTransientError,
				PushError:      errMsg,
			})
		})
	})
}
