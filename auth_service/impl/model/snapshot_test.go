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

package model

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	. "github.com/smartystreets/goconvey/convey"
)

func TestTakeSnapshot(t *testing.T) {
	t.Parallel()

	Convey("Testing TakeSnapshot", t, func() {
		ctx := memory.Use(context.Background())

		_, err := TakeSnapshot(ctx)
		So(err.(errors.MultiError).First(), ShouldEqual, datastore.ErrNoSuchEntity)

		So(datastore.Put(ctx,
			testAuthReplicationState(ctx, 12345),
			testAuthGlobalConfig(ctx),
			testAuthGroup(ctx, "group-2", nil),
			testAuthGroup(ctx, "group-1", nil),
			testIPAllowlist(ctx, "ip-allowlist-2", nil),
			testIPAllowlist(ctx, "ip-allowlist-1", nil),
		), ShouldBeNil)

		snap, err := TakeSnapshot(ctx)
		So(err, ShouldBeNil)

		So(snap, ShouldResemble, &Snapshot{
			ReplicationState: testAuthReplicationState(ctx, 12345),
			GlobalConfig:     testAuthGlobalConfig(ctx),
			Groups: []*AuthGroup{
				testAuthGroup(ctx, "group-1", nil),
				testAuthGroup(ctx, "group-2", nil),
			},
			IPAllowlists: []*AuthIPAllowlist{
				testIPAllowlist(ctx, "ip-allowlist-1", nil),
				testIPAllowlist(ctx, "ip-allowlist-2", nil),
			},
		})
	})
}
