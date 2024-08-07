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

package execute

import (
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	bbpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/gae/service/datastore"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/run"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
)

func TestFindReuseInCV(t *testing.T) {
	t.Parallel()

	Convey("FindReuseInCV", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		const reuseKey = "cafecafe"
		const lProject = "testProj"
		const tjID = 6436
		var runID = common.MakeRunID(lProject, ct.Clock.Now().Add(-1*time.Hour), 1, []byte("abcd"))
		w := &worker{
			run: &run.Run{
				ID:   runID,
				Mode: run.DryRun,
			},
			knownTryjobIDs: make(common.TryjobIDSet),
			reuseKey:       reuseKey,
			rm:             run.NewNotifier(ct.TQDispatcher),
		}
		builder := &bbpb.BuilderID{
			Project: "ProjectFoo",
			Bucket:  "BucketFoo",
			Builder: "BuilderFoo",
		}
		defFoo := &tryjob.Definition{
			Backend: &tryjob.Definition_Buildbucket_{
				Buildbucket: &tryjob.Definition_Buildbucket{
					Host:    "buildbucket.example.com",
					Builder: builder,
				},
			},
		}

		now := ct.Clock.Now().UTC()
		tj := &tryjob.Tryjob{
			ID:               tjID,
			ExternalID:       tryjob.MustBuildbucketID("buildbucket.example.com", 9821),
			EVersion:         2,
			EntityCreateTime: now.Add(-staleTryjobAge / 2),
			EntityUpdateTime: now.Add(-1 * time.Minute),
			ReuseKey:         reuseKey,
			Definition:       defFoo,
			Status:           tryjob.Status_ENDED,
			LaunchedBy:       common.MakeRunID(lProject, now.Add(-2*time.Hour), 1, []byte("efgh")),
			Result: &tryjob.Result{
				CreateTime: timestamppb.New(now.Add(-staleTryjobAge / 2)),
				Backend: &tryjob.Result_Buildbucket_{
					Buildbucket: &tryjob.Result_Buildbucket{
						Builder: builder,
					},
				},
				Status: tryjob.Result_SUCCEEDED,
			},
		}

		Convey("No tryjob to reuse", func() {
			result, err := w.findReuseInCV(ctx, []*tryjob.Definition{defFoo})
			So(err, ShouldBeNil)
			So(result, ShouldBeEmpty)
		})

		Convey("Found Reuse", func() {
			So(datastore.Put(ctx, tj), ShouldBeNil)
			result, err := w.findReuseInCV(ctx, []*tryjob.Definition{defFoo})
			So(err, ShouldBeNil)
			So(result, ShouldHaveLength, 1)
			So(result, ShouldContainKey, defFoo)
			So(result[defFoo].EVersion, ShouldEqual, tj.EVersion+1)
			So(result[defFoo].EntityUpdateTime, ShouldEqual, now)
			So(result[defFoo].ReusedBy, ShouldResemble, common.RunIDs{runID})
		})

		Convey("Definition doesn't match but builder matches", func() {
			tj.Definition = &tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host: "buildbucket.example.com",
						Builder: &bbpb.BuilderID{
							Project: "ProjectBar",
							Bucket:  "BucketBar",
							Builder: "BuilderBar",
						},
					},
				},
				EquivalentTo: defFoo,
			}
			So(datastore.Put(ctx, tj), ShouldBeNil)
			result, err := w.findReuseInCV(ctx, []*tryjob.Definition{defFoo})
			So(err, ShouldBeNil)
			So(result, ShouldHaveLength, 1)
			So(result, ShouldContainKey, defFoo)
		})

		Convey("None of definition and builder matches", func() {
			tj.Definition = &tryjob.Definition{
				Backend: &tryjob.Definition_Buildbucket_{
					Buildbucket: &tryjob.Definition_Buildbucket{
						Host: "buildbucket.example.com",
						Builder: &bbpb.BuilderID{
							Project: "ProjectBar",
							Bucket:  "BucketBar",
							Builder: "BuilderBar",
						},
					},
				},
			}
			tj.Result.GetBuildbucket().Builder = &bbpb.BuilderID{
				Project: "ProjectBar",
				Bucket:  "BucketBar",
				Builder: "BuilderBar",
			}
			So(datastore.Put(ctx, tj), ShouldBeNil)
			result, err := w.findReuseInCV(ctx, []*tryjob.Definition{defFoo})
			So(err, ShouldBeNil)
			So(result, ShouldBeEmpty)
		})

		Convey("Tryjob already known", func() {
			w.knownTryjobIDs.Add(tjID)
			So(datastore.Put(ctx, tj), ShouldBeNil)
			result, err := w.findReuseInCV(ctx, []*tryjob.Definition{defFoo})
			So(err, ShouldBeNil)
			So(result, ShouldBeEmpty)
		})

		Convey("Tryjob is from different project", func() {
			tj.LaunchedBy = common.MakeRunID("anotherProj", now.Add(-2*time.Hour), 1, []byte("cool"))
			So(datastore.Put(ctx, tj), ShouldBeNil)
			result, err := w.findReuseInCV(ctx, []*tryjob.Definition{defFoo})
			So(err, ShouldBeNil)
			So(result, ShouldBeEmpty)
		})

		Convey("Tryjob is not reusable", func() {
			tj.Result.CreateTime = timestamppb.New(now.Add(-2 * staleTryjobAge))
			So(datastore.Put(ctx, tj), ShouldBeNil)
			result, err := w.findReuseInCV(ctx, []*tryjob.Definition{defFoo})
			So(err, ShouldBeNil)
			So(result, ShouldBeEmpty)
		})

		Convey("Pick the latest one", func() {
			newerTryjob := *tj // shallow copy
			newerTryjob.ID = tjID - 1
			newerTryjob.EntityCreateTime = tj.EntityCreateTime.Add(1 * time.Minute)
			So(datastore.Put(ctx, tj, &newerTryjob), ShouldBeNil)
			result, err := w.findReuseInCV(ctx, []*tryjob.Definition{defFoo})
			So(err, ShouldBeNil)
			So(result, ShouldHaveLength, 1)
			So(result, ShouldContainKey, defFoo)
			So(result[defFoo].ID, ShouldEqual, newerTryjob.ID)
			So(result[defFoo].ReusedBy, ShouldResemble, common.RunIDs{runID})
			So(datastore.Get(ctx, tj), ShouldBeNil)
			So(tj.ReusedBy.Index(runID), ShouldBeLessThan, 0)
		})

		Convey("Run already in Tryjob", func() {
			tj.ReusedBy = common.RunIDs{runID}
			So(datastore.Put(ctx, tj), ShouldBeNil)
			result, err := w.findReuseInCV(ctx, []*tryjob.Definition{defFoo})
			So(err, ShouldBeNil)
			So(result, ShouldHaveLength, 1)
			So(result, ShouldContainKey, defFoo)
			So(result[defFoo].ID, ShouldEqual, tj.ID)
			So(result[defFoo].EVersion, ShouldEqual, tj.EVersion)
			So(result[defFoo].EntityUpdateTime, ShouldEqual, tj.EntityUpdateTime)
			So(result[defFoo].ReusedBy, ShouldResemble, common.RunIDs{runID})
		})
	})
}
