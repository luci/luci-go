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

// Package pubsub provides a generic way to batch pubsub pull
// notifications.
package pubsub

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"cloud.google.com/go/pubsub"

	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/cv/internal/common"
)

// PullingBatchProcessor batches notifications pulled from a pubsub
// subscription, and calls a custom process function on each batch.
//
// Provides an endpoint to be called by e.g. a cron job that starts the message
// pulling and processing cycle for the specified time. (See .Process())
type PullingBatchProcessor struct {
	// ProcessBatch is a function to handle one batch of messages.
	//
	// The messages aren't yet ack-ed when they are passed to this func.
	// The func is allowed to Ack or Nack them as it sees fit.
	// This can be useful, for example, if processing some messages in a batch
	// succeeds and fails on others.
	//
	// As a fail-safe, PullingBatchProcessor will **always** call Nack() or
	// Ack() on all the messages after ProcessBatch completes.
	// This is fine because PubSub client ignores any subsequent calls to Nack
	// or Ack, thus the fail-safe won't override any Nack/Ack previously issued
	// by the ProcessBatch.
	//
	// The fail-safe uses Nack() if the error returned by the ProcessBatch is
	// transient, thus asking for re-delivery and an eventual retry.
	// Ack() is used otherwise, which prevents retries on permanent errors.
	ProcessBatch ProcessBatchFunc

	// ProjectID is the project id for the subscription below.
	ProjectID string

	// SubID is the id of the pubsub subscription to pull messages from.
	// It must exist.
	SubID string

	// Options are optional. Only nonzero fields will be applied.
	Options Options
}

// Options control the operation of a PullingBatchProcessor.
type Options struct {
	// ReceiveDuration limits the duration of the Process() execution.
	//
	// It actually determines how long to receive pubsub messages for.
	ReceiveDuration time.Duration

	// MaxBatchSize limits how many messages to process in a single batch.
	MaxBatchSize int

	// ConcurrentBatches controls the number of batches being processed
	// concurrently.
	ConcurrentBatches int
}

// ProcessBatchFunc is the signature that the batch processing function needs
// to have.
type ProcessBatchFunc func(context.Context, []*pubsub.Message) error

func defaultOptions() Options {
	return Options{
		// 5 minutes is reasonable because e.g. in AppEngine with auto scaling
		// all request handlers can run for up to 10 minutes with the golang
		// runtime.
		//
		// It also isn't too long to wait for old requests to complete when
		// deploying a new version.
		ReceiveDuration: 5 * time.Minute,

		// 20 is large enough that the advantages of batching are clear
		// e.g a 95% reduction in fixed overhead per message
		// (only true if the overhead is independent of the batch size),
		// but also not so large that a list of build ids of this size is
		// unwieldy to visually parse, or process manually.
		MaxBatchSize: 20,

		// Canonical cardinality concerning classic concurrency conundrum.
		// See EWD-310 p.20
		ConcurrentBatches: 5,
	}
}

// batchErrKind is to communicate to Process() about transient/non-transient errors
// in a processed batch.
type batchErrKind int

const (
	ok    batchErrKind = iota // No errors occurred in the batch.
	trans                     // Transient errors occurred in the batch, retry the whole batch.
	perm                      // Permanent errors occurred in the batch, drop the whole batch.
)

// Process is the endpoint that (e.g. by cron job) should be periodically hit
// to operate the PullingBatchProcessor.
//
// It creates the pubsub client and processes notifications for up to
// Options.CronRunTime.
func (pbp *PullingBatchProcessor) Process(ctx context.Context) error {
	client, err := pubsub.NewClient(ctx, pbp.ProjectID)
	if err != nil {
		return err
	}
	defer func() {
		if err := client.Close(); err != nil {
			logging.Errorf(ctx, "failed to close PubSub client: %s", err)
		}
	}()
	return pbp.process(ctx, client)
}

