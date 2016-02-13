// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package ackbuffer implements a Pub/Sub acknowledgement buffer capability.
// Pub/Sub ACKs will be collected and batched before being sent to Pub/Sub,
// with specific deadline enforcement.
package ackbuffer

import (
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/gcloud/pubsub"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/parallel"
	"golang.org/x/net/context"
)

const (
	// DefaultMaxBufferTime is the default amount of time that an ACK will remain
	// buffered before being sent.
	//
	// We base this off the default acknowledgement delay.
	DefaultMaxBufferTime = (pubsub.DefaultMaxAckDelay / 6)

	// DefaultMaxParallelACK is the default maximum number of simultaneous
	// parallel ACK request goroutines.
	DefaultMaxParallelACK = 16
)

// DiscardCallback is a callback method that will be invoked if ACK IDs must
// be discarded.
type DiscardCallback func(ackIDs []string)

// Config is a set of configuration parameters for an AckBuffer.
type Config struct {
	// Ack is the Pub/Sub instance to ACK with.
	Ack Acknowledger

	// MaxBufferTime is the maximum amount of time to buffer an ACK before sending it.
	MaxBufferTime time.Duration

	// The maximum number of parallel ACK requests that can be simultaneously
	// open. If zero, DefaultMaxParallelACK will be used.
	MaxParallelACK int

	// DiscardCallback is invoked when a series of ACK IDs is discarded after
	// repeated failures to ACK. If this is nil, no callback will be invoked.
	DiscardCallback DiscardCallback
}

// AckBuffer buffers Pub/Sub ACK requests together and sends them in batches.
// If a batch of ACKs fails to send (after retries), it will be discarded with
// an optional callback.
//
// After ACKs are enqueued, the AckBuffer should be flushed via CloseAndFlush
// to ensure that all ACKs have had a time to be sent.
type AckBuffer struct {
	cfg *Config
	ctx context.Context

	meterFinishedC chan struct{}

	ackC         chan string   // Used to send ACK requests.
	ackRequestC  chan []string // Used to submit ACK requests to ACK goroutine.
	ackFinishedC chan struct{} // Closed when ACK goroutine is finished.

	ackReceivedC chan string // (Testing) if not nil, send received ACKs.
}

// New instantiates a new AckBuffer. The returned AckBuffer must have its
// CloseAndFlush method invoked before terminating, else data loss may occur.
func New(ctx context.Context, c Config) *AckBuffer {
	if c.MaxBufferTime <= 0 {
		c.MaxBufferTime = DefaultMaxBufferTime
	}
	if c.MaxParallelACK <= 0 {
		c.MaxParallelACK = DefaultMaxParallelACK
	}

	batchSize := c.Ack.AckBatchSize()
	b := AckBuffer{
		cfg:            &c,
		ctx:            ctx,
		ackC:           make(chan string, batchSize),
		meterFinishedC: make(chan struct{}),
		ackRequestC:    make(chan []string),
		ackFinishedC:   make(chan struct{}),
	}

	// Start a meter goroutine. This will buffer ACKs and send them at either
	// capacity or timer intervals.
	go func() {
		defer close(b.ackRequestC)

		// Create a timer. This will tick each time the buffer is empty and get a
		// new ACK to track the longest time we've been buffering an ACK. We will
		// reset the timer each time we clear the buffer.
		timerRunning := false
		timer := clock.NewTimer(ctx)
		defer timer.Stop()

		buf := make([]string, 0, batchSize)
		send := func() {
			if len(buf) > 0 {
				ackIDs := make([]string, len(buf))
				copy(ackIDs, buf)
				b.ackRequestC <- ackIDs
				buf = buf[:0]
			}

			timer.Stop()
			timerRunning = false
		}

		// When terminating, flush any remaining ACKs in the buffer.
		defer send()

		// Ingest and dispatch ACKs.
		for {
			select {
			case ack, ok := <-b.ackC:
				if !ok {
					// Closing, exit loop.
					return
				}
				buf = append(buf, ack)
				switch {
				case len(buf) == cap(buf):
					send()
				case !timerRunning:
					// Not at capacity yet, and our timer's not running, so start counting
					// down.
					timer.Reset(b.cfg.MaxBufferTime)
					timerRunning = true
				}

				// (Testing) Notify when ACKs are received.
				if b.ackReceivedC != nil {
					b.ackReceivedC <- ack
				}

			case <-timer.GetC():
				send()
			}
		}
	}()

	// Start our ACK loop.
	go func() {
		defer close(b.ackFinishedC)

		// Allocate and populate a set of ACK tokens. This will be used as a
		// semaphore to control the number of parallel ACK requests.
		sem := make(parallel.Semaphore, b.cfg.MaxParallelACK)
		for req := range b.ackRequestC {
			req := req

			// Take out an ACK token.
			sem.Lock()
			go func() {
				defer sem.Unlock()
				b.acknowledge(req)
			}()
		}

		// Block until all ACK goroutines finish.
		sem.TakeAll()
	}()

	return &b
}

// Ack enqueues a message's ACK ID for acknowledgement.
func (b *AckBuffer) Ack(id string) {
	b.ackC <- id
}

// CloseAndFlush closes the AckBuffer, blocking until all pending ACKs are
// complete.
func (b *AckBuffer) CloseAndFlush() {
	// Close our ackC. This will terminate our meter goroutine, which will
	// terminate our ACK goroutine.
	close(b.ackC)

	// Wait for ACK goroutine to terminate.
	<-b.ackFinishedC
}

// acknowledge acknowledges a set of IDs.
//
// This method will discard the ACKs if they fail.
func (b *AckBuffer) acknowledge(ackIDs []string) {
	if err := b.cfg.Ack.Ack(b.ctx, ackIDs...); err != nil {
		log.Fields{
			log.ErrorKey: err,
			"count":      len(ackIDs),
		}.Errorf(b.ctx, "Failed to ACK.")
		if b.cfg.DiscardCallback != nil {
			b.cfg.DiscardCallback(ackIDs)
		}
	}
}
