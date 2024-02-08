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
	"context"
	"slices"
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
		ctx, cancel := ct.SetUp(t)
		defer cancel()
		const bbHost = "buildbucket.example.com"
		now := ct.Clock.Now().UTC()
		var runID = common.MakeRunID("infra", now.Add(-1*time.Hour), 1, []byte("foo"))
		var notified map[common.RunID]common.TryjobIDs
		notifyFn := func(ctx context.Context, runID common.RunID, events *TryjobUpdatedEvents) error {
			if notified == nil {
				notified = make(map[common.RunID]common.TryjobIDs)
			}
			for _, evt := range events.Events {
				notified[runID] = append(notified[runID], common.TryjobID(evt.TryjobId))
			}
			return nil
		}

		Convey("No internal ID", func() {
			Convey("No external ID", func() {
				tj := &Tryjob{
					EVersion:         1,
					EntityCreateTime: now,
					EntityUpdateTime: now,
					LaunchedBy:       runID,
				}
				So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return SaveTryjobs(ctx, []*Tryjob{tj}, notifyFn)
				}, nil), ShouldBeNil)
				So(tj.ID, ShouldNotBeEmpty)
				So(notified, ShouldResemble, map[common.RunID]common.TryjobIDs{
					runID: {tj.ID},
				})
			})
			Convey("External ID provided", func() {
				Convey("Does not map to any existing tryjob", func() {
					eid := MustBuildbucketID(bbHost, 10)
					tj := &Tryjob{
						ExternalID:       eid,
						EVersion:         1,
						EntityCreateTime: now,
						EntityUpdateTime: now,
						LaunchedBy:       runID,
					}
					So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return SaveTryjobs(ctx, []*Tryjob{tj}, notifyFn)
					}, nil), ShouldBeNil)
					tj, err := eid.Load(ctx)
					So(err, ShouldBeNil)
					So(tj, ShouldNotBeNil)
					So(notified, ShouldResemble, map[common.RunID]common.TryjobIDs{
						runID: {tj.ID},
					})
				})
				Convey("Map to an existing tryjob", func() {
					eid := MustBuildbucketID(bbHost, 10)
					eid.MustCreateIfNotExists(ctx)
					tj := &Tryjob{
						ExternalID:       eid,
						EVersion:         1,
						EntityCreateTime: now,
						EntityUpdateTime: now,
						LaunchedBy:       runID,
					}
					So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return SaveTryjobs(ctx, []*Tryjob{tj}, notifyFn)
					}, nil), ShouldErrLike, "has already mapped to internal id")
					So(notified, ShouldBeEmpty)
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
					ReusedBy:         common.RunIDs{runID},
				}
				So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
					return SaveTryjobs(ctx, []*Tryjob{tj}, notifyFn)
				}, nil), ShouldBeNil)
				So(notified, ShouldResemble, map[common.RunID]common.TryjobIDs{
					runID: {tjID},
				})
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
						ReusedBy:         common.RunIDs{runID},
					}
					So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
						return SaveTryjobs(ctx, []*Tryjob{tj}, notifyFn)
					}, nil), ShouldBeNil)
					So(eid.MustLoad(ctx).ID, ShouldEqual, tjID)
					So(notified, ShouldResemble, map[common.RunID]common.TryjobIDs{
						runID: {tjID},
					})
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
							ReusedBy:         common.RunIDs{runID},
						}
						So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
							return SaveTryjobs(ctx, []*Tryjob{tj}, notifyFn)
						}, nil), ShouldBeNil)
						So(eid.MustLoad(ctx).EVersion, ShouldEqual, existing.EVersion+1)
						So(notified, ShouldResemble, map[common.RunID]common.TryjobIDs{
							runID: {tj.ID},
						})
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
							ReusedBy:         common.RunIDs{runID},
						}
						So(datastore.RunInTransaction(ctx, func(ctx context.Context) error {
							return SaveTryjobs(ctx, []*Tryjob{tj}, notifyFn)
						}, nil), ShouldErrLike, "has already mapped to")
						So(notified, ShouldBeEmpty)
					})
				})
			})
		})
	})
}

func TestQueryTryjobIDsUpdatedBefore(t *testing.T) {
	t.Parallel()

	Convey("QueryTryjobIDsUpdatedBefore", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp(t)
		defer cancel()

		nextBuildID := int64(1)
		createNTryjobs := func(n int) []*Tryjob {
			tryjobs := make([]*Tryjob, n)
			for i := range tryjobs {
				eid := MustBuildbucketID("example.com", nextBuildID)
				nextBuildID++
				tryjobs[i] = eid.MustCreateIfNotExists(ctx)
			}
			return tryjobs
		}

		var allTryjobs []*Tryjob
		allTryjobs = append(allTryjobs, createNTryjobs(1000)...)
		ct.Clock.Add(1 * time.Minute)
		allTryjobs = append(allTryjobs, createNTryjobs(1000)...)
		ct.Clock.Add(1 * time.Minute)
		allTryjobs = append(allTryjobs, createNTryjobs(1000)...)

		before := ct.Clock.Now().Add(-30 * time.Second)
		var expected common.TryjobIDs
		for _, tj := range allTryjobs {
			if tj.EntityUpdateTime.Before(before) {
				expected = append(expected, tj.ID)
			}
		}
		slices.Sort(expected)

		actual, err := QueryTryjobIDsUpdatedBefore(ctx, before)
		So(err, ShouldBeNil)
		So(actual, ShouldResemble, expected)
	})
}
