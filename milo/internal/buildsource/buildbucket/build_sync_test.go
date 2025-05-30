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
	"compress/zlib"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang/mock/gomock"
	"github.com/gomodule/redigo/redis"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/appengine/gaetesting"
	"go.chromium.org/luci/auth/identity"
	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/sync/parallel"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/gae/impl/memory"
	"go.chromium.org/luci/gae/service/datastore"
	"go.chromium.org/luci/server/auth"
	"go.chromium.org/luci/server/auth/authtest"
	"go.chromium.org/luci/server/caching"
	"go.chromium.org/luci/server/pubsub"
	"go.chromium.org/luci/server/redisconn"

	"go.chromium.org/luci/milo/internal/model"
	"go.chromium.org/luci/milo/internal/model/milostatus"
	"go.chromium.org/luci/milo/internal/projectconfig"
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

func TestPubSub(t *testing.T) {
	t.Parallel()

	ftt.Run(`TestPubSub`, t, func(t *ftt.Test) {
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
		assert.Loosely(t, err, should.BeNil)

		// Initialize the appropriate project config.
		err = datastore.Put(c, &projectconfig.Project{
			ID: "fake",
		})
		assert.Loosely(t, err, should.BeNil)

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

		t.Run("New in-process build", func(t *ftt.Test) {
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
			outProperties, _ := structpb.NewStruct(propertiesMap)

			verifyDatastore := func() {
				datastore.GetTestable(c).CatchupIndexes()
				buildAct := model.BuildSummary{BuildKey: bKey}
				err = datastore.Get(c, &buildAct)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, buildAct.BuildKey.String(), should.Equal(bKey.String()))
				assert.Loosely(t, buildAct.BuilderID, should.Equal("buildbucket/luci.fake.bucket/fake_builder"))
				assert.Loosely(t, buildAct.Summary, should.Match(model.Summary{
					Status: milostatus.Running,
					Start:  RefTime.Add(3 * time.Hour),
				}))
				assert.Loosely(t, buildAct.Created, should.Match(RefTime.Add(2*time.Hour)))
				assert.Loosely(t, buildAct.Experimental, should.BeTrue)
				assert.Loosely(t, buildAct.BlamelistPins, should.Match([]string{
					"commit/gitiles/chromium.googlesource.com/angle/angle/+/8930f18245df678abc944376372c77ba5e2a658b",
					"commit/gitiles/chromium.googlesource.com/chromium/src/+/07033c702f81a75dfc2d83888ba3f8b354d0e920",
				}))

				blder := model.BuilderSummary{BuilderID: "buildbucket/luci.fake.bucket/fake_builder"}
				err = datastore.Get(c, &blder)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, blder.LastFinishedStatus, should.Match(milostatus.NotRun))
				assert.Loosely(t, blder.LastFinishedBuildID, should.BeEmpty)
			}

			t.Run("stores BuildSummary and BuilderSummary", func(t *ftt.Test) {
				buildExp.Output.Properties = outProperties
				pubsubMsg, err := makeBuildsV2PpubsubMsg(buildExp)
				assert.Loosely(t, err, should.BeNil)

				err = pubSubHandlerImpl(c, pubsub.Message{}, pubsubMsg)
				assert.Loosely(t, err, should.ErrLike(nil))
				verifyDatastore()
			})

			t.Run("need additional GetBuild call", func(t *ftt.Test) {
				pubsubMsg, err := makeBuildsV2PpubsubMsg(buildExp)
				assert.Loosely(t, err, should.BeNil)

				pubsubMsg.BuildLargeFields = nil
				pubsubMsg.BuildLargeFieldsDropped = true
				// Set up buildbucket client.
				c, ctrl, mbc := newMockClient(c, t.T)
				defer ctrl.Finish()
				mbc.EXPECT().GetBuild(gomock.Any(), gomock.Any()).Return(&buildbucketpb.Build{
					Output: &buildbucketpb.Build_Output{
						Properties: outProperties,
					},
				}, nil).AnyTimes()

				err = pubSubHandlerImpl(c, pubsub.Message{}, pubsubMsg)
				assert.Loosely(t, err, should.ErrLike(nil))
				datastore.GetTestable(c).CatchupIndexes()
				verifyDatastore()
			})
		})

		t.Run("Completed build", func(t *ftt.Test) {
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
			pubsubMsg, err := makeBuildsV2PpubsubMsg(buildExp)
			assert.Loosely(t, err, should.BeNil)

			err = pubSubHandlerImpl(c, pubsub.Message{}, pubsubMsg)
			assert.Loosely(t, err, should.ErrLike(nil))
			t.Run("stores BuildSummary and BuilderSummary", func(t *ftt.Test) {
				buildAct := model.BuildSummary{BuildKey: bKey}
				err = datastore.Get(c, &buildAct)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, buildAct.BuildKey.String(), should.Equal(bKey.String()))
				assert.Loosely(t, buildAct.BuilderID, should.Equal("buildbucket/luci.fake.bucket/fake_builder"))
				assert.Loosely(t, buildAct.Summary, should.Match(model.Summary{
					Status: milostatus.Success,
					Start:  RefTime.Add(3 * time.Hour),
					End:    RefTime.Add(6 * time.Hour),
				}))
				assert.Loosely(t, buildAct.Created, should.Match(RefTime.Add(2*time.Hour)))

				blder := model.BuilderSummary{BuilderID: "buildbucket/luci.fake.bucket/fake_builder"}
				err = datastore.Get(c, &blder)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, blder.LastFinishedCreated, should.Match(RefTime.Add(2*time.Hour)))
				assert.Loosely(t, blder.LastFinishedStatus, should.Match(milostatus.Success))
				assert.Loosely(t, blder.LastFinishedBuildID, should.Equal("buildbucket/2234"))
				assert.Loosely(t, buildAct.BlamelistPins, should.Match([]string{
					"commit/gitiles/chromium.googlesource.com/angle/angle/+/8930f18245df678abc944376372c77ba5e2a658b",
				}))
			})

			t.Run("results in earlier update not being ingested", func(t *ftt.Test) {
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
				pubsubMsg, err := makeBuildsV2PpubsubMsg(eBuild)
				assert.Loosely(t, err, should.BeNil)

				err = pubSubHandlerImpl(c, pubsub.Message{}, pubsubMsg)
				assert.Loosely(t, err, should.ErrLike(nil))

				buildAct := model.BuildSummary{BuildKey: bKey}
				err = datastore.Get(c, &buildAct)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, buildAct.Summary, should.Match(model.Summary{
					Status: milostatus.Success,
					Start:  RefTime.Add(3 * time.Hour),
					End:    RefTime.Add(6 * time.Hour),
				}))
				assert.Loosely(t, buildAct.Created, should.Match(RefTime.Add(2*time.Hour)))

				blder := model.BuilderSummary{BuilderID: "buildbucket/luci.fake.bucket/fake_builder"}
				err = datastore.Get(c, &blder)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, blder.LastFinishedCreated, should.Match(RefTime.Add(2*time.Hour)))
				assert.Loosely(t, blder.LastFinishedStatus, should.Match(milostatus.Success))
				assert.Loosely(t, blder.LastFinishedBuildID, should.Equal("buildbucket/2234"))
			})
		})
	})
}

