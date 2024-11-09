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

package logdog

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"cloud.google.com/go/pubsub"
	"github.com/golang/protobuf/proto"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/data/recordio"
	gcps "go.chromium.org/luci/common/gcloud/pubsub"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/gologger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/logdog/api/logpb"
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

		data, err = io.ReadAll(r)
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
	ftt.Run(`An Output using a test Pub/Sub instance`, t, func(t *ftt.Test) {
		ctx, tc := testclock.UseTime(context.Background(), time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))
		ctx = gologger.StdConfig.Use(ctx)
		ctx = logging.SetLevel(ctx, logging.Debug)
		tt := &testTopic{
			msgC: make(chan *pubsub.Message),
		}
		conf := pubsubConfig{
			Topic: tt,
		}
		o := newPubsub(ctx, conf).(*pubSubOutput)
		assert.Loosely(t, o, should.NotBeNil)
		defer o.Close()

		bundle := &logpb.ButlerLogBundle{
			Timestamp: timestamppb.New(clock.Now(ctx)),
			Entries: []*logpb.ButlerLogBundle_Entry{
				{},
			},
		}

		t.Run(`Can send/receive a bundle.`, func(t *ftt.Test) {
			errC := make(chan error)
			go func() {
				errC <- o.SendBundle(bundle)
			}()
			msg := <-tt.msgC
			assert.Loosely(t, <-errC, should.BeNil)

			h, b, err := deconstructMessage(msg)
			assert.Loosely(t, err, should.BeNil)
			assert.Loosely(t, h.Compression, should.Equal(logpb.ButlerMetadata_NONE))
			assert.Loosely(t, b, should.Resemble(bundle))

			t.Run(`And records stats.`, func(t *ftt.Test) {
				st := o.Stats()
				assert.Loosely(t, st.Errors(), should.BeZero)
				assert.Loosely(t, st.SentBytes(), should.BeGreaterThan(0))
				assert.Loosely(t, st.SentMessages(), should.Equal(1))
				assert.Loosely(t, st.DiscardedMessages(), should.BeZero)
			})
		})

		t.Run(`Will return an error if Publish failed non-transiently.`, func(t *ftt.Test) {
			err := status.Error(codes.InvalidArgument, "boom")
			tt.err = func() error { return err }
			assert.Loosely(t, o.SendBundle(bundle), should.Equal(err))
		})

		t.Run(`Will retry indefinitely if Publish fails transiently (Context deadline).`, func(t *ftt.Test) {
			const retries = 30

			stopErr := status.Error(codes.InvalidArgument, "boom")

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
					tt.err = func() error { return stopErr }
				}
			})

			// Time our our RPC. Because of our timer callback, this will always be
			// hit.
			o.RPCTimeout = 30 * time.Second
			assert.Loosely(t, o.SendBundle(bundle), should.Equal(stopErr))
			assert.Loosely(t, count, should.Equal(retries))
		})

		t.Run(`Will retry indefinitely if Publish fails transiently (gRPC).`, func(t *ftt.Test) {
			const retries = 30

			// Advance our clock each time there is a delay up until count.
			tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
				tc.Add(d)
			})

			stopErr := status.Error(codes.NotFound, "boom")

			count := 0
			tt.msgC = nil
			tt.err = func() error {
				count++
				if count < retries {
					return status.Error(codes.Internal, "internal server error")
				}
				return stopErr // Non-transient.
			}

			// Time our our RPC. Because of our timer callback, this will always be
			// hit.
			assert.Loosely(t, o.SendBundle(bundle), should.Equal(stopErr))
			assert.Loosely(t, count, should.Equal(retries))
		})
	})
}
