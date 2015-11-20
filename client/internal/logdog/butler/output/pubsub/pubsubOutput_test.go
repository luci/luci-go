// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pubsub

import (
	"bytes"
	"compress/zlib"
	"errors"
	"fmt"
	"io/ioutil"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/gcloud/gcps"
	"github.com/luci/luci-go/common/logdog/protocol"
	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/common/recordio"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
	"google.golang.org/cloud/pubsub"
)

type testPubSub struct {
	sync.Mutex

	err   error
	topic gcps.Topic

	msgC          chan *pubsub.Message
	nextMessageID int
}

func (*testPubSub) TopicExists(gcps.Topic) (bool, error)                   { panic("not implemented") }
func (*testPubSub) SubExists(gcps.Subscription) (bool, error)              { panic("not implemented") }
func (*testPubSub) Pull(gcps.Subscription, int) ([]*pubsub.Message, error) { panic("not implemented") }
func (*testPubSub) Ack(gcps.Subscription, ...string) error                 { panic("not implemented") }

func (ps *testPubSub) Publish(t gcps.Topic, msgs ...*pubsub.Message) ([]string, error) {
	if ps.err != nil {
		return nil, ps.err
	}
	if t != ps.topic {
		return nil, fmt.Errorf("test: published topic doesn't match configured (%s != %s)", t, ps.topic)
	}

	ids := make([]string, len(msgs))
	for i, m := range msgs {
		ps.msgC <- m
		ids[i] = ps.getNextMessageID()
	}
	return ids, nil
}

func (ps *testPubSub) getNextMessageID() string {
	ps.Lock()
	defer ps.Unlock()

	id := ps.nextMessageID
	ps.nextMessageID++
	return fmt.Sprintf("%d", id)
}

func TestConfig(t *testing.T) {
	Convey(`A configuration instance`, t, func() {
		ps := &testPubSub{}
		conf := Config{
			PubSub: ps,
			Topic:  gcps.Topic("test-topic"),
		}

		Convey(`Will successfully validate.`, func() {
			So(conf.Validate(), ShouldBeNil)
		})

		Convey(`Will not validate without a PubSub instance.`, func() {
			conf.PubSub = nil
			So(conf.Validate(), ShouldNotBeNil)
		})

		Convey(`Will not validate with an empty Topic.`, func() {
			conf.Topic = ""
			So(conf.Validate(), ShouldNotBeNil)
		})

		Convey(`Will not validate with an invalid Topic.`, func() {
			conf.Topic = gcps.Topic("a!")
			So(conf.Topic.Validate(), ShouldNotBeNil)
			So(conf.Validate(), ShouldNotBeNil)
		})
	})
}

func deconstructMessage(msg *pubsub.Message) (*protocol.ButlerMetadata, *protocol.ButlerLogBundle, error) {
	fr := recordio.NewReader(bytes.NewBuffer(msg.Data), gcps.MaxPublishSize)

	// Validate header frame.
	headerBytes, err := fr.ReadFrameAll()
	if err != nil {
		return nil, nil, fmt.Errorf("test: failed to read header frame: %s", err)
	}

	header := protocol.ButlerMetadata{}
	if err := proto.Unmarshal(headerBytes, &header); err != nil {
		return nil, nil, fmt.Errorf("test: failed to unmarshal header: %s", err)
	}

	if header.Type != protocol.ButlerMetadata_ButlerLogBundle {
		return nil, nil, fmt.Errorf("test: unknown frame data type: %v", header.Type)
	}

	// Validate data frame.
	data, err := fr.ReadFrameAll()
	if err != nil {
		return nil, nil, fmt.Errorf("test: failed to read data frame: %s", err)
	}

	switch header.Compression {
	case protocol.ButlerMetadata_ZLIB:
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

	dataBundle := protocol.ButlerLogBundle{}
	if err := proto.Unmarshal(data, &dataBundle); err != nil {
		return nil, nil, fmt.Errorf("test: failed to unmarshal bundle: %s", err)
	}

	return &header, &dataBundle, nil
}

func TestOutput(t *testing.T) {
	Convey(`An Output using a test Pub/Sub instance`, t, func() {
		ctx, _ := testclock.UseTime(context.Background(), time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC))
		ps := &testPubSub{
			topic: gcps.Topic("test-topic"),
			msgC:  make(chan *pubsub.Message, 1),
		}
		conf := Config{
			PubSub: ps,
			Topic:  gcps.Topic("test-topic"),
		}
		o := New(ctx, conf).(*gcpsOutput)
		So(o, ShouldNotBeNil)
		defer o.Close()

		bundle := &protocol.ButlerLogBundle{
			Source:    "GCPS Test",
			Timestamp: google.NewTimestamp(clock.Now(ctx)),
			Entries: []*protocol.ButlerLogBundle_Entry{
				{},
			},
		}

		Convey(`Can send/receive a bundle.`, func() {
			So(o.SendBundle(bundle), ShouldBeNil)

			h, b, err := deconstructMessage(<-ps.msgC)
			So(err, ShouldBeNil)
			So(h.Compression, ShouldEqual, protocol.ButlerMetadata_NONE)
			So(b, ShouldResemble, bundle)

			Convey(`And records stats.`, func() {
				st := o.Stats()
				So(st.Errors(), ShouldEqual, 0)
				So(st.SentBytes(), ShouldBeGreaterThan, 0)
				So(st.SentMessages(), ShouldEqual, 1)
				So(st.DiscardedMessages(), ShouldEqual, 0)
			})
		})

		Convey(`Will return an error if Publish failed.`, func() {
			ps.err = errors.New("test: error")
			So(o.SendBundle(bundle), ShouldNotBeNil)
		})
	})
}
