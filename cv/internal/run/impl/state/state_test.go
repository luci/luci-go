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

package state

import (
	"context"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRemoveRunFromCLs(t *testing.T) {
	t.Parallel()

	Convey("RemoveRunFromCLs", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		s := &RunState{
			Run: run.Run{
				ID:  common.RunID("chromium/111-2-deadbeef"),
				CLs: []common.CLID{1},
			},
		}
		Convey("Works", func() {
			err := datastore.Put(ctx, &changelist.CL{
				ID:             1,
				IncompleteRuns: common.MakeRunIDs("chromium/111-2-deadbeef", "infra/999-2-cafecafe"),
				EVersion:       3,
				UpdateTime:     clock.Now(ctx).UTC(),
			})
			So(err, ShouldBeNil)

			ct.Clock.Add(1 * time.Hour)
			now := clock.Now(ctx).UTC()
			err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				return s.RemoveRunFromCLs(ctx)
			}, nil)
			So(err, ShouldBeNil)

			cl := changelist.CL{ID: 1}
			So(datastore.Get(ctx, &cl), ShouldBeNil)
			So(cl, ShouldResemble, changelist.CL{
				ID:             1,
				IncompleteRuns: common.MakeRunIDs("infra/999-2-cafecafe"),
				EVersion:       4,
				UpdateTime:     now,
			})
		})
	})
}
