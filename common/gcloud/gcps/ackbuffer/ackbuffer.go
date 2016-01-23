// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package ackbuffer implements a Pub/Sub acknowledgement buffer capability.
// Pub/Sub ACKs will be collected and batched before being sent to Pub/Sub,
// with specific deadline enforcement.
package ackbuffer

import (
	"sync"
	"time"

	"github.com/luci/luci-go/common/gcloud/gcps"
	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/meter"
	"github.com/luci/luci-go/common/parallel"
	"golang.org/x/net/context"
)

const (
	// DefaultMaxBufferTime is the default amount of time that an ACK will remain
	// buffered before being sent.
	//
	// We base this off the default acknowledgement delay.
	DefaultMaxBufferTime = (gcps.DefaultMaxAckDelay / 6)

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

	meter meter.Meter

	ackRequestC  chan []string // Used to submit ACK requests to ACK goroutine.
	ackFinishedC chan struct{} // Closed when ACK goroutine is finished.
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

	b := &AckBuffer{
		cfg:          &c,
		ctx:          ctx,
		ackRequestC:  make(chan []string),
		ackFinishedC: make(chan struct{}),
	}
	b.meter = meter.New(ctx, meter.Config{
		Count:    b.cfg.Ack.AckBatchSize(),
		Delay:    b.cfg.MaxBufferTime,
		Callback: b.meterCallback,
	})

	// Start our ACK loop.
	wg := sync.WaitGroup{}
	go func() {
		defer close(b.ackFinishedC)

		// Allocate and populate a set of ACK tokens. This will be used as a
		// semaphore to control the number of parallel ACK requests.
		sem := make(parallel.Semaphore, b.cfg.MaxParallelACK)
		for req := range b.ackRequestC {
			req := req

			// Take out an ACK token.
			sem.Lock()
			wg.Add(1)
			go func() {
				defer func() {
					sem.Unlock()
					wg.Done()
				}()
				b.acknowledge(req)
			}()
		}

		// Block until all ACK goroutines finish.
		wg.Wait()
	}()

	return b
}

// Ack enqueues a message's ACK ID for acknowledgement.
func (b *AckBuffer) Ack(id string) {
	b.meter.AddWait(id)
}

// CloseAndFlush closes the AckBuffer, blocking until all pending ACKs are
// complete.
func (b *AckBuffer) CloseAndFlush() {
	b.meter.Stop()

	// Wait for ACK goroutine to terminate.
	close(b.ackRequestC)
	<-b.ackFinishedC
}

// meterCallback is the Meter callback that is invoked when a new batch of ACKs
// is encountered.
//
// This shouldn't block if possible, else the Meter will block. However, if
// ACK requests build up, this will block until they are finished.
func (b *AckBuffer) meterCallback(work []interface{}) {
	ackIDs := make([]string, len(work))
	for idx, w := range work {
		ackIDs[idx] = w.(string)
	}
	b.ackRequestC <- ackIDs
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
