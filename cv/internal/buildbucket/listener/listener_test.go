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
	"encoding/json"
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
	. "go.chromium.org/luci/common/testing/assertions"

	"go.chromium.org/luci/cv/internal/buildbucket"
	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseData(t *testing.T) {
	t.Parallel()
	Convey("parseV1Data", t, func() {
		Convey("handles valid expected input", func() {
			json := `{"hostname": "buildbucket.example.com", ` +
				`"build": {"id": "123456789", "other": "ignored"}}`
			extracted, err := parseV1Data(context.Background(), []byte(json))
			So(err, ShouldBeNil)
			So(extracted, ShouldResemble, buildbucket.PubsubMessage{
				Hostname: "buildbucket.example.com",
				Build:    buildbucket.PubsubBuildMessage{ID: 123456789},
			})
		})
		Convey("with no build ID gives error", func() {
			json := `{"hostname": "buildbucket.example.com", "build": {"other": "ignored"}}`
			data, err := parseV1Data(context.Background(), []byte(json))
			So(err, ShouldErrLike, "missing build details")
			So(data, ShouldResemble, buildbucket.PubsubMessage{})
		})
		Convey("with no build details gives error", func() {
			json := `{"hostname": "buildbucket.example.com"}`
			data, err := parseV1Data(context.Background(), []byte(json))
			So(err, ShouldErrLike, "missing build details")
			So(data, ShouldResemble, buildbucket.PubsubMessage{})
		})
	})
}

func TestExtractTopicProject(t *testing.T) {
	t.Parallel()
	Convey("bad", t, func() {
		_, err := extractTopicProject("bad-topic")
		So(err, ShouldErrLike, `topic bad-topic doesn't match "^projects/(.*)/topics/(.*)$"`)
	})

	Convey("success", t, func() {
		prj, err := extractTopicProject("projects/my-project/topics/topic")
		So(err, ShouldBeNil)
		So(prj, ShouldEqual, "my-project")
	})
}

