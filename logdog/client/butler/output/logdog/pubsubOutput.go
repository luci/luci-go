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
	"context"
	"fmt"
	"sync"
	"time"

	"cloud.google.com/go/pubsub"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/data/recordio"
	"go.chromium.org/luci/common/errors"
	gcps "go.chromium.org/luci/common/gcloud/pubsub"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/grpc/grpcutil"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/butler/bootstrap"
	"go.chromium.org/luci/logdog/client/butler/output"
	"go.chromium.org/luci/logdog/client/pubsubprotocol"
	"go.chromium.org/luci/logdog/common/types"
)

// pubsubTopic is an interface for a Pub/Sub topic.
//
// pubsub.Topic implements Topic.
type pubsubTopic interface {
	// String returns the name of the topic.
	String() string

	// Publish mirrors the pubsub.Connection Publish method.
	Publish(context.Context, *pubsub.Message) (string, error)
}

// pubsubConfig is a configuration structure for Pub/Sub output.
type pubsubConfig struct {
	// Topic is the Pub/Sub topic to publish to.
	Topic pubsubTopic

	// The coordinator host which is managing the published data.
	Host string

	// Project/Prefix/Secret to inject into each published bundle.
	Project string
	Prefix  string
	Secret  types.PrefixSecret

	// Compress, if true, enables zlib compression.
	Compress bool

	// RPCTimeout is the timeout to apply to an individual RPC.
	RPCTimeout time.Duration
}

// buffer
type buffer struct {
	bytes.Buffer // Output buffer for published message data.

	frameWriter recordio.Writer
	protoWriter *pubsubprotocol.Writer
}

// Butler Output that sends messages into Google Cloud PubSub as compressed
// protocol buffer blobs.
type pubSubOutput struct {
	*pubsubConfig
	context.Context

	bufferPool sync.Pool // Pool of reusable buffer instances.

	statsMu sync.Mutex
	stats   output.StatsBase
}

// newPubsub instantiates a new GCPS output.
func newPubsub(ctx context.Context, c pubsubConfig) output.Output {
	o := pubSubOutput{
		pubsubConfig: &c,
	}
	o.bufferPool.New = func() any { return &buffer{} }

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

	bundle.Project = o.Project
	bundle.Prefix = o.Prefix
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

	st.F.SentBytes += int64(len(message.Data))
	st.F.SentMessages++
	return nil
}

func (*pubSubOutput) MaxSendBundles() int {
	return 16
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

func (o *pubSubOutput) URLConstructionEnv() bootstrap.Environment {
	return bootstrap.Environment{
		CoordinatorHost: o.Host,
		Project:         o.Project,
		Prefix:          o.Prefix,
	}
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
		buf.protoWriter = &pubsubprotocol.Writer{
			Compress:          o.Compress,
			CompressThreshold: pubsubprotocol.DefaultCompressThreshold,
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
			var cancel context.CancelFunc
			ctx, cancel = clock.WithTimeout(o, o.RPCTimeout)
			defer cancel()
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
