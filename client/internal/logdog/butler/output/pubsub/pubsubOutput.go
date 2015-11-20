// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pubsub

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/luci/luci-go/client/internal/logdog/butler/output"
	"github.com/luci/luci-go/common/gcloud/gcps"
	"github.com/luci/luci-go/common/logdog/butlerproto"
	"github.com/luci/luci-go/common/logdog/protocol"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/recordio"
	"github.com/luci/luci-go/common/retry"
	"golang.org/x/net/context"
	"google.golang.org/cloud/pubsub"
)

// Config is a configuration structure for GCPS output.
type Config struct {
	// Pubsub is the Pub/Sub instance to use.
	PubSub gcps.PubSub

	// Topic is the name of the Cloud Pub/Sub topic to publish to.
	Topic gcps.Topic

	// Compress, if true, enables zlib compression.
	Compress bool
}

// Validate validates the Output configuration.
func (c *Config) Validate() error {
	if c.PubSub == nil {
		return errors.New("gcps: no pub/sub instance configured")
	}
	if err := c.Topic.Validate(); err != nil {
		return fmt.Errorf("gcps: invalid Topic [%s]: %s", c.Topic, err)
	}
	return nil
}

// gcpsBuffer
type gcpsBuffer struct {
	bytes.Buffer // Output buffer for published message data.

	frameWriter recordio.Writer
	protoWriter *butlerproto.Writer
}

// Butler Output that sends messages into Google Cloud PubSub as compressed
// protocol buffer blobs.
type gcpsOutput struct {
	*Config

	ctx        context.Context // Retained context object.
	bufferPool sync.Pool       // Pool of reusable gcpsBuffer instances.

	statsMu sync.Mutex
	stats   output.StatsBase
}

// New instantiates a new GCPS output.
func New(ctx context.Context, c Config) output.Output {
	o := gcpsOutput{
		Config: &c,
		ctx:    ctx,
	}
	o.bufferPool.New = func() interface{} { return &gcpsBuffer{} }
	o.ctx = log.SetField(o.ctx, "output", &o)
	return &o
}

func (o *gcpsOutput) String() string {
	return fmt.Sprintf("gcps(%s)", o.Topic)
}

func (o *gcpsOutput) SendBundle(bundle *protocol.ButlerLogBundle) error {
	st := output.StatsBase{}
	defer o.mergeStats(&st)

	b := o.bufferPool.Get().(*gcpsBuffer)
	defer o.bufferPool.Put(b)

	message, err := o.buildMessage(b, bundle)
	if err != nil {
		log.Errorf(log.SetError(o.ctx, err), "Failed to build PubSub Message from bundle.")
		st.F.DiscardedMessages++
		st.F.Errors++
		return err
	}
	if len(message.Data) > gcps.MaxPublishSize {
		log.Fields{
			"messageSize":   len(message.Data),
			"maxPubSubSize": gcps.MaxPublishSize,
		}.Errorf(o.ctx, "Constructed message exceeds Pub/Sub maximum size.")
		return errors.New("gcps: bundle contents violate Pub/Sub size limit")
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

func (*gcpsOutput) MaxSize() int {
	return gcps.MaxPublishSize / 2
}

func (o *gcpsOutput) Stats() output.Stats {
	o.statsMu.Lock()
	defer o.statsMu.Unlock()

	statsCopy := o.stats
	return &statsCopy
}

func (o *gcpsOutput) Close() {
	// Nothing to do.
}

// buildMessage constructs a Pub/Sub Message out of LogDog frames.
//
// The first frame will be a ButlerMetadata message describing the second
// frame. The second frame will be a ButlerLogBundle containing the bundle
// data.
func (o *gcpsOutput) buildMessage(buf *gcpsBuffer, bundle *protocol.ButlerLogBundle) (*pubsub.Message, error) {
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
func (o *gcpsOutput) publishMessages(messages []*pubsub.Message) error {
	var messageIDs []string
	count := 0
	err := retry.Retry(o.ctx, retry.TransientOnly(o.publishRetryIterator()), func() error {
		ids, err := o.PubSub.Publish(o.Topic, messages...)
		if err != nil {
			return err
		}
		messageIDs = ids
		return nil
	}, func(err error, d time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"count":      count,
			"delay":      d,
		}.Warningf(o.ctx, "Transient publish error; retrying.")
		count++
	})
	if err != nil {
		log.Errorf(log.SetError(o.ctx, err), "Failed to send PubSub message.")
		return err
	}

	log.Debugf(log.SetField(o.ctx, "messageIds", messageIDs), "Published messages.")
	return nil
}

func (o *gcpsOutput) mergeStats(s output.Stats) {
	o.statsMu.Lock()
	defer o.statsMu.Unlock()

	o.stats.Merge(s)
}

// publishRetryIterator returns a retry.Iterator configured for publish
// requests.
//
// Note that this iterator has no retry upper bound. It will continue retrying
// indefinitely until success.
func (*gcpsOutput) publishRetryIterator() retry.Iterator {
	return &retry.ExponentialBackoff{
		Limited: retry.Limited{
			Delay: 500 * time.Millisecond,
		},
		Multiplier: 3.0,
		MaxDelay:   15 * time.Second,
	}
}