func makeBuildsV2PpubsubMsg(b *buildbucketpb.Build) (*buildbucketpb.BuildsV2PubSub, error) {
	copyB := proto.Clone(b).(*buildbucketpb.Build)
	large := &buildbucketpb.Build{
		Input: &buildbucketpb.Build_Input{
			Properties: copyB.GetInput().GetProperties(),
		},
		Output: &buildbucketpb.Build_Output{
			Properties: copyB.GetOutput().GetProperties(),
		},
		Steps: copyB.GetSteps(),
	}
	if copyB.Input != nil {
		copyB.Input.Properties = nil
	}
	if copyB.Output != nil {
		copyB.Output.Properties = nil
	}
	copyB.Steps = nil
	compress := func(data []byte) ([]byte, error) {
		buf := &bytes.Buffer{}
		zw := zlib.NewWriter(buf)
		if _, err := zw.Write(data); err != nil {
			return nil, errors.Fmt("failed to compress: %w", err)
		}
		if err := zw.Close(); err != nil {
			return nil, errors.Fmt("error closing zlib writer: %w", err)
		}
		return buf.Bytes(), nil
	}
	largeBytes, err := proto.Marshal(large)
	if err != nil {
		return nil, errors.Fmt("failed to marshal large: %w", err)
	}
	compressedLarge, err := compress(largeBytes)
	if err != nil {
		return nil, err
	}
	return &buildbucketpb.BuildsV2PubSub{
		Build:            copyB,
		BuildLargeFields: compressedLarge,
	}, nil
}

