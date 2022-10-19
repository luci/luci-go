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
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/pubsub/pstest"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/retry/transient"
	. "go.chromium.org/luci/common/testing/assertions"

	"go.chromium.org/luci/cv/internal/common"
	"go.chromium.org/luci/cv/internal/cvtesting"
	"go.chromium.org/luci/cv/internal/tryjob"

	. "github.com/smartystreets/goconvey/convey"
)

func TestParseData(t *testing.T) {
	t.Parallel()
	Convey("parseData", t, func() {
		Convey("handles valid expected input", func() {
			json := `{"hostname": "buildbucket.example.com", ` +
				`"build": {"id": "123456789", "other": "ignored"}}`
			extracted, err := parseData(context.Background(), []byte(json))
			So(err, ShouldBeNil)
			So(extracted, ShouldResemble, &notificationMessage{
				Hostname: "buildbucket.example.com",
				Build:    &buildMessage{ID: 123456789},
			})
		})
		Convey("with no build ID gives error", func() {
			json := `{"hostname": "buildbucket.example.com", "build": {"other": "ignored"}}`
			data, err := parseData(context.Background(), []byte(json))
			So(err, ShouldErrLike, "missing build details")
			So(data, ShouldBeNil)
		})
		Convey("with no build details gives error", func() {
			json := `{"hostname": "buildbucket.example.com"}`
			data, err := parseData(context.Background(), []byte(json))
			So(err, ShouldErrLike, "missing build details")
			So(data, ShouldBeNil)
		})
	})
}

func TestListener(t *testing.T) {
	t.Parallel()
	const bbHost = "buildbucket.example.com"
	Convey("Listener", t, func() {
		ct := cvtesting.Test{}
		ctx, cancel := ct.SetUp()
		defer cancel()

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
		_, err = client.CreateSubscription(ctx, SubscriptionID, pubsub.SubscriptionConfig{
			Topic: topic,
		})
		So(err, ShouldBeNil)

		tjNotifier := &testTryjobNotifier{}
		l := &listener{
			pubsubClient: client,
			tjNotifier:   tjNotifier,
			processedCh:  make(chan string, 10),
		}

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
			Convey("Unparseable", func() {
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
	json, err := json.Marshal(notificationMessage{
		Build:    &buildMessage{ID: id},
		Hostname: host,
	})
	if err != nil {
		panic(err)
	}
	return json
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

var (
	errTransient error = transient.Tag.Apply(errors.New("transient error"))
	errPermanent error = errors.New("permanent error")
)
