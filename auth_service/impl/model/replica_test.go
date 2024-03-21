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

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth/service/protocol"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func testReplicaState(ctx context.Context, appID string, authDBRev int64) *AuthReplicaState {
	return &AuthReplicaState{
		Kind:      "AuthReplicaState",
		ID:        appID,
		Parent:    replicasRootKey(ctx),
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
				Parent: replicasRootKey(ctx),
			}
			So(datastore.Get(ctx, actual), ShouldBeNil)
			So(actual, ShouldResemble, &AuthReplicaState{
				Kind:            "AuthReplicaState",
				ID:              testAppID,
				Parent:          replicasRootKey(ctx),
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