func TestShouldUpdateBuilderSummary(t *testing.T) {
	ftt.Run("TestShouldUpdateBuilderSummary", t, func(t *ftt.Test) {
		c := context.Background()
		startTime := time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC).
			Truncate(time.Duration(entityUpdateIntervalInS) * time.Second)

		// Set up a test redis server.
		s, err := miniredis.Run()
		assert.Loosely(t, err, should.BeNil)
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

		t.Run("Single call", func(t *ftt.Test) {
			// Ensures `shouldUpdateBuilderSummary` is called at the start of the time bucket.
			c, _ := testclock.UseTime(c, startTime)

			start := clock.Now(c)
			// Should return without advancing the clock.
			shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-1", buildbucketpb.Status_SUCCESS, start))
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, shouldUpdate, should.BeTrue)
		})

		t.Run("Single call followed by multiple parallel calls", func(tc *ftt.Test) {
			// Ensures all `shouldUpdateBuilderSummary` calls are in the same time bucket.
			c, tClock := testclock.UseTime(c, startTime)

			pivot := clock.Now(c).Add(-time.Hour)

			shouldUpdates := make([]bool, 4)

			shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-2", buildbucketpb.Status_SUCCESS, pivot))
			assert.Loosely(tc, err, should.BeNil)
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
				assert.Loosely(tc, <-eventC, should.Equal("timer"))
				tasks <- func() error {
					createdAt := pivot.Add(15 * time.Millisecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-2", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[2] = shouldUpdate
					return err
				}

				// Wait until the previous call reaches a blocking point.
				assert.Loosely(tc, <-eventC, should.Equal("timer"))
				tasks <- func() error {
					createdAt := pivot.Add(10 * time.Millisecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-2", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[3] = shouldUpdate
					eventC <- "return"
					return err
				}

				// Wait until the last shouldUpdateBuilderSummary call returns then
				// advance the clock to the next time bucket.
				assert.Loosely(tc, <-eventC, should.Equal("return"))
				tClock.Add(time.Duration(entityUpdateIntervalInS) * time.Second)
			})
			assert.Loosely(tc, err, should.BeNil)

			// The first value should be true because there's no recent updates.
			assert.Loosely(tc, shouldUpdates[0], should.BeTrue)

			// The second value should be false because there's a recent update, so it
			// moves to the next time bucket, wait for the timebucket to begin.
			// Then it's replaced by the next shouldUpdateBuilderSummary call.
			assert.Loosely(tc, shouldUpdates[1], should.BeFalse)

			// The third value should be true because it has a newer build than the
			// current one. And it's not replaced by any new builds.
			assert.Loosely(tc, shouldUpdates[2], should.BeTrue)

			// The forth value should be false because the build is not created earlier
			// than the build associated with the current pending update in it's pending bucket.
			assert.Loosely(tc, shouldUpdates[3], should.BeFalse)
		})

		t.Run("Single call followed by multiple parallel calls that are nanoseconds apart", func(tc *ftt.Test) {
			// This test ensures that the timestamp percision is not lost.

			// Ensures all `shouldUpdateBuilderSummary` calls are in the same time bucket.
			c, tClock := testclock.UseTime(c, startTime)

			pivot := clock.Now(c).Add(-time.Hour)

			shouldUpdates := make([]bool, 4)

			shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-3", buildbucketpb.Status_SUCCESS, pivot))
			assert.Loosely(t, err, should.BeNil)
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
				assert.Loosely(t, <-eventC, should.Equal("timer"))
				tasks <- func() error {
					createdAt := pivot.Add(3 * time.Nanosecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-3", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[2] = shouldUpdate
					return err
				}

				// Wait until the previous call reaches a blocking point.
				assert.Loosely(t, <-eventC, should.Equal("timer"))
				tasks <- func() error {
					createdAt := pivot.Add(2 * time.Nanosecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-3", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[3] = shouldUpdate
					eventC <- "return"
					return err
				}

				// Wait until the last shouldUpdateBuilderSummary call returns then
				// advance the clock to the next time bucket.
				assert.Loosely(t, <-eventC, should.Equal("return"))
				tClock.Add(time.Duration(entityUpdateIntervalInS) * time.Second)
			})
			assert.Loosely(t, err, should.BeNil)

			// The first value should be true because there's no pending/recent updates.
			assert.Loosely(t, shouldUpdates[0], should.BeTrue)

			// The second value should be false because there's a recent update, so it
			// moves to the next time bucket, wait for the timebucket to begin.
			// Then it's replaced by the next shouldUpdateBuilderSummary call.
			assert.Loosely(t, shouldUpdates[1], should.BeFalse)

			// The third value should be true because it has a newer build than the
			// current pending one. And it's not replaced by any new builds.
			assert.Loosely(t, shouldUpdates[2], should.BeTrue)

			// The forth value should be false because the build is not created earlier
			// than the build associated with the current pending update in it's pending bucket.
			assert.Loosely(t, shouldUpdates[3], should.BeFalse)
		})

		t.Run("Single call followed by multiple parallel calls in different time buckets", func(t *ftt.Test) {
			c, tClock := testclock.UseTime(c, startTime)

			pivot := clock.Now(c).Add(-time.Hour)
			shouldUpdates := make([]bool, 4)

			shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-4", buildbucketpb.Status_SUCCESS, pivot))
			assert.Loosely(t, err, should.BeNil)
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
				assert.Loosely(t, <-eventC, should.Equal("return"))
				tasks <- func() error {
					createdAt := pivot.Add(15 * time.Millisecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-4", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[2] = shouldUpdate
					return err
				}

				// Wait until the previous call reaches a blocking point.
				assert.Loosely(t, <-eventC, should.Equal("timer"))
				tasks <- func() error {
					createdAt := pivot.Add(20 * time.Millisecond)
					shouldUpdate, err := shouldUpdateBuilderSummary(c, createBuildSummary("test-builder-id-4", buildbucketpb.Status_SUCCESS, createdAt))
					shouldUpdates[3] = shouldUpdate
					return err
				}

				// Wait until the last shouldUpdateBuilderSummary call returns then
				// advance the clock to the next time bucket.
				assert.Loosely(t, <-eventC, should.Equal("timer"))
				tClock.Add(time.Duration(entityUpdateIntervalInS) * time.Second)
			})
			assert.Loosely(t, err, should.BeNil)

			// The first value should be true because there's no pending/recent updates.
			assert.Loosely(t, shouldUpdates[0], should.BeTrue)

			// The second value should be true because it has move to a new time bucket
			// and there's no pending/recent updates in that bucket.
			assert.Loosely(t, shouldUpdates[1], should.BeTrue)

			// The third value should be false because there's a recent update, so it
			// moves to the next time bucket, wait for the timebucket to begin.
			// Then it's replaced by the next shouldUpdateBuilderSummary call.
			assert.Loosely(t, shouldUpdates[2], should.BeFalse)

			// The forth value should be true because it has a newer build than the
			// current pending one. And it's not replaced by any new builds.
			assert.Loosely(t, shouldUpdates[3], should.BeTrue)
		})
	})
}

