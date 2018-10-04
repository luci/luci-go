// Copyright 2015 The LUCI Authors.
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

package pubsub

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"io/ioutil"
	"sync"
	"testing"
	"time"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/recordio"
	gcps "go.chromium.org/luci/common/gcloud/pubsub"
	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/grpc/grpcutil"
	"go.chromium.org/luci/logdog/api/logpb"

	"cloud.google.com/go/pubsub"
	"github.com/golang/protobuf/proto"

	. "github.com/smartystreets/goconvey/convey"
	. "go.chromium.org/luci/common/testing/assertions"
)

type testTopic struct {
	sync.Mutex

	err func() error

	msgC          chan *pubsub.Message
	nextMessageID int
}

func (t *testTopic) String() string { return "test" }

func (t *testTopic) Publish(c context.Context, msg *pubsub.Message) (string, error) {
	if t.err != nil {
		if err := t.err(); err != nil {
			return "", err
		}
	}

	if t.msgC != nil {
		select {
		case t.msgC <- msg:
		case <-c.Done():
			return "", c.Err()
		}
	}
	return t.getNextMessageID(), nil
}

func (t *testTopic) getNextMessageID() string {
	t.Lock()
	defer t.Unlock()

	id := t.nextMessageID
	t.nextMessageID++
	return fmt.Sprintf("%d", id)
}

func deconstructMessage(msg *pubsub.Message) (*logpb.ButlerMetadata, *logpb.ButlerLogBundle, error) {
	fr := recordio.NewReader(bytes.NewBuffer(msg.Data), gcps.MaxPublishRequestBytes)

	// Validate header frame.
	headerBytes, err := fr.ReadFrameAll()
	if err != nil {
		return nil, nil, fmt.Errorf("test: failed to read header frame: %s", err)
	}

	header := logpb.ButlerMetadata{}
	if err := proto.Unmarshal(headerBytes, &header); err != nil {
		return nil, nil, fmt.Errorf("test: failed to unmarshal header: %s", err)
	}

	if header.Type != logpb.ButlerMetadata_ButlerLogBundle {
		return nil, nil, fmt.Errorf("test: unknown frame data type: %v", header.Type)
	}

	// Validate data frame.
	data, err := fr.ReadFrameAll()
	if err != nil {
		return nil, nil, fmt.Errorf("test: failed to read data frame: %s", err)
	}

	switch header.Compression {
	case logpb.ButlerMetadata_ZLIB:
		r, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			return nil, nil, fmt.Errorf("test: failed to create zlib reader: %s", err)
		}
		defer r.Close()

		data, err = ioutil.ReadAll(r)
		if err != nil {
			return nil, nil, fmt.Errorf("test: failed to read compressed data: %s", err)
		}
	}

	dataBundle := logpb.ButlerLogBundle{}
	if err := proto.Unmarshal(data, &dataBundle); err != nil {
		return nil, nil, fmt.Errorf("test: failed to unmarshal bundle: %s", err)
	}

	return &header, &dataBundle, nil
}

func TestOutput(t *testing.T) {
	Convey(`An Output using a test Pub/Sub instance`, t, func() {
		ctx, tc := testclock.UseTime(context.Background(), time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))
		ctx = gologger.StdConfig.Use(ctx)
		ctx = logging.SetLevel(ctx, logging.Debug)
		tt := &testTopic{
			msgC: make(chan *pubsub.Message),
		}
		conf := Config{
			Topic: tt,
		}
		o := New(ctx, conf).(*pubSubOutput)
		So(o, ShouldNotBeNil)
		defer o.Close()

		bundle := &logpb.ButlerLogBundle{
			Timestamp: google.NewTimestamp(clock.Now(ctx)),
			Entries: []*logpb.ButlerLogBundle_Entry{
				{},
			},
		}

		Convey(`Can send/receive a bundle.`, func() {
			errC := make(chan error)
			go func() {
				errC <- o.SendBundle(bundle)
			}()
			msg := <-tt.msgC
			So(<-errC, ShouldBeNil)

			h, b, err := deconstructMessage(msg)
			So(err, ShouldBeNil)
			So(h.Compression, ShouldEqual, logpb.ButlerMetadata_NONE)
			So(b, ShouldResembleProto, bundle)

			Convey(`And records stats.`, func() {
				st := o.Stats()
				So(st.Errors(), ShouldEqual, 0)
				So(st.SentBytes(), ShouldBeGreaterThan, 0)
				So(st.SentMessages(), ShouldEqual, 1)
				So(st.DiscardedMessages(), ShouldEqual, 0)
			})
		})

		Convey(`Will return an error if Publish failed non-transiently.`, func() {
			tt.err = func() error { return grpcutil.InvalidArgument }
			So(o.SendBundle(bundle), ShouldEqual, grpcutil.InvalidArgument)
		})

		Convey(`Will retry indefinitely if Publish fails transiently (Context deadline).`, func() {
			const retries = 30

			// Advance our clock each time there is a delay up until count.
			count := 0
			tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
				switch {
				case !testclock.HasTags(t, clock.ContextDeadlineTag):
					// Other timer (probably retry sleep), advance time.
					tc.Add(d)

				case count < retries:
					// Still retrying, advance time.
					count++
					tc.Add(d)

				default:
					// Done retrying, don't expire Contexts anymore. Consume the message
					// when it is sent.
					tt.err = func() error { return grpcutil.InvalidArgument }
				}
			})

			// Time our our RPC. Because of our timer callback, this will always be
			// hit.
			o.RPCTimeout = 30 * time.Second
			So(o.SendBundle(bundle), ShouldEqual, grpcutil.InvalidArgument)
			So(count, ShouldEqual, retries)
		})

		Convey(`Will retry indefinitely if Publish fails transiently (gRPC).`, func() {
			const retries = 30

			// Advance our clock each time there is a delay up until count.
			tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
				tc.Add(d)
			})

			count := 0
			tt.msgC = nil
			tt.err = func() error {
				count++
				if count < retries {
					return grpcutil.Internal
				}
				return grpcutil.NotFound // Non-transient.
			}

			// Time our our RPC. Because of our timer callback, this will always be
			// hit.
			So(o.SendBundle(bundle), ShouldEqual, grpcutil.NotFound)
			So(count, ShouldEqual, retries)
		})
	})
}
