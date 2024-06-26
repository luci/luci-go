// Copyright 2017 The LUCI Authors.
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

package buildbucket

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang/mock/gomock"
	"github.com/gomodule/redigo/redis"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/router"

	"go.chromium.org/luci/milo/internal/model"
	"go.chromium.org/luci/milo/internal/model/milostatus"
	"go.chromium.org/luci/milo/internal/projectconfig"
	"go.chromium.org/luci/milo/internal/utils"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

func newMockClient(c context.Context, t *testing.T) (context.Context, *gomock.Controller, *buildbucketpb.MockBuildsClient) {
	ctrl := gomock.NewController(t)
	client := buildbucketpb.NewMockBuildsClient(ctrl)
	factory := func(c context.Context, host string, as auth.RPCAuthorityKind, opts ...auth.RPCOption) (buildbucketpb.BuildsClient, error) {
		return client, nil
	}
	return WithBuildsClientFactory(c, factory), ctrl, client
}

// Buildbucket timestamps round off to milliseconds, so define a reference.
var RefTime = time.Date(2016, time.February, 3, 4, 5, 6, 0, time.UTC)

func makeReq(build *buildbucketpb.Build) io.ReadCloser {
	bmsg := &buildbucketpb.BuildsV2PubSub{Build: build}
	bm, _ := protojson.Marshal(bmsg)

	msg := utils.PubSubSubscription{
		Message: utils.PubSubMessage{
			Data: base64.StdEncoding.EncodeToString(bm),
		},
	}
	jmsg, _ := json.Marshal(msg)
	return io.NopCloser(bytes.NewReader(jmsg))
}