func TestDeleteOldBuilds(t *testing.T) {
	t.Parallel()

	ftt.Run("DeleteOldBuilds", t, func(t *ftt.Test) {
		now := time.Date(2020, 01, 01, 0, 0, 0, 0, time.UTC)

		ctx, _ := testclock.UseTime(memory.Use(context.Background()), now)
		datastore.GetTestable(ctx).AutoIndex(true)
		datastore.GetTestable(ctx).Consistent(true)

		createBuild := func(id string, tm time.Time) *model.BuildSummary {
			b := &model.BuildSummary{
				BuildKey: model.MakeBuildKey(ctx, "host", id),
				Created:  tm,
			}
			assert.Loosely(t, datastore.Put(ctx, b), should.BeNil)
			return b
		}

		t.Run("keeps builds", func(t *ftt.Test) {
			t.Run("as old as BuildSummaryStorageDuration", func(t *ftt.Test) {
				build := createBuild("1", now.Add(-BuildSummaryStorageDuration))
				assert.Loosely(t, DeleteOldBuilds(ctx), should.BeNil)
				assert.Loosely(t, datastore.Get(ctx, build), should.BeNil)
			})
			t.Run("younger than BuildSummaryStorageDuration", func(t *ftt.Test) {
				build := createBuild("2", now.Add(-BuildSummaryStorageDuration+time.Minute))
				assert.Loosely(t, DeleteOldBuilds(ctx), should.BeNil)
				assert.Loosely(t, datastore.Get(ctx, build), should.BeNil)
			})
		})

		t.Run("deletes builds older than BuildSummaryStorageDuration", func(t *ftt.Test) {
			build := createBuild("3", now.Add(-BuildSummaryStorageDuration-time.Minute))
			assert.Loosely(t, DeleteOldBuilds(ctx), should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, build), should.Equal(datastore.ErrNoSuchEntity))
		})

		t.Run("removes many builds", func(t *ftt.Test) {
			bs := make([]*model.BuildSummary, 234)
			old := now.Add(-BuildSummaryStorageDuration - time.Minute)
			for i := range bs {
				bs[i] = createBuild(fmt.Sprintf("4-%d", i), old)
			}
			assert.Loosely(t, DeleteOldBuilds(ctx), should.BeNil)
			assert.Loosely(t, datastore.Get(ctx, bs), should.ErrLike(
				"err[19]: datastore: no such entity\nerr[20:234] <omitted>"))
		})
	})
}