// process actually does what Process advertises, modulo creation of the
// pubsub client.
//
// Unit tests can call this directly with a mock client.
func (pbp *PullingBatchProcessor) process(ctx context.Context, client *pubsub.Client) error {
	if client == nil {
		return errors.New("cannot run process() without an initialized client")
	}

	// These are atomically incremented when batch processing results in either
	// kind of error.
	//
	// Process() will return an error if there's one or more permanent errors.
	var permanentErrorCount, transientErrorCount int32

	sub := client.Subscription(pbp.SubID)
	sub.ReceiveSettings.Synchronous = true
	// Only lease as many messages from pubsub as we can concurrently send.
	sub.ReceiveSettings.MaxOutstandingMessages = pbp.Options.MaxBatchSize * pbp.Options.ConcurrentBatches

	// Get the first permanent error for surfacing details.
	var firstPermErr error

	workItems := make(chan *pubsub.Message)
	wg := sync.WaitGroup{}
	wg.Add(pbp.Options.ConcurrentBatches)
	for i := 0; i < pbp.Options.ConcurrentBatches; i++ {
		go func() {
			defer wg.Done()
			for {
				batch := nextBatch(workItems, pbp.Options.MaxBatchSize)
				if batch == nil {
					return
				}
				switch status, err := pbp.onBatch(ctx, batch); status {
				case perm:
					if atomic.AddInt32(&permanentErrorCount, 1) == 1 {
						firstPermErr = err
					}
				case trans:
					atomic.AddInt32(&transientErrorCount, 1)
				}
			}
		}()
	}

	receiveCtx, receiveCancel := clock.WithTimeout(ctx, pbp.Options.ReceiveDuration)
	defer receiveCancel()
	err := sub.Receive(receiveCtx, func(ctx context.Context, msg *pubsub.Message) {
		workItems <- msg
	})
	close(workItems)
	wg.Wait()
	logging.Debugf(ctx, "Processed: %d batches with transient and %d with permanent errors", transientErrorCount, permanentErrorCount)

	// Check receive error _after_ worker pool is done to avoid leakages.
	if err != nil {
		// Receive exitted due to something other than timeout or cancellation, i.e. non-retryable service error.
		return errors.Annotate(err, "failed call to pubsub receive").Err()
	}
	if permanentErrorCount > 0 {
		return errors.Reason("Process had non-transient errors. E.g. %q. Review logs for more details.", firstPermErr).Err()
	}
	return nil
}

func (opts *Options) normalize() {
	defaults := defaultOptions()
	if opts.ReceiveDuration == 0 {
		opts.ReceiveDuration = defaults.ReceiveDuration
	}
	if opts.MaxBatchSize == 0 {
		opts.MaxBatchSize = defaults.MaxBatchSize
	}
	if opts.ConcurrentBatches == 0 {
		opts.ConcurrentBatches = defaults.ConcurrentBatches
	}
}

// Validate checks missing required fields and normalizes options.
func (pbp *PullingBatchProcessor) Validate() error {
	if pbp.ProjectID == "" {
		return errors.Reason("PullingBatchProcessor.ProjectID is required").Err()
	}
	if pbp.SubID == "" {
		return errors.Reason("PullingBatchProcessor.SubID is required").Err()
	}
	if pbp.ProcessBatch == nil {
		return errors.Reason("PullingBatchProcessor.ProcessBatch is required").Err()
	}
	if pbp.Options.ReceiveDuration < 0 {
		return errors.Reason("Options.ReceiveDuration cannot be negative").Err()
	}
	if pbp.Options.ConcurrentBatches < 0 {
		return errors.Reason("Options.ConcurrentBatches cannot be negative").Err()
	}
	if pbp.Options.MaxBatchSize < 0 {
		return errors.Reason("Options.MaxBatchSize cannot be negative").Err()
	}
	pbp.Options.normalize()
	return nil
}

func (pbp *PullingBatchProcessor) onBatch(ctx context.Context, msgs []*pubsub.Message) (batchErrKind, error) {
	// Make a copy of the messages slice to prevent losing access to messages
	// (and thus, the ability to ack/nack them) if ProcessBatch were to change
	// the contents of the slice.
	msgsCopy := append(make([]*pubsub.Message, 0, len(msgs)), msgs...)
	err := pbp.ProcessBatch(ctx, msgsCopy)
	// Note that ProcessBatch is allowed to ack/nack messages itself,
	// for those messages, our acking/nacking below will have no effect.
	switch {
	case transient.Tag.In(err):
		// Ask for re-delivery later.
		common.LogError(ctx, errors.Annotate(err, "NACKing for redelivery").Err())
		nackAll(msgs)
		return trans, err
	case err != nil:
		common.LogError(ctx, errors.Annotate(err, "ACKing to avoid retries").Err())
		ackAll(msgs)
		return perm, err
	default:
		ackAll(msgs)
		return ok, err
	}
}

// nextBatch pulls up to n immediately available items from c.
//
// It blocks until at least one item is available.
// If called with a closed channel, will return nil.
func nextBatch(c <-chan *pubsub.Message, n int) []*pubsub.Message {
	msg, stillOpen := <-c
	if !stillOpen {
		return nil
	}
	out := append(make([]*pubsub.Message, 0, n), msg)
	for len(out) < n {
		select {
		case msg, stillOpen := <-c:
			if !stillOpen {
				return out
			}
			out = append(out, msg)
		default:
			return out
		}
	}
	return out
}

func ackAll(msgs []*pubsub.Message) {
	for _, msg := range msgs {
		msg.Ack()
	}
}

func nackAll(msgs []*pubsub.Message) {
	for _, msg := range msgs {
		msg.Nack()
	}
}
