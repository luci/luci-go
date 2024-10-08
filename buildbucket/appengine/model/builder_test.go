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
	"time"

	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"

	pb "go.chromium.org/luci/buildbucket/proto"
)

func TestBuilderStat(t *testing.T) {
	t.Parallel()

	ftt.Run("BuilderStat", t, func(t *ftt.Test) {
		ctx := memory.Use(context.Background())
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		ts := testclock.TestTimeUTC
		assert.Loosely(t, datastore.Put(ctx, &BuilderStat{
			ID:            "proj:bucket:builder1",
			LastScheduled: ts,
		}), should.BeNil)

		t.Run("update builder", func(t *ftt.Test) {
			builds := []*Build{
				{
					ID: 1,
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "proj",
							Bucket:  "bucket",
							Builder: "builder1",
						},
					},
				},
				{
					ID: 2,
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "proj",
							Bucket:  "bucket",
							Builder: "builder2",
						},
					},
				},
				{
					ID: 3,
					Proto: &pb.Build{
						Builder: &pb.BuilderID{
							Project: "proj",
							Bucket:  "bucket",
							Builder: "builder1",
						},
					},
				},
			}
			now := ts.Add(3600 * time.Second)
			err := UpdateBuilderStat(ctx, builds, now)
			assert.Loosely(t, err, should.BeNil)

			currentBuilders := []*BuilderStat{
				{ID: "proj:bucket:builder1"},
				{ID: "proj:bucket:builder2"},
			}
			err = datastore.Get(ctx, currentBuilders)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, currentBuilders, should.Resemble([]*BuilderStat{
				{
					ID:            "proj:bucket:builder1",
					LastScheduled: datastore.RoundTime(now),
				},
				{
					ID:            "proj:bucket:builder2",
					LastScheduled: datastore.RoundTime(now),
				},
			}))
		})

		t.Run("uninitialized build.proto.builder", func(t *ftt.Test) {
			builds := []*Build{{ID: 1}}
			assert.Loosely(t, func() { UpdateBuilderStat(ctx, builds, ts) }, should.Panic)
		})
	})
}
