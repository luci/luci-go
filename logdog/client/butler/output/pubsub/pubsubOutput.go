// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package pubsub

import (
	"bytes"
	"fmt"
	"sync"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/data/recordio"
	"github.com/luci/luci-go/common/errors"
	gcps "github.com/luci/luci-go/common/gcloud/pubsub"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/retry"
	"github.com/luci/luci-go/common/retry/transient"
	"github.com/luci/luci-go/grpc/grpcutil"
	"github.com/luci/luci-go/logdog/api/logpb"
	"github.com/luci/luci-go/logdog/client/butler/output"
	"github.com/luci/luci-go/logdog/client/butlerproto"
	"github.com/luci/luci-go/logdog/common/types"

	"cloud.google.com/go/pubsub"
	"golang.org/x/net/context"
)

// Topic is an interface for a Pub/Sub topic.
//
// pubsub.Topic implements Topic.
type Topic interface {
	// String returns the name of the topic.
	String() string

	// Publish mirrors the pubsub.Connection Publish method.
	Publish(context.Context, *pubsub.Message) (string, error)
}

// Config is a configuration structure for Pub/Sub output.
type Config struct {
	// Topic is the Pub/Sub topic to publish to.
	Topic Topic

	// Secret, if not nil, is the prefix secret to attach to each outgoing bundle.
	Secret types.PrefixSecret

	// Compress, if true, enables zlib compression.
	Compress bool

	// Track, if true, tracks all log entries that have been successfully
	// submitted.
	Track bool

	// RPCTimeout is the timeout to apply to an individual RPC.
	RPCTimeout time.Duration
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

	et *output.EntryTracker
}

// New instantiates a new GCPS output.
func New(ctx context.Context, c Config) output.Output {
	o := pubSubOutput{
		Config: &c,
	}
	o.bufferPool.New = func() interface{} { return &buffer{} }

	if c.Track {
		o.et = &output.EntryTracker{}
	}

	o.Context = log.SetField(ctx, "pubsub", &o)
	return &o
}

func (o *pubSubOutput) String() string {
	return fmt.Sprintf("pubsub(%s)", o.Topic.String())
}

func (o *pubSubOutput) SendBundle(bundle *logpb.ButlerLogBundle) error {
	st := output.StatsBase{}
	defer o.mergeStats(&st)

	b := o.bufferPool.Get().(*buffer)
	defer o.bufferPool.Put(b)

	bundle.Secret = []byte(o.Secret)
	message, err := o.buildMessage(b, bundle)
	if err != nil {
		log.Fields{
			log.ErrorKey: err,
		}.Errorf(o, "Failed to build PubSub Message from bundle.")
		st.F.DiscardedMessages++
		st.F.Errors++
		return err
	}
	if len(message.Data) > gcps.MaxPublishRequestBytes {
		log.Fields{
			"messageSize":   len(message.Data),
			"maxPubSubSize": gcps.MaxPublishRequestBytes,
		}.Errorf(o, "Constructed message exceeds Pub/Sub maximum size.")
		return errors.New("pubsub: bundle contents violate Pub/Sub size limit")
	}
	if err := o.publishMessage(message); err != nil {
		st.F.DiscardedMessages++
		st.F.Errors++
		return err
	}

	if o.et != nil {
		o.et.Track(bundle)
	}

	st.F.SentBytes += int64(len(message.Data))
	st.F.SentMessages++
	return nil
}

func (*pubSubOutput) MaxSize() int {
	return gcps.MaxPublishRequestBytes / 2
}

func (o *pubSubOutput) Stats() output.Stats {
	o.statsMu.Lock()
	defer o.statsMu.Unlock()

	statsCopy := o.stats
	return &statsCopy
}

func (o *pubSubOutput) Record() *output.EntryRecord {
	if o.et == nil {
		return nil
	}
	return o.et.Record()
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

// publishMessage handles an individual publish request. It will indefinitely
// retry transient errors until the publish succeeds.
func (o *pubSubOutput) publishMessage(message *pubsub.Message) error {
	var messageID string
	transientErrors := 0
	err := retry.Retry(o, transient.Only(indefiniteRetry), func() (err error) {
		ctx := o.Context
		if o.RPCTimeout > 0 {
			var cancelFunc context.CancelFunc
			ctx, cancelFunc = clock.WithTimeout(o, o.RPCTimeout)
			defer cancelFunc()
		}

		messageID, err = o.Topic.Publish(ctx, message)
		if err == context.DeadlineExceeded {
			// If we hit our publish deadline, retry.
			err = transient.Tag.Apply(err)
		} else {
			err = grpcutil.WrapIfTransient(err)
		}
		return
	}, func(err error, d time.Duration) {
		log.Fields{
			log.ErrorKey: err,
			"delay":      d,
		}.Warningf(o, "TRANSIENT error publishing messages; retrying...")
		transientErrors++
	})
	if err != nil {
		log.WithError(err).Errorf(o, "Failed to send PubSub message.")
		return err
	}

	if transientErrors > 0 {
		// We successfully published, but we hit a transient error, so explicitly
		// acknowledge this at warning-level for log message closure.
		log.Fields{
			"messageId":       messageID,
			"transientErrors": transientErrors,
		}.Warningf(o, "Successfully published messages after transient errors.")
	} else {
		log.Fields{
			"messageId": messageID,
		}.Debugf(o, "Published messages.")
	}
	return nil
}

func (o *pubSubOutput) mergeStats(s output.Stats) {
	o.statsMu.Lock()
	defer o.statsMu.Unlock()

	o.stats.Merge(s)
}

// indefiniteRetry is a retry.Iterator that will indefinitely retry errors with
// a maximum backoff.
func indefiniteRetry() retry.Iterator {
	return &retry.ExponentialBackoff{
		Limited: retry.Limited{
			Retries: -1,
			Delay:   500 * time.Millisecond,
		},
		MaxDelay: 30 * time.Second,
	}
}
