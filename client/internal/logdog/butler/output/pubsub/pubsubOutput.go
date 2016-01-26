// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pubsub

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	"github.com/luci/luci-go/client/internal/logdog/butler/output"
	"github.com/luci/luci-go/common/gcloud/pubsub"
	"github.com/luci/luci-go/common/logdog/butlerproto"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
	"github.com/luci/luci-go/common/recordio"
	"golang.org/x/net/context"
)

// Publisher is an interface for something that publishes Pub/Sub messages.
//
// pubsub.Connection implements this interface.
type Publisher interface {
	// Publish mirrors the pubsub.Connection Publish method.
	Publish(context.Context, pubsub.Topic, ...*pubsub.Message) ([]string, error)
}

var _ Publisher = pubsub.Connection(nil)

// Config is a configuration structure for Pub/Sub output.
type Config struct {
	// Publisher is the Pub/Sub instance to use.
	Publisher Publisher

	// Topic is the name of the Cloud Pub/Sub topic to publish to.
	Topic pubsub.Topic

	// Compress, if true, enables zlib compression.
	Compress bool
}

// Validate validates the Output configuration.
func (c *Config) Validate() error {
	if c.Publisher == nil {
		return errors.New("pubsub: no pub/sub instance configured")
	}
	if err := c.Topic.Validate(); err != nil {
		return fmt.Errorf("pubsub: invalid Topic [%s]: %s", c.Topic, err)
	}
	return nil
}

// buffer
type buffer struct {
	bytes.Buffer // Output buffer for published message data.

	frameWriter recordio.Writer
	protoWriter *butlerproto.Writer
}

// Butler Output that sends messages into Google Cloud PubSub as compressed
// protocol buffer blobs.
type pubSubOutput struct {
	*Config
	context.Context

	bufferPool sync.Pool // Pool of reusable buffer instances.

	statsMu sync.Mutex
	stats   output.StatsBase
}

// New instantiates a new GCPS output.
func New(ctx context.Context, c Config) output.Output {
	o := pubSubOutput{
		Config: &c,
	}
	o.bufferPool.New = func() interface{} { return &buffer{} }
	o.Context = log.SetField(ctx, "pubsub", &o)
	return &o
}

func (o *pubSubOutput) String() string {
	return fmt.Sprintf("pubsub(%s)", o.Topic)
}

func (o *pubSubOutput) SendBundle(bundle *logpb.ButlerLogBundle) error {
	st := output.StatsBase{}
	defer o.mergeStats(&st)

	b := o.bufferPool.Get().(*buffer)
	defer o.bufferPool.Put(b)

	message, err := o.buildMessage(b, bundle)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(o, "Failed to build PubSub Message from bundle.")
		st.F.DiscardedMessages++
		st.F.Errors++
		return err
	}
	if len(message.Data) > pubsub.MaxPublishSize {
		log.Fields{
			"messageSize":   len(message.Data),
			"maxPubSubSize": pubsub.MaxPublishSize,
		}.Errorf(o, "Constructed message exceeds Pub/Sub maximum size.")
		return errors.New("pubsub: bundle contents violate Pub/Sub size limit")
	}
	if err := o.publishMessages([]*pubsub.Message{message}); err != nil {
		st.F.DiscardedMessages++
		st.F.Errors++
		return err
	}

	st.F.SentBytes += len(message.Data)
	st.F.SentMessages++
	return nil
}

func (*pubSubOutput) MaxSize() int {
	return pubsub.MaxPublishSize / 2
}

func (o *pubSubOutput) Stats() output.Stats {
	o.statsMu.Lock()
	defer o.statsMu.Unlock()

	statsCopy := o.stats
	return &statsCopy
}

func (o *pubSubOutput) Close() {
	// Nothing to do.
}

// buildMessage constructs a Pub/Sub Message out of LogDog frames.
//
// The first frame will be a ButlerMetadata message describing the second
// frame. The second frame will be a ButlerLogBundle containing the bundle
// data.
func (o *pubSubOutput) buildMessage(buf *buffer, bundle *logpb.ButlerLogBundle) (*pubsub.Message, error) {
	if buf.protoWriter == nil {
		buf.protoWriter = &butlerproto.Writer{
			Compress:          o.Compress,
			CompressThreshold: butlerproto.DefaultCompressThreshold,
		}
	}

	// Clear our buffer and (re)initialize our frame writer.
	buf.Reset()
	if buf.frameWriter == nil {
		buf.frameWriter = recordio.NewWriter(buf)
	} else {
		buf.frameWriter.Reset(buf)
	}

	if err := buf.protoWriter.WriteWith(buf.frameWriter, bundle); err != nil {
		return nil, err
	}

	return &pubsub.Message{
		Data: buf.Bytes(),
	}, nil
}

// publishMessages handles an individual publish request. It will indefinitely
// retry transient errors until the publish succeeds.
func (o *pubSubOutput) publishMessages(messages []*pubsub.Message) error {
	messageIDs, err := o.Publisher.Publish(o, o.Topic, messages...)
	if err != nil {
		return err
	}
	if err != nil {
		log.Errorf(log.SetError(o, err), "Failed to send PubSub message.")
		return err
	}

	log.Debugf(log.SetField(o, "messageIds", messageIDs), "Published messages.")
	return nil
}

func (o *pubSubOutput) mergeStats(s output.Stats) {
	o.statsMu.Lock()
	defer o.statsMu.Unlock()

	o.stats.Merge(s)
}
