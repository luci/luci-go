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

package bblistener

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsub/pstest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	buildbucketpb "go.chromium.org/luci/buildbucket/proto"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/tryjob"
)

func TestExtractTopicProject(t *testing.T) {
	t.Parallel()
	t.Run("bad", func(t *testing.T) {
		_, err := extractTopicProject("bad-topic")
		assert.ErrIsLike(t, err, `topic bad-topic doesn't match "^projects/(.*)/topics/(.*)$"`)
	})

	t.Run("bad", func(t *testing.T) {
		prj, err := extractTopicProject("projects/my-project/topics/topic")
		assert.NoErr(t, err)
		assert.That(t, prj, should.Equal("my-project"))
	})
}

func TestListener(t *testing.T) {
	t.Parallel()
	const bbHost = "buildbucket.example.com"
	ftt.Run("Listener", t, func(t *ftt.Test) {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		srv := pstest.NewServer()
		defer srv.Close()
		conn, err := grpc.NewClient(srv.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		assert.NoErr(t, err)
		defer conn.Close()
		client, err := pubsub.NewClient(ctx, "testProj", option.WithGRPCConn(conn))
		assert.NoErr(t, err)
		defer client.Close()
		topic, err := client.CreateTopic(ctx, "build-update")
		assert.NoErr(t, err)
		subConfig := pubsub.SubscriptionConfig{
			Topic: topic,
		}
		sub, err := client.CreateSubscription(ctx, SubscriptionID, subConfig)
		assert.NoErr(t, err)
		tProj, err := extractTopicProject(subConfig.Topic.String())
		assert.NoErr(t, err)
		tjNotifier := &testTryjobNotifier{}
		tjUpdater := &testTryjobUpdater{}
		l := &listener{
			bbHost:       fmt.Sprintf("%s.appspot.com", tProj),
			subscription: sub,
			tjNotifier:   tjNotifier,
			tjUpdater:    tjUpdater,
			processedCh:  make(chan string, 10),
		}
		assert.That(t, l.bbHost, should.Equal("testProj.appspot.com"))

		cctx, cancel := context.WithCancel(ctx)
		listenerDoneCh := make(chan struct{})
		defer func() {
			cancel()
			<-listenerDoneCh
		}()
		go func() {
			defer close(listenerDoneCh)
			if err := l.start(cctx); err != nil {
				panic(errors.Reason("failed to start listener. reason: %s", err).Err())
			}
		}()

		ensureAcked := func(msgID string) {
			timer := time.After(10 * time.Second)
			for {
				select {
				case <-timer:
					t.Fatalf("took too long to ack message %s", msgID)
				default:
					acks := srv.Message(msgID).Acks
					if acks > 0 {
						return
					}
				}
			}
		}

		t.Run("Successful", func(t *ftt.Test) {
			t.Run("Relevant - large fields dropped", func(t *ftt.Test) {
				eid := tryjob.MustBuildbucketID(bbHost, 123)
				eid.MustCreateIfNotExists(ctx)
				b := &buildbucketpb.Build{
					Id: 123,
					Builder: &buildbucketpb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Infra: &buildbucketpb.BuildInfra{
						Buildbucket: &buildbucketpb.BuildInfra_Buildbucket{
							Hostname: bbHost,
						},
					},
					Status: buildbucketpb.Status_SUCCESS,
				}
				msgID := srv.Publish(topic.String(), makeBuildPubsubData(t, b, true), makeBuildsPubsubAttrs(b))
				select {
				case <-time.After(15 * time.Second):
					t.Fatal("took too long to process message")
				case processedMsgID := <-l.processedCh:
					assert.That(t, processedMsgID, should.Equal(msgID))
				}
				assert.That(t, tjNotifier.notified, should.Match([]tryjob.ExternalID{eid}))
				ensureAcked(msgID)
			})

			t.Run("Relevant", func(t *ftt.Test) {
				eid := tryjob.MustBuildbucketID(bbHost, 123)
				eid.MustCreateIfNotExists(ctx)
				b := &buildbucketpb.Build{
					Id: 123,
					Builder: &buildbucketpb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Infra: &buildbucketpb.BuildInfra{
						Buildbucket: &buildbucketpb.BuildInfra_Buildbucket{
							Hostname: bbHost,
						},
					},
					Status: buildbucketpb.Status_SUCCESS,
				}
				msgID := srv.Publish(topic.String(), makeBuildPubsubData(t, b, false), makeBuildsPubsubAttrs(b))
				select {
				case <-time.After(15 * time.Second):
					t.Fatal("took too long to process message")
				case processedMsgID := <-l.processedCh:
					assert.That(t, processedMsgID, should.Equal(msgID))
				}
				assert.That(t, tjUpdater.updated, should.Match([]tryjob.ExternalID{eid}))
				ensureAcked(msgID)
			})

			t.Run("Relevant - empty hostname in pubsub msg", func(t *ftt.Test) {
				b := &buildbucketpb.Build{
					Id: 123,
					Builder: &buildbucketpb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Status: buildbucketpb.Status_SUCCESS,
				}
				// When b.infra.buildbucket.hostname is empty, it should use the computed bbhost in listener - l.
				eid := tryjob.MustBuildbucketID(l.bbHost, 123)
				eid.MustCreateIfNotExists(ctx)

				msgID := srv.Publish(topic.String(), makeBuildPubsubData(t, b, false), makeBuildsPubsubAttrs(b))
				select {
				case <-time.After(15 * time.Second):
					t.Fatal("took too long to process message")
				case processedMsgID := <-l.processedCh:
					assert.That(t, processedMsgID, should.Equal(msgID))
				}
				assert.That(t, tjUpdater.updated, should.Match([]tryjob.ExternalID{eid}))
				ensureAcked(msgID)
			})

			t.Run("Irrelevant", func(t *ftt.Test) {
				b := &buildbucketpb.Build{
					Id: 404,
					Builder: &buildbucketpb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Infra: &buildbucketpb.BuildInfra{
						Buildbucket: &buildbucketpb.BuildInfra_Buildbucket{
							Hostname: bbHost,
						},
					},
					Status: buildbucketpb.Status_SUCCESS,
				}
				msgID := srv.Publish(topic.String(), makeBuildPubsubData(t, b, false), makeBuildsPubsubAttrs(b))
				select {
				case <-time.After(15 * time.Second):
					t.Fatal("took too long to process message")
				case processedMsgID := <-l.processedCh:
					assert.That(t, processedMsgID, should.Equal(msgID))
				}
				assert.Loosely(t, tjUpdater.updated, should.BeEmpty)
				ensureAcked(msgID)
			})
		})

		t.Run("Transient failure", func(t *ftt.Test) {
			t.Run("Schedule call fails transiently", func(t *ftt.Test) {
				eid := tryjob.MustBuildbucketID(bbHost, 1)
				eid.MustCreateIfNotExists(ctx)
				tjNotifier.response = map[tryjob.ExternalID]error{
					eid: errTransient,
				}
				b := &buildbucketpb.Build{
					Id: 1,
					Builder: &buildbucketpb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Infra: &buildbucketpb.BuildInfra{
						Buildbucket: &buildbucketpb.BuildInfra_Buildbucket{
							Hostname: bbHost,
						},
					},
					Status: buildbucketpb.Status_SUCCESS,
				}
				msgID := srv.Publish(topic.String(), makeBuildPubsubData(t, b, true), makeBuildsPubsubAttrs(b))
				timer := time.After(15 * time.Second)
				for srv.Message(msgID).Deliveries <= 1 {
					select {
					case <-timer:
						t.Fatal("took too long for pub/sub to redeliver messages")
					case processedMsgID := <-l.processedCh:
						assert.That(t, processedMsgID, should.Equal(msgID))
					}
				}
				assert.Loosely(t, tjNotifier.notified, should.BeEmpty)
				assert.That(t, srv.Message(msgID).Acks, should.Equal(0))
			})
		})

		t.Run("Permanent failure", func(t *ftt.Test) {
			t.Run("Unparseable msg", func(t *ftt.Test) {
				msgID := srv.Publish(topic.String(), []byte("Unparseable hot garbage.'}]\""), makeBuildsPubsubAttrs(&buildbucketpb.Build{}))
				select {
				case <-time.After(15 * time.Second):
					t.Fatal("took too long to process message")
				case processedMsgID := <-l.processedCh:
					assert.That(t, processedMsgID, should.Equal(msgID))
				}
				assert.Loosely(t, tjUpdater.updated, should.BeEmpty)
				assert.That(t, l.stats.permanentErrCount, should.Equal(int64(1)))
				ensureAcked(msgID)
			})

			t.Run("tjUpdater failure", func(t *ftt.Test) {
				eid := tryjob.MustBuildbucketID(bbHost, 123)
				eid.MustCreateIfNotExists(ctx)
				tjUpdater.response = map[tryjob.ExternalID]error{
					eid: errors.New("failed to update tryjob"),
				}
				b := &buildbucketpb.Build{
					Id: 123,
					Builder: &buildbucketpb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Infra: &buildbucketpb.BuildInfra{
						Buildbucket: &buildbucketpb.BuildInfra_Buildbucket{
							Hostname: bbHost,
						},
					},
					Status: buildbucketpb.Status_SUCCESS,
				}
				msgID := srv.Publish(topic.String(), makeBuildPubsubData(t, b, false), makeBuildsPubsubAttrs(b))
				select {
				case <-time.After(15 * time.Second):
					t.Fatal("took too long to process message")
				case processedMsgID := <-l.processedCh:
					assert.That(t, processedMsgID, should.Equal(msgID))
				}
				assert.Loosely(t, tjUpdater.updated, should.BeEmpty)
				assert.That(t, l.stats.permanentErrCount, should.Equal(int64(1)))
				ensureAcked(msgID)
			})

			t.Run("Schedule call fails permanently", func(t *ftt.Test) {
				eid := tryjob.MustBuildbucketID(bbHost, 1)
				eid.MustCreateIfNotExists(ctx)
				tjNotifier.response = map[tryjob.ExternalID]error{
					eid: errPermanent,
				}
				b := &buildbucketpb.Build{
					Id: 1,
					Builder: &buildbucketpb.BuilderID{
						Project: "project",
						Bucket:  "bucket",
						Builder: "builder",
					},
					Infra: &buildbucketpb.BuildInfra{
						Buildbucket: &buildbucketpb.BuildInfra_Buildbucket{
							Hostname: bbHost,
						},
					},
					Status: buildbucketpb.Status_SUCCESS,
				}
				msgID := srv.Publish(topic.String(), makeBuildPubsubData(t, b, true), makeBuildsPubsubAttrs(b))
				select {
				case <-time.After(15 * time.Second):
					t.Fatal("took too long to process message")
				case processedMsgID := <-l.processedCh:
					assert.That(t, processedMsgID, should.Equal(msgID))
				}
				assert.Loosely(t, tjNotifier.notified, should.BeEmpty)
				assert.That(t, l.stats.permanentErrCount, should.Equal(int64(1)))
				ensureAcked(msgID)
			})
		})
	})
}

func makeBuildsPubsubAttrs(b *buildbucketpb.Build) map[string]string {
	isCompleted := b.Status&buildbucketpb.Status_ENDED_MASK == buildbucketpb.Status_ENDED_MASK
	return map[string]string{
		"project":      b.Builder.GetProject(),
		"bucket":       b.Builder.GetBucket(),
		"builder":      b.Builder.GetBuilder(),
		"is_completed": strconv.FormatBool(isCompleted),
		"version":      "v2",
	}
}

func makeBuildPubsubData(t *ftt.Test, b *buildbucketpb.Build, dropLargeFields bool) []byte {
	t.Helper()
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
	compress := func(data []byte) []byte {
		buf := &bytes.Buffer{}
		zw := zlib.NewWriter(buf)
		_, err := zw.Write(data)
		assert.NoErr(t, err)
		assert.NoErr(t, zw.Close())
		return buf.Bytes()
	}

	msg := &buildbucketpb.BuildsV2PubSub{
		Build: copyB,
	}
	if dropLargeFields {
		msg.BuildLargeFieldsDropped = true
	} else {
		largeBytes, err := proto.Marshal(large)
		assert.NoErr(t, err)
		msg.BuildLargeFields = compress(largeBytes)
	}
	data, err := protojson.Marshal(msg)
	assert.NoErr(t, err)
	return data
}

type testTryjobNotifier struct {
	mu sync.Mutex

	response map[tryjob.ExternalID]error
	notified []tryjob.ExternalID
}

// Schedule mocks tryjob.Schedule, and returns an error based on the host in
// the given ExternalID.
func (ttn *testTryjobNotifier) ScheduleUpdate(ctx context.Context, id common.TryjobID, eid tryjob.ExternalID) error {
	if id == 0 {
		return errors.New("must provide internal tryjob id")
	}
	ttn.mu.Lock()
	defer ttn.mu.Unlock()
	if err, ok := ttn.response[eid]; ok {
		return err
	}
	ttn.notified = append(ttn.notified, eid)
	return nil
}

type testTryjobUpdater struct {
	mu sync.Mutex

	response map[tryjob.ExternalID]error
	updated  []tryjob.ExternalID
}

func (ttu *testTryjobUpdater) Update(ctx context.Context, eid tryjob.ExternalID, data any) error {
	host, buildID, err := eid.ParseBuildbucketID()
	if err != nil {
		return err
	}
	build, ok := data.(*buildbucketpb.Build)
	if !ok {
		return errors.Reason("not buildbucket.build proto").Err()
	}
	hostInBuild := build.GetInfra().GetBuildbucket().GetHostname()
	if hostInBuild != "" && hostInBuild != host {
		return errors.Reason("build hostname doesn't match").Err()
	}
	if build.Id != buildID {
		return errors.Reason("build id doesn't match").Err()
	}

	ttu.mu.Lock()
	defer ttu.mu.Unlock()
	if err, ok := ttu.response[eid]; ok {
		return err
	}
	ttu.updated = append(ttu.updated, eid)
	return nil
}

var (
	errTransient error = transient.Tag.Apply(errors.New("transient error"))
	errPermanent error = errors.New("permanent error")
)