func TestSyncBuilds(t *testing.T) {
	t.Parallel()

	ftt.Run("SyncBuilds", t, func(t *ftt.Test) {
		now := time.Date(2020, 01, 01, 0, 0, 0, 0, time.UTC)

		c, _ := testclock.UseTime(memory.Use(context.Background()), now)
		datastore.GetTestable(c).AutoIndex(true)
		datastore.GetTestable(c).Consistent(true)

		createBuild := func(id string, tm time.Time, status milostatus.Status) *model.BuildSummary {
			b := &model.BuildSummary{
				BuildKey:  model.MakeBuildKey(c, "host", id),
				BuilderID: "buildbucket/luci.proj.bucket/builder",
				BuildID:   "buildbucket/" + id,
				Created:   tm,
				Summary: model.Summary{
					Status: status,
				},
				Version: tm.UnixNano(),
			}
			assert.Loosely(t, datastore.Put(c, b), should.BeNil)
			return b
		}

		t.Run("don't update builds", func(t *ftt.Test) {
			t.Run("as old as BuildSummaryStorageDuration", func(t *ftt.Test) {
				build := createBuild("luci.proj.bucket/builder/1234", now.Add(-BuildSummarySyncThreshold), milostatus.Running)
				assert.Loosely(t, syncBuildsImpl(c), should.BeNil)
				assert.Loosely(t, datastore.Get(c, build), should.BeNil)
				assert.Loosely(t, build.Summary.Status, should.Equal(milostatus.Running))
			})

			t.Run("younger than BuildSummaryStorageDuration", func(t *ftt.Test) {
				build := createBuild("luci.proj.bucket/builder/1234", now.Add(-BuildSummarySyncThreshold+time.Minute), milostatus.NotRun)
				assert.Loosely(t, syncBuildsImpl(c), should.BeNil)
				assert.Loosely(t, datastore.Get(c, build), should.BeNil)
				assert.Loosely(t, build.Summary.Status, should.Equal(milostatus.NotRun))
			})
		})

		t.Run("update builds older than BuildSummarySyncThreshold", func(t *ftt.Test) {
			build := createBuild("luci.proj.bucket/builder/1234", now.Add(-BuildSummarySyncThreshold-time.Minute), milostatus.NotRun)

			c, ctrl, mbc := newMockClient(c, t.T)
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

			assert.Loosely(t, syncBuildsImpl(c), should.BeNil)
			assert.Loosely(t, datastore.Get(c, build), should.BeNil)
			assert.Loosely(t, build.Summary.Status, should.Equal(milostatus.Success))
		})

		t.Run("ensure BuildKey stays the same", func(t *ftt.Test) {
			build := createBuild("123456", now.Add(-BuildSummarySyncThreshold-time.Minute), milostatus.NotRun)

			c, ctrl, mbc := newMockClient(c, t.T)
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

			assert.Loosely(t, syncBuildsImpl(c), should.BeNil)
			assert.Loosely(t, datastore.Get(c, build), should.BeNil)
			assert.Loosely(t, build.Summary.Status, should.Equal(milostatus.Success))

			buildWithNewKey := &model.BuildSummary{
				BuildKey: model.MakeBuildKey(c, "host", "luci.proj.bucket/builder/1234"),
			}
			assert.Loosely(t, datastore.Get(c, buildWithNewKey), should.Equal(datastore.ErrNoSuchEntity))
		})
	})
}
