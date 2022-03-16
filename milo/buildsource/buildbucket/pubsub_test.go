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
	"io"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	bbv1 "go.chromium.org/luci/common/api/buildbucket/buildbucket/v1"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/milo/common"
	"go.chromium.org/luci/milo/common/model"
	"go.chromium.org/luci/milo/common/model/milostatus"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/redisconn"
	"go.chromium.org/luci/server/router"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang/mock/gomock"
	"github.com/gomodule/redigo/redis"
	. "github.com/smartystreets/goconvey/convey"
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

func makeReq(build bbv1.LegacyApiCommonBuildMessage) io.ReadCloser {
	bmsg := struct {
		Build    bbv1.LegacyApiCommonBuildMessage `json:"build"`
		Hostname string                           `json:"hostname"`
	}{build, "hostname"}
	bm, _ := json.Marshal(bmsg)

	msg := common.PubSubSubscription{
		Message: common.PubSubMessage{
			Data: base64.StdEncoding.EncodeToString(bm),
		},
	}
	jmsg, _ := json.Marshal(msg)
	return ioutil.NopCloser(bytes.NewReader(jmsg))
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
		c, ctrl, mbc := newMockClient(c, t)
		defer ctrl.Finish()

		// Initialize the appropriate builder.
		builderSummary := &model.BuilderSummary{
			BuilderID: "buildbucket/luci.fake.bucket/fake_builder",
		}
		err := datastore.Put(c, builderSummary)
		So(err, ShouldBeNil)

		// Initialize the appropriate project config.
		err = datastore.Put(c, &common.Project{
			ID: "fake",
		})
		So(err, ShouldBeNil)

		// We'll copy this LegacyApiCommonBuildMessage base for convenience.
		buildBase := bbv1.LegacyApiCommonBuildMessage{
			Project:   "fake",
			Bucket:    "luci.fake.bucket",
			Tags:      []string{"builder:fake_builder"},
			CreatedBy: string(identity.AnonymousIdentity),
			CreatedTs: bbv1.FormatTimestamp(RefTime.Add(2 * time.Hour)),
		}

		Convey("New in-process build", func() {
			bKey := MakeBuildKey(c, "hostname", "1234")
			buildExp := buildBase
			buildExp.Id = 1234
			created := timestamppb.New(RefTime.Add(2 * time.Hour))
			started := timestamppb.New(RefTime.Add(3 * time.Hour))
			updated := timestamppb.New(RefTime.Add(5 * time.Hour))

			propertiesMap := map[string]interface{}{
				"$recipe_engine/milo/blamelist_pins": []interface{}{
					map[string]interface{}{
						"host":    "chromium.googlesource.com",
						"id":      "8930f18245df678abc944376372c77ba5e2a658b",
						"project": "angle/angle",
					},
					map[string]interface{}{
						"host":    "chromium.googlesource.com",
						"id":      "07033c702f81a75dfc2d83888ba3f8b354d0e920",
						"project": "chromium/src",
					},
				},
			}
			properties, _ := structpb.NewStruct(propertiesMap)

			mbc.EXPECT().GetBuild(gomock.Any(), gomock.Any()).Return(&buildbucketpb.Build{
				Id:         1234,
				Status:     buildbucketpb.Status_STARTED,
				CreateTime: created,
				StartTime:  started,
				UpdateTime: updated,
				Builder: &buildbucketpb.BuilderID{
					Project: "fake",
					Bucket:  "bucket",
					Builder: "fake_builder",
				},
				Input: &buildbucketpb.Build_Input{
					Experimental: true,
				},
				Output: &buildbucketpb.Build_Output{
					Properties: properties,
				},
			}, nil).AnyTimes()

			h := httptest.NewRecorder()
			r := &http.Request{Body: makeReq(buildExp)}
			PubSubHandler(&router.Context{
				Context: c,
				Writer:  h,
				Request: r,
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
			bKey := MakeBuildKey(c, "hostname", "2234")
			buildExp := buildBase
			buildExp.Id = 2234
			created := timestamppb.New(RefTime.Add(2 * time.Hour))
			started := timestamppb.New(RefTime.Add(3 * time.Hour))
			updated := timestamppb.New(RefTime.Add(6 * time.Hour))
			completed := timestamppb.New(RefTime.Add(6 * time.Hour))

			mbc.EXPECT().GetBuild(gomock.Any(), gomock.Any()).Return(&buildbucketpb.Build{
				Id:         2234,
				Status:     buildbucketpb.Status_SUCCESS,
				CreateTime: created,
				StartTime:  started,
				EndTime:    completed,
				UpdateTime: updated,
				Builder: &buildbucketpb.BuilderID{
					Project: "fake",
					Bucket:  "bucket",
					Builder: "fake_builder",
				},
				Input: &buildbucketpb.Build_Input{
					GitilesCommit: &buildbucketpb.GitilesCommit{
						Host:    "chromium.googlesource.com",
						Id:      "8930f18245df678abc944376372c77ba5e2a658b",
						Project: "angle/angle",
					},
				},
			}, nil).AnyTimes()

			h := httptest.NewRecorder()
			r := &http.Request{Body: makeReq(buildExp)}
			PubSubHandler(&router.Context{
				Context: c,
				Writer:  h,
				Request: r,
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
				eBuild := bbv1.LegacyApiCommonBuildMessage{
					Id:        2234,
					Project:   "fake",
					Bucket:    "luci.fake.bucket",
					Tags:      []string{"builder:fake_builder"},
					CreatedBy: string(identity.AnonymousIdentity),
					CreatedTs: bbv1.FormatTimestamp(RefTime.Add(2 * time.Hour)),
					StartedTs: bbv1.FormatTimestamp(RefTime.Add(3 * time.Hour)),
					UpdatedTs: bbv1.FormatTimestamp(RefTime.Add(4 * time.Hour)),
					Status:    "STARTED",
				}

				h := httptest.NewRecorder()
				r := &http.Request{Body: makeReq(eBuild)}
				PubSubHandler(&router.Context{
					Context: c,
					Writer:  h,
					Request: r,
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
	// Skip due to https://crbug.com/1304383
	SkipConvey("TestShouldUpdateBuilderSummary", t, func() {
		ctx := context.Background()

		// Set up a test redis server.
		s, err := miniredis.Run()
		So(err, ShouldBeNil)
		defer s.Close()
		ctx = redisconn.UsePool(ctx, &redis.Pool{
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
			start := time.Now()
			shouldUpdate, err := shouldUpdateBuilderSummary(ctx, createBuildSummary("test-builder-id-1", buildbucketpb.Status_SUCCESS, start))
			So(err, ShouldBeNil)
			So(shouldUpdate, ShouldBeTrue)

			// If there's no recent updates, the call should return immediately.
			// So builds from low traffic builders can be processed immediately.
			So(time.Since(start), ShouldBeLessThan, 20*time.Millisecond)
		})

		Convey("Single call followed by multiple parallel calls", func() {
			pivot := time.Now()

			shouldUpdates := make([]bool, 4)

			shouldUpdate, err := shouldUpdateBuilderSummary(ctx, createBuildSummary("test-builder-id-2", buildbucketpb.Status_SUCCESS, pivot))
			So(err, ShouldBeNil)
			shouldUpdates[0] = shouldUpdate

			err = parallel.FanOutIn(func(tasks chan<- func() error) {
				tasks <- func() error {
					createdAt := pivot.Add(5 * time.Millisecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(ctx, createBuildSummary("test-builder-id-2", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[1] = shouldUpdate
					return err
				}

				tasks <- func() error {
					time.Sleep(5 * time.Millisecond)
					createdAt := pivot.Add(15 * time.Millisecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(ctx, createBuildSummary("test-builder-id-2", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[2] = shouldUpdate
					return err
				}

				tasks <- func() error {
					time.Sleep(10 * time.Millisecond)
					createdAt := pivot.Add(10 * time.Millisecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(ctx, createBuildSummary("test-builder-id-2", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[3] = shouldUpdate
					return err
				}
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

		Convey("Single call followed by multiple parallel calls that are nanoseconds apart", func() {
			// This test ensures that the timestamp percision is not lost.

			pivot := time.Now()

			shouldUpdates := make([]bool, 4)

			shouldUpdate, err := shouldUpdateBuilderSummary(ctx, createBuildSummary("test-builder-id-3", buildbucketpb.Status_SUCCESS, pivot))
			So(err, ShouldBeNil)
			shouldUpdates[0] = shouldUpdate

			err = parallel.FanOutIn(func(tasks chan<- func() error) {
				tasks <- func() error {
					createdAt := pivot.Add(time.Nanosecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(ctx, createBuildSummary("test-builder-id-3", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[1] = shouldUpdate
					return err
				}

				tasks <- func() error {
					time.Sleep(5 * time.Millisecond)
					createdAt := pivot.Add(3 * time.Nanosecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(ctx, createBuildSummary("test-builder-id-3", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[2] = shouldUpdate
					return err
				}

				tasks <- func() error {
					time.Sleep(10 * time.Millisecond)
					createdAt := pivot.Add(2 * time.Nanosecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(ctx, createBuildSummary("test-builder-id-3", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[3] = shouldUpdate
					return err
				}
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

		Convey("Single call followed by multiple parallel calls in different time buckets", func() {
			pivot := time.Now()

			shouldUpdates := make([]bool, 4)

			shouldUpdate, err := shouldUpdateBuilderSummary(ctx, createBuildSummary("test-builder-id-4", buildbucketpb.Status_SUCCESS, pivot))
			So(err, ShouldBeNil)
			shouldUpdates[0] = shouldUpdate

			time.Sleep(time.Duration(entityUpdateIntervalInS * int64(time.Second)))
			err = parallel.FanOutIn(func(tasks chan<- func() error) {
				tasks <- func() error {
					createdAt := pivot.Add(5 * time.Millisecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(ctx, createBuildSummary("test-builder-id-4", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[1] = shouldUpdate
					return err
				}

				tasks <- func() error {
					time.Sleep(5 * time.Millisecond)
					createdAt := pivot.Add(15 * time.Millisecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(ctx, createBuildSummary("test-builder-id-4", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[2] = shouldUpdate
					return err
				}

				tasks <- func() error {
					time.Sleep(10 * time.Millisecond)
					createdAt := pivot.Add(20 * time.Millisecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(ctx, createBuildSummary("test-builder-id-4", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[3] = shouldUpdate
					return err
				}
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