func TestPubSub(t *testing.T) {
	t.Parallel()

	Convey(`TestPubSub`, t, func() {
		c := gaetesting.TestingContextWithAppID("luci-milo-dev")
		datastore.GetTestable(c).Consistent(true)
		c, _ = testclock.UseTime(c, RefTime)
		c = auth.WithState(c, &authtest.FakeState{
			Identity:       identity.AnonymousIdentity,
			IdentityGroups: []string{"all"},
		})
		c = caching.WithRequestCache(c)

		// Initialize the appropriate builder.
		builderSummary := &model.BuilderSummary{
			BuilderID: "buildbucket/luci.fake.bucket/fake_builder",
		}
		err := datastore.Put(c, builderSummary)
		So(err, ShouldBeNil)

		// Initialize the appropriate project config.
		err = datastore.Put(c, &projectconfig.Project{
			ID: "fake",
		})
		So(err, ShouldBeNil)

		// We'll copy this LegacyApiCommonBuildMessage base for convenience.
		buildBase := &buildbucketpb.Build{
			Builder: &buildbucketpb.BuilderID{
				Project: "fake",
				Bucket:  "bucket",
				Builder: "fake_builder",
			},
			Infra: &buildbucketpb.BuildInfra{
				Buildbucket: &buildbucketpb.BuildInfra_Buildbucket{
					Hostname: "hostname",
				},
			},
			Input:      &buildbucketpb.Build_Input{},
			Output:     &buildbucketpb.Build_Output{},
			CreatedBy:  string(identity.AnonymousIdentity),
			CreateTime: timestamppb.New(RefTime.Add(2 * time.Hour)),
		}

		Convey("New in-process build", func() {
			bKey := model.MakeBuildKey(c, "hostname", "1234")
			buildExp := proto.Clone(buildBase).(*buildbucketpb.Build)
			buildExp.Id = 1234
			buildExp.Status = buildbucketpb.Status_STARTED
			buildExp.CreateTime = timestamppb.New(RefTime.Add(2 * time.Hour))
			buildExp.StartTime = timestamppb.New(RefTime.Add(3 * time.Hour))
			buildExp.UpdateTime = timestamppb.New(RefTime.Add(5 * time.Hour))
			buildExp.Input.Experimental = true
			propertiesMap := map[string]any{
				"$recipe_engine/milo/blamelist_pins": []any{
					map[string]any{
						"host":    "chromium.googlesource.com",
						"id":      "8930f18245df678abc944376372c77ba5e2a658b",
						"project": "angle/angle",
					},
					map[string]any{
						"host":    "chromium.googlesource.com",
						"id":      "07033c702f81a75dfc2d83888ba3f8b354d0e920",
						"project": "chromium/src",
					},
				},
			}
			buildExp.Output.Properties, _ = structpb.NewStruct(propertiesMap)

			h := httptest.NewRecorder()
			r := &http.Request{Body: makeReq(buildExp)}
			PubSubHandler(&router.Context{
				Writer:  h,
				Request: r.WithContext(c),
			})
			So(h.Code, ShouldEqual, 200)
			datastore.GetTestable(c).CatchupIndexes()

			Convey("stores BuildSummary and BuilderSummary", func() {
				buildAct := model.BuildSummary{BuildKey: bKey}
				err := datastore.Get(c, &buildAct)
				So(err, ShouldBeNil)
				So(buildAct.BuildKey.String(), ShouldEqual, bKey.String())
				So(buildAct.BuilderID, ShouldEqual, "buildbucket/luci.fake.bucket/fake_builder")
				So(buildAct.Summary, ShouldResemble, model.Summary{
					Status: milostatus.Running,
					Start:  RefTime.Add(3 * time.Hour),
				})
				So(buildAct.Created, ShouldResemble, RefTime.Add(2*time.Hour))
				So(buildAct.Experimental, ShouldBeTrue)
				So(buildAct.BlamelistPins, ShouldResemble, []string{
					"commit/gitiles/chromium.googlesource.com/angle/angle/+/8930f18245df678abc944376372c77ba5e2a658b",
					"commit/gitiles/chromium.googlesource.com/chromium/src/+/07033c702f81a75dfc2d83888ba3f8b354d0e920",
				})

				blder := model.BuilderSummary{BuilderID: "buildbucket/luci.fake.bucket/fake_builder"}
				err = datastore.Get(c, &blder)
				So(err, ShouldBeNil)
				So(blder.LastFinishedStatus, ShouldResemble, milostatus.NotRun)
				So(blder.LastFinishedBuildID, ShouldEqual, "")
			})
		})

		Convey("Completed build", func() {
			bKey := model.MakeBuildKey(c, "hostname", "2234")
			buildExp := buildBase
			buildExp.Id = 2234
			buildExp.Status = buildbucketpb.Status_SUCCESS
			buildExp.CreateTime = timestamppb.New(RefTime.Add(2 * time.Hour))
			buildExp.StartTime = timestamppb.New(RefTime.Add(3 * time.Hour))
			buildExp.UpdateTime = timestamppb.New(RefTime.Add(6 * time.Hour))
			buildExp.EndTime = timestamppb.New(RefTime.Add(6 * time.Hour))
			buildExp.Input.GitilesCommit = &buildbucketpb.GitilesCommit{
				Host:    "chromium.googlesource.com",
				Id:      "8930f18245df678abc944376372c77ba5e2a658b",
				Project: "angle/angle",
			}

			h := httptest.NewRecorder()
			r := &http.Request{Body: makeReq(buildExp)}
			PubSubHandler(&router.Context{
				Writer:  h,
				Request: r.WithContext(c),
			})
			So(h.Code, ShouldEqual, 200)

			Convey("stores BuildSummary and BuilderSummary", func() {
				buildAct := model.BuildSummary{BuildKey: bKey}
				err := datastore.Get(c, &buildAct)
				So(err, ShouldBeNil)
				So(buildAct.BuildKey.String(), ShouldEqual, bKey.String())
				So(buildAct.BuilderID, ShouldEqual, "buildbucket/luci.fake.bucket/fake_builder")
				So(buildAct.Summary, ShouldResemble, model.Summary{
					Status: milostatus.Success,
					Start:  RefTime.Add(3 * time.Hour),
					End:    RefTime.Add(6 * time.Hour),
				})
				So(buildAct.Created, ShouldResemble, RefTime.Add(2*time.Hour))

				blder := model.BuilderSummary{BuilderID: "buildbucket/luci.fake.bucket/fake_builder"}
				err = datastore.Get(c, &blder)
				So(err, ShouldBeNil)
				So(blder.LastFinishedCreated, ShouldResemble, RefTime.Add(2*time.Hour))
				So(blder.LastFinishedStatus, ShouldResemble, milostatus.Success)
				So(blder.LastFinishedBuildID, ShouldEqual, "buildbucket/2234")
				So(buildAct.BlamelistPins, ShouldResemble, []string{
					"commit/gitiles/chromium.googlesource.com/angle/angle/+/8930f18245df678abc944376372c77ba5e2a658b",
				})
			})

			Convey("results in earlier update not being ingested", func() {
				eBuild := &buildbucketpb.Build{
					Id: 2234,
					Builder: &buildbucketpb.BuilderID{
						Project: "fake",
						Bucket:  "bucket",
						Builder: "fake_builder",
					},
					Infra: &buildbucketpb.BuildInfra{
						Buildbucket: &buildbucketpb.BuildInfra_Buildbucket{
							Hostname: "hostname",
						},
					},
					CreatedBy:  string(identity.AnonymousIdentity),
					CreateTime: timestamppb.New(RefTime.Add(2 * time.Hour)),
					StartTime:  timestamppb.New(RefTime.Add(3 * time.Hour)),
					UpdateTime: timestamppb.New(RefTime.Add(4 * time.Hour)),
					Status:     buildbucketpb.Status_STARTED,
				}

				h := httptest.NewRecorder()
				r := &http.Request{Body: makeReq(eBuild)}
				PubSubHandler(&router.Context{
					Writer:  h,
					Request: r.WithContext(c),
				})
				So(h.Code, ShouldEqual, 200)

				buildAct := model.BuildSummary{BuildKey: bKey}
				err := datastore.Get(c, &buildAct)
				So(err, ShouldBeNil)
				So(buildAct.Summary, ShouldResemble, model.Summary{
					Status: milostatus.Success,
					Start:  RefTime.Add(3 * time.Hour),
					End:    RefTime.Add(6 * time.Hour),
				})
				So(buildAct.Created, ShouldResemble, RefTime.Add(2*time.Hour))

				blder := model.BuilderSummary{BuilderID: "buildbucket/luci.fake.bucket/fake_builder"}
				err = datastore.Get(c, &blder)
				So(err, ShouldBeNil)
				So(blder.LastFinishedCreated, ShouldResemble, RefTime.Add(2*time.Hour))
				So(blder.LastFinishedStatus, ShouldResemble, milostatus.Success)
				So(blder.LastFinishedBuildID, ShouldEqual, "buildbucket/2234")
			})
		})
	})
}

