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

package handler

import (
	"context"
	"sort"
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/changelist"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
)

func TestRemoveRunFromCLs(t *testing.T) {
	t.Parallel()

	Convey("RemoveRunFromCLs", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		run1 := common.MakeRunID("infra", ct.Clock.Now(), 1, []byte("deadbeef"))
		run2 := common.MakeRunID("infra", ct.Clock.Now(), 1, []byte("cafecafe"))
		runs := common.RunIDs{run1, run2}
		sort.Sort(runs)
		Convey("Works", func() {
			err := datastore.Put(ctx, &changelist.CL{
				ID:             1,
				IncompleteRuns: runs,
				EVersion:       3,
				UpdateTime:     clock.Now(ctx).UTC(),
			})
			So(err, ShouldBeNil)

			ct.Clock.Add(1 * time.Hour)
			now := clock.Now(ctx).UTC()
			err = datastore.RunInTransaction(ctx, func(ctx context.Context) error {
				return removeRunFromCLs(ctx, run1, common.CLIDs{1})
			}, nil)
			So(err, ShouldBeNil)

			cl := changelist.CL{ID: 1}
			So(datastore.Get(ctx, &cl), ShouldBeNil)
			So(cl, ShouldResemble, changelist.CL{
				ID:             1,
				IncompleteRuns: common.RunIDs{run2},
				EVersion:       4,
				UpdateTime:     now,
			})
		})
	})
}
