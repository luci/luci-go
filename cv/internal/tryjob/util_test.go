// Copyright 2022 The LUCI Authors.
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

package tryjob

import (
	"testing"
	"time"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func TestSaveTryjobs(t *testing.T) {
	t.Parallel()

	Convey("SaveTryjobs works", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()
		const bbHost = "buildbucket.example.com"
		now := ct.Clock.Now().UTC()

		Convey("No internal ID", func() {
			Convey("No external ID", func() {
				tj := &Tryjob{
					EVersion:         1,
					EntityCreateTime: now,
					EntityUpdateTime: now,
				}
				So(SaveTryjobs(ctx, []*Tryjob{tj}), ShouldBeNil)
				So(tj.ID, ShouldNotBeEmpty)
			})
			Convey("External ID provided", func() {
				Convey("Does not map to any existing tryjob", func() {
					eid := MustBuildbucketID(bbHost, 10)
					tj := &Tryjob{
						ExternalID:       eid,
						EVersion:         1,
						EntityCreateTime: now,
						EntityUpdateTime: now,
					}
					So(SaveTryjobs(ctx, []*Tryjob{tj}), ShouldBeNil)
					tj, err := eid.Load(ctx)
					So(err, ShouldBeNil)
					So(tj, ShouldNotBeNil)
				})
				Convey("Map to an existing tryjob", func() {
					eid := MustBuildbucketID(bbHost, 10)
					eid.MustCreateIfNotExists(ctx)
					tj := &Tryjob{
						ExternalID:       eid,
						EVersion:         1,
						EntityCreateTime: now,
						EntityUpdateTime: now,
					}
					So(SaveTryjobs(ctx, []*Tryjob{tj}), ShouldErrLike, "has already mapped to internal id")
				})
			})
		})

		Convey("Internal ID provided", func() {
			const tjID = common.TryjobID(45366)
			Convey("No external ID", func() {
				tj := &Tryjob{
					ID:               tjID,
					EVersion:         1,
					EntityCreateTime: now,
					EntityUpdateTime: now,
				}
				So(SaveTryjobs(ctx, []*Tryjob{tj}), ShouldBeNil)
			})
			Convey("External ID provided", func() {
				Convey("Does not map to any existing tryjob", func() {
					eid := MustBuildbucketID(bbHost, 10)
					tj := &Tryjob{
						ID:               tjID,
						ExternalID:       eid,
						EVersion:         1,
						EntityCreateTime: now,
						EntityUpdateTime: now,
					}
					So(SaveTryjobs(ctx, []*Tryjob{tj}), ShouldBeNil)
					So(eid.MustLoad(ctx).ID, ShouldEqual, tjID)
				})

				Convey("Map to an existing tryjob", func() {
					Convey("existing tryjob has the same ID", func() {
						eid := MustBuildbucketID(bbHost, 10)
						existing := eid.MustCreateIfNotExists(ctx)
						ct.Clock.Add(10 * time.Minute)
						tj := &Tryjob{
							ID:               existing.ID,
							ExternalID:       eid,
							EVersion:         existing.EVersion + 1,
							EntityUpdateTime: datastore.RoundTime(clock.Now(ctx).UTC()),
						}
						So(SaveTryjobs(ctx, []*Tryjob{tj}), ShouldBeNil)
						So(eid.MustLoad(ctx).EVersion, ShouldEqual, existing.EVersion+1)
					})
					Convey("existing tryjob has a different ID", func() {
						eid := MustBuildbucketID(bbHost, 10)
						existing := eid.MustCreateIfNotExists(ctx)
						ct.Clock.Add(10 * time.Minute)
						tj := &Tryjob{
							ID:               existing.ID + 10,
							ExternalID:       eid,
							EVersion:         existing.EVersion + 1,
							EntityUpdateTime: datastore.RoundTime(clock.Now(ctx).UTC()),
						}
						So(SaveTryjobs(ctx, []*Tryjob{tj}), ShouldErrLike, "has already mapped to")
					})
				})
			})
		})
	})
}