func TestShouldUpdateBuilderSummary(t *testing.T) {
	Convey("TestShouldUpdateBuilderSummary", t, func() {
		c := context.Background()
		startTime := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC).
			Truncate(time.Duration(entityUpdateIntervalInS) * time.Second)

		// Set up a test redis server.
		s, err := miniredis.Run()
		So(err, ShouldBeNil)
		defer s.Close()
		c = redisconn.UsePool(c, &redis.Pool{
			Dial: func() (redis.Conn, error) {
				return redis.Dial("tcp", s.Addr())
			},
		})

		createBuildSummary := func(builderID string, status buildbucketpb.Status, createdAt time.Time) *model.BuildSummary {
			return &model.BuildSummary{
				BuilderID: builderID,
				Summary: model.Summary{
					Status: milostatus.FromBuildbucket(status),
				},
				Created: createdAt,
			}
		}

		Convey("Single call", func() {
			// Ensures `shouldUpdateBuilderSummary` is called at the start of the time bucket.
			c, _ := testclock.UseTime(c, startTime)

			start := clock.Now(c)
			// Should return without advancing the clock.
			shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-1", buildbucketpb.Status_SUCCESS, start))
			So(err, ShouldBeNil)
			So(shouldUpdate, ShouldBeTrue)
		})

		Convey("Single call followed by multiple parallel calls", func(tc C) {
			// Ensures all `shouldUpdateBuilderSummary` calls are in the same time bucket.
			c, tClock := testclock.UseTime(c, startTime)

			pivot := clock.Now(c).Add(-time.Hour)

			shouldUpdates := make([]bool, 4)

			shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-2", buildbucketpb.Status_SUCCESS, pivot))
			So(err, ShouldBeNil)
			shouldUpdates[0] = shouldUpdate

			err = parallel.FanOutIn(func(tasks chan<- func() error) {
				eventC := make(chan string)
				defer close(eventC)
				tClock.SetTimerCallback(func(d time.Duration, t clock.Timer) {
					eventC <- "timer"
				})

				tasks <- func() error {
					createdAt := pivot.Add(5 * time.Millisecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-2", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[1] = shouldUpdate
					return err
				}

				// Wait until the previous call reaches a blocking point.
				tc.So(<-eventC, ShouldEqual, "timer")
				tasks <- func() error {
					createdAt := pivot.Add(15 * time.Millisecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-2", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[2] = shouldUpdate
					return err
				}

				// Wait until the previous call reaches a blocking point.
				tc.So(<-eventC, ShouldEqual, "timer")
				tasks <- func() error {
					createdAt := pivot.Add(10 * time.Millisecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-2", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[3] = shouldUpdate
					eventC <- "return"
					return err
				}

				// Wait until the last shouldUpdateBuilderSummary call returns then
				// advance the clock to the next time bucket.
				tc.So(<-eventC, ShouldEqual, "return")
				tClock.Add(time.Duration(entityUpdateIntervalInS) * time.Second)
			})
			So(err, ShouldBeNil)

			// The first value should be true because there's no recent updates.
			So(shouldUpdates[0], ShouldBeTrue)

			// The second value should be false because there's a recent update, so it
			// moves to the next time bucket, wait for the timebucket to begin.
			// Then it's replaced by the next shouldUpdateBuilderSummary call.
			So(shouldUpdates[1], ShouldBeFalse)

			// The third value should be true because it has a newer build than the
			// current one. And it's not replaced by any new builds.
			So(shouldUpdates[2], ShouldBeTrue)

			// The forth value should be false because the build is not created earlier
			// than the build associated with the current pending update in it's pending bucket.
			So(shouldUpdates[3], ShouldBeFalse)
		})

		Convey("Single call followed by multiple parallel calls that are nanoseconds apart", func(tc C) {
			// This test ensures that the timestamp percision is not lost.

			// Ensures all `shouldUpdateBuilderSummary` calls are in the same time bucket.
			c, tClock := testclock.UseTime(c, startTime)

			pivot := clock.Now(c).Add(-time.Hour)

			shouldUpdates := make([]bool, 4)

			shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-3", buildbucketpb.Status_SUCCESS, pivot))
			So(err, ShouldBeNil)
			shouldUpdates[0] = shouldUpdate

			err = parallel.FanOutIn(func(tasks chan<- func() error) {
				eventC := make(chan string)
				defer close(eventC)
				tClock.SetTimerCallback(func(d time.Duration, t clock.Timer) {
					eventC <- "timer"
				})

				tasks <- func() error {
					createdAt := pivot.Add(time.Nanosecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-3", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[1] = shouldUpdate
					return err
				}

				// Wait until the previous call reaches a blocking point.
				tc.So(<-eventC, ShouldEqual, "timer")
				tasks <- func() error {
					createdAt := pivot.Add(3 * time.Nanosecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-3", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[2] = shouldUpdate
					return err
				}

				// Wait until the previous call reaches a blocking point.
				tc.So(<-eventC, ShouldEqual, "timer")
				tasks <- func() error {
					createdAt := pivot.Add(2 * time.Nanosecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-3", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[3] = shouldUpdate
					eventC <- "return"
					return err
				}

				// Wait until the last shouldUpdateBuilderSummary call returns then
				// advance the clock to the next time bucket.
				tc.So(<-eventC, ShouldEqual, "return")
				tClock.Add(time.Duration(entityUpdateIntervalInS) * time.Second)
			})
			So(err, ShouldBeNil)

			// The first value should be true because there's no pending/recent updates.
			So(shouldUpdates[0], ShouldBeTrue)

			// The second value should be false because there's a recent update, so it
			// moves to the next time bucket, wait for the timebucket to begin.
			// Then it's replaced by the next shouldUpdateBuilderSummary call.
			So(shouldUpdates[1], ShouldBeFalse)

			// The third value should be true because it has a newer build than the
			// current pending one. And it's not replaced by any new builds.
			So(shouldUpdates[2], ShouldBeTrue)

			// The forth value should be false because the build is not created earlier
			// than the build associated with the current pending update in it's pending bucket.
			So(shouldUpdates[3], ShouldBeFalse)
		})

		Convey("Single call followed by multiple parallel calls in different time buckets", func(tc C) {
			c, tClock := testclock.UseTime(c, startTime)

			pivot := clock.Now(c).Add(-time.Hour)
			shouldUpdates := make([]bool, 4)

			shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-4", buildbucketpb.Status_SUCCESS, pivot))
			So(err, ShouldBeNil)
			shouldUpdates[0] = shouldUpdate

			// Ensures the following `shouldUpdateBuilderSummary` calls are in a
			// different time bucket.
			tClock.Add(time.Duration(entityUpdateIntervalInS) * time.Second)

			err = parallel.FanOutIn(func(tasks chan<- func() error) {
				eventC := make(chan string)
				defer close(eventC)
				tClock.SetTimerCallback(func(d time.Duration, t clock.Timer) {
					eventC <- "timer"
				})

				tasks <- func() error {
					createdAt := pivot.Add(5 * time.Millisecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-4", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[1] = shouldUpdate
					eventC <- "return"
					return err
				}

				// Wait until the previous shouldUpdateBuilderSummary call returns.
				tc.So(<-eventC, ShouldEqual, "return")
				tasks <- func() error {
					createdAt := pivot.Add(15 * time.Millisecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-4", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[2] = shouldUpdate
					return err
				}

				// Wait until the previous call reaches a blocking point.
				tc.So(<-eventC, ShouldEqual, "timer")
				tasks <- func() error {
					createdAt := pivot.Add(20 * time.Millisecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-4", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[3] = shouldUpdate
					return err
				}

				// Wait until the last shouldUpdateBuilderSummary call returns then
				// advance the clock to the next time bucket.
				tc.So(<-eventC, ShouldEqual, "timer")
				tClock.Add(time.Duration(entityUpdateIntervalInS) * time.Second)
			})
			So(err, ShouldBeNil)

			// The first value should be true because there's no pending/recent updates.
			So(shouldUpdates[0], ShouldBeTrue)

			// The second value should be true because it has move to a new time bucket
			// and there's no pending/recent updates in that bucket.
			So(shouldUpdates[1], ShouldBeTrue)

			// The third value should be false because there's a recent update, so it
			// moves to the next time bucket, wait for the timebucket to begin.
			// Then it's replaced by the next shouldUpdateBuilderSummary call.
			So(shouldUpdates[2], ShouldBeFalse)

			// The forth value should be true because it has a newer build than the
			// current pending one. And it's not replaced by any new builds.
			So(shouldUpdates[3], ShouldBeTrue)
		})
	})
}

func TestDeleteOldBuilds(t *testing.T) {
	t.Parallel()

	Convey("DeleteOldBuilds", t, func() {
		now := time.Date(2020, 01, 01, 0, 0, 0, 0, time.UTC)

		ctx, _ := testclock.UseTime(memory.Use(context.Background()), now)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		createBuild := func(id string, t time.Time) *model.BuildSummary {
			b := &model.BuildSummary{
				BuildKey: model.MakeBuildKey(ctx, "host", id),
				Created:  t,
			}
			So(datastore.Put(ctx, b), ShouldBeNil)
			return b
		}

		Convey("keeps builds", func() {
			Convey("as old as BuildSummaryStorageDuration", func() {
				build := createBuild("1", now.Add(-BuildSummaryStorageDuration))
				So(DeleteOldBuilds(ctx), ShouldBeNil)
				So(datastore.Get(ctx, build), ShouldBeNil)
			})
			Convey("younger than BuildSummaryStorageDuration", func() {
				build := createBuild("2", now.Add(-BuildSummaryStorageDuration+time.Minute))
				So(DeleteOldBuilds(ctx), ShouldBeNil)
				So(datastore.Get(ctx, build), ShouldBeNil)
			})
		})

		Convey("deletes builds older than BuildSummaryStorageDuration", func() {
			build := createBuild("3", now.Add(-BuildSummaryStorageDuration-time.Minute))
			So(DeleteOldBuilds(ctx), ShouldBeNil)
			So(datastore.Get(ctx, build), ShouldEqual, datastore.ErrNoSuchEntity)
		})

		Convey("removes many builds", func() {
			bs := make([]*model.BuildSummary, 234)
			old := now.Add(-BuildSummaryStorageDuration - time.Minute)
			for i := range bs {
				bs[i] = createBuild(fmt.Sprintf("4-%d", i), old)
			}
			So(DeleteOldBuilds(ctx), ShouldBeNil)
			So(datastore.Get(ctx, bs), ShouldErrLike,
				"datastore: no such entity (and 233 other errors)")
		})
	})
}

func TestSyncBuilds(t *testing.T) {
	t.Parallel()

	Convey("SyncBuilds", t, func() {
		now := time.Date(2020, 01, 01, 0, 0, 0, 0, time.UTC)

		c, _ := testclock.UseTime(memory.Use(context.Background()), now)
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)

		createBuild := func(id string, t time.Time, status milostatus.Status) *model.BuildSummary {
			b := &model.BuildSummary{
				BuildKey:  model.MakeBuildKey(c, "host", id),
				BuilderID: "buildbucket/luci.proj.bucket/builder",
				BuildID:   "buildbucket/" + id,
				Created:   t,
				Summary: model.Summary{
					Status: status,
				},
				Version: t.UnixNano(),
			}
			So(datastore.Put(c, b), ShouldBeNil)
			return b
		}

		Convey("don't update builds", func() {
			Convey("as old as BuildSummaryStorageDuration", func() {
				build := createBuild("luci.proj.bucket/builder/1234", now.Add(-BuildSummarySyncThreshold), milostatus.Running)
				So(syncBuildsImpl(c), ShouldBeNil)
				So(datastore.Get(c, build), ShouldBeNil)
				So(build.Summary.Status, ShouldEqual, milostatus.Running)
			})

			Convey("younger than BuildSummaryStorageDuration", func() {
				build := createBuild("luci.proj.bucket/builder/1234", now.Add(-BuildSummarySyncThreshold+time.Minute), milostatus.NotRun)
				So(syncBuildsImpl(c), ShouldBeNil)
				So(datastore.Get(c, build), ShouldBeNil)
				So(build.Summary.Status, ShouldEqual, milostatus.NotRun)
			})
		})

		Convey("update builds older than BuildSummarySyncThreshold", func() {
			build := createBuild("luci.proj.bucket/builder/1234", now.Add(-BuildSummarySyncThreshold-time.Minute), milostatus.NotRun)

			c, ctrl, mbc := newMockClient(c, t)
			defer ctrl.Finish()
			mbc.EXPECT().GetBuild(gomock.Any(), gomock.Any()).Return(&buildbucketpb.Build{
				Number: 1234,
				Builder: &buildbucketpb.BuilderID{
					Project: "proj",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status:     buildbucketpb.Status_SUCCESS,
				CreateTime: timestamppb.New(build.Created),
				UpdateTime: timestamppb.New(build.Created.Add(time.Hour)),
			}, nil).AnyTimes()

			So(syncBuildsImpl(c), ShouldBeNil)
			So(datastore.Get(c, build), ShouldBeNil)
			So(build.Summary.Status, ShouldEqual, milostatus.Success)
		})

		Convey("ensure BuildKey stays the same", func() {
			build := createBuild("123456", now.Add(-BuildSummarySyncThreshold-time.Minute), milostatus.NotRun)

			c, ctrl, mbc := newMockClient(c, t)
			defer ctrl.Finish()
			mbc.EXPECT().GetBuild(gomock.Any(), gomock.Any()).Return(&buildbucketpb.Build{
				Id:     123456,
				Number: 1234,
				Builder: &buildbucketpb.BuilderID{
					Project: "proj",
					Bucket:  "bucket",
					Builder: "builder",
				},
				Status:     buildbucketpb.Status_SUCCESS,
				CreateTime: timestamppb.New(build.Created),
				UpdateTime: timestamppb.New(build.Created.Add(time.Hour)),
			}, nil).AnyTimes()

			So(syncBuildsImpl(c), ShouldBeNil)
			So(datastore.Get(c, build), ShouldBeNil)
			So(build.Summary.Status, ShouldEqual, milostatus.Success)

			buildWithNewKey := &model.BuildSummary{
				BuildKey: model.MakeBuildKey(c, "host", "luci.proj.bucket/builder/1234"),
			}
			So(datastore.Get(c, buildWithNewKey), ShouldEqual, datastore.ErrNoSuchEntity)
		})
	})
}