func TestListener(t *testing.T) {
	t.Parallel()
	const bbHost = "buildbucket.example.com"
	Convey("Listener", t, func() {
		ct := cvtesting.Test{}
		ctx := ct.SetUp(t)

		srv := pstest.NewServer()
		defer srv.Close()
		conn, err := grpc.Dial(srv.Addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
		So(err, ShouldBeNil)
		defer conn.Close()
		client, err := pubsub.NewClient(ctx, "testProj", option.WithGRPCConn(conn))
		So(err, ShouldBeNil)
		defer client.Close()
		topic, err := client.CreateTopic(ctx, "build-update")
		So(err, ShouldBeNil)
		subConfig := pubsub.SubscriptionConfig{
			Topic: topic,
		}
		sub, err := client.CreateSubscription(ctx, SubscriptionID, subConfig)
		So(err, ShouldBeNil)
		tProj, err := extractTopicProject(subConfig.Topic.String())
		So(err, ShouldBeNil)
		tjNotifier := &testTryjobNotifier{}
		tjUpdater := &testTryjobUpdater{}
		l := &listener{
			bbHost:       fmt.Sprintf("%s.appspot.com", tProj),
			subscription: sub,
			tjNotifier:   tjNotifier,
			tjUpdater:    tjUpdater,
			processedCh:  make(chan string, 10),
		}
		So(l.bbHost, ShouldEqual, "testProj.appspot.com")

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
					panic(errors.Reason("took too long to ack message %s", msgID))
				default:
					acks := srv.Message(msgID).Acks
					if acks > 0 {
						return
					}
				}
			}
		}

		Convey("Successful", func() {
			Convey("Relevant", func() {
				eid := tryjob.MustBuildbucketID(bbHost, 1)
				eid.MustCreateIfNotExists(ctx)
				msgID := srv.Publish(topic.String(), toPubsubMessageData(eid), nil)
				select {
				case <-time.After(15 * time.Second):
					panic("took too long to process message")
				case processedMsgID := <-l.processedCh:
					So(processedMsgID, ShouldEqual, msgID)
				}
				So(tjNotifier.notified, ShouldResemble, []tryjob.ExternalID{eid})
				ensureAcked(msgID)
			})

			Convey("Relevant v2", func() {
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
				msgID := srv.Publish(topic.String(), makeBuildsV2PubsubData(b), makeBuildsV2PubsubAttrs(b))
				select {
				case <-time.After(15 * time.Second):
					panic("took too long to process message")
				case processedMsgID := <-l.processedCh:
					So(processedMsgID, ShouldEqual, msgID)
				}
				So(tjUpdater.updated, ShouldResemble, []tryjob.ExternalID{eid})
				ensureAcked(msgID)
			})

			Convey("Relevant v2 - empty hostname in pubsub msg", func() {
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

				msgID := srv.Publish(topic.String(), makeBuildsV2PubsubData(b), makeBuildsV2PubsubAttrs(b))
				select {
				case <-time.After(15 * time.Second):
					panic("took too long to process message")
				case processedMsgID := <-l.processedCh:
					So(processedMsgID, ShouldEqual, msgID)
				}
				So(tjUpdater.updated, ShouldResemble, []tryjob.ExternalID{eid})
				ensureAcked(msgID)
			})

			Convey("Irrelevant", func() {
				eid := tryjob.MustBuildbucketID(bbHost, 404)
				msgID := srv.Publish(topic.String(), toPubsubMessageData(eid), nil)
				select {
				case <-time.After(15 * time.Second):
					panic("took too long to process message")
				case processedMsgID := <-l.processedCh:
					So(processedMsgID, ShouldEqual, msgID)
				}
				So(tjNotifier.notified, ShouldBeEmpty)
				ensureAcked(msgID)
			})

			Convey("Irrelevant v2", func() {
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
				msgID := srv.Publish(topic.String(), makeBuildsV2PubsubData(b), makeBuildsV2PubsubAttrs(b))
				select {
				case <-time.After(15 * time.Second):
					panic("took too long to process message")
				case processedMsgID := <-l.processedCh:
					So(processedMsgID, ShouldEqual, msgID)
				}
				So(tjUpdater.updated, ShouldBeEmpty)
				ensureAcked(msgID)
			})
		})

		Convey("Transient failure", func() {
			Convey("Schedule call fails transiently", func() {
				eid := tryjob.MustBuildbucketID(bbHost, 1)
				eid.MustCreateIfNotExists(ctx)
				tjNotifier.response = map[tryjob.ExternalID]error{
					eid: errTransient,
				}
				msgID := srv.Publish(topic.String(), toPubsubMessageData(eid), nil)
				timer := time.After(15 * time.Second)
				for srv.Message(msgID).Deliveries <= 1 {
					select {
					case <-timer:
						panic(errors.New("took too long for pub/sub to redeliver messages"))
					case processedMsgID := <-l.processedCh:
						So(processedMsgID, ShouldEqual, msgID)
					}
				}
				So(tjNotifier.notified, ShouldBeEmpty)
				So(srv.Message(msgID).Acks, ShouldEqual, 0)
			})
		})

		Convey("Permanent failure", func() {
			Convey("Unparseable v1 msg", func() {
				msgID := srv.Publish(topic.String(), []byte("Unparseable hot garbage.'}]\""), nil)
				select {
				case <-time.After(15 * time.Second):
					panic("took too long to process message")
				case processedMsgID := <-l.processedCh:
					So(processedMsgID, ShouldEqual, msgID)
				}
				So(tjNotifier.notified, ShouldBeEmpty)
				So(l.stats.permanentErrCount, ShouldEqual, 1)
				ensureAcked(msgID)
			})

			Convey("Unparseable v2 msg", func() {
				msgID := srv.Publish(topic.String(), []byte("Unparseable hot garbage.'}]\""), makeBuildsV2PubsubAttrs(&buildbucketpb.Build{}))
				select {
				case <-time.After(15 * time.Second):
					panic("took too long to process message")
				case processedMsgID := <-l.processedCh:
					So(processedMsgID, ShouldEqual, msgID)
				}
				So(tjUpdater.updated, ShouldBeEmpty)
				So(l.stats.permanentErrCount, ShouldEqual, 1)
				ensureAcked(msgID)
			})

			Convey("tjUpdater failure", func() {
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
				msgID := srv.Publish(topic.String(), makeBuildsV2PubsubData(b), makeBuildsV2PubsubAttrs(b))
				select {
				case <-time.After(15 * time.Second):
					panic("took too long to process message")
				case processedMsgID := <-l.processedCh:
					So(processedMsgID, ShouldEqual, msgID)
				}
				So(tjUpdater.updated, ShouldBeEmpty)
				So(l.stats.permanentErrCount, ShouldEqual, 1)
				ensureAcked(msgID)
			})

			Convey("Schedule call fails permanently", func() {
				eid := tryjob.MustBuildbucketID(bbHost, 1)
				eid.MustCreateIfNotExists(ctx)
				tjNotifier.response = map[tryjob.ExternalID]error{
					eid: errPermanent,
				}
				msgID := srv.Publish(topic.String(), toPubsubMessageData(eid), nil)
				select {
				case <-time.After(15 * time.Second):
					panic("took too long to process message")
				case processedMsgID := <-l.processedCh:
					So(processedMsgID, ShouldEqual, msgID)
				}
				So(tjNotifier.notified, ShouldBeEmpty)
				So(l.stats.permanentErrCount, ShouldEqual, 1)
				ensureAcked(msgID)
			})
		})

	})
}

// toPubsubMessageData constructs sample message JSON for tests.
func toPubsubMessageData(eid tryjob.ExternalID) []byte {
	host, id, err := eid.ParseBuildbucketID()
	if err != nil {
		panic(err)
	}
	json, err := json.Marshal(buildbucket.PubsubMessage{
		Build:    buildbucket.PubsubBuildMessage{ID: id},
		Hostname: host,
	})
	if err != nil {
		panic(err)
	}
	return json
}

func makeBuildsV2PubsubAttrs(b *buildbucketpb.Build) map[string]string {
	isCompleted := b.Status&buildbucketpb.Status_ENDED_MASK == buildbucketpb.Status_ENDED_MASK
	return map[string]string{
		"project":      b.Builder.GetProject(),
		"bucket":       b.Builder.GetBucket(),
		"builder":      b.Builder.GetBuilder(),
		"is_completed": strconv.FormatBool(isCompleted),
		"version":      "v2",
	}
}

func makeBuildsV2PubsubData(b *buildbucketpb.Build) []byte {
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
		if _, err := zw.Write(data); err != nil {
			panic(errors.Annotate(err, "failed to compress"))
		}
		if err := zw.Close(); err != nil {
			panic(errors.Annotate(err, "error closing zlib writer"))
		}
		return buf.Bytes()
	}
	largeBytes, err := proto.Marshal(large)
	if err != nil {
		panic(errors.Annotate(err, "failed to marshal build large fields"))
	}
	compressedLarge := compress(largeBytes)
	data, err := protojson.Marshal(&buildbucketpb.BuildsV2PubSub{
		Build:            copyB,
		BuildLargeFields: compressedLarge,
	})
	if err != nil {
		panic(errors.Annotate(err, "failed to marshal BuildsV2PubSub message"))
	}
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
		panic(errors.New("must provide internal tryjob id"))
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
