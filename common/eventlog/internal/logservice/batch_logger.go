// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package logservice provides loggers which can be used to to collect and send batches of logs to the eventlog service.
package logservice

import (
	"context"
	"log"
	"sync"
	"time"

	logpb "github.com/luci/luci-go/common/eventlog/proto"
)

// numRetries is the number of retry attempts we will make to log a given event in the face of transient errors.
const numRetries = 3

// BatchLogger accumlates log events and sends them in batches. It is safe for concurrent use.
type BatchLogger struct {
	logger syncLogger

	// ctx is the context to use when uploading batch of entries.
	ctx context.Context

	wg    sync.WaitGroup
	tickc <-chan time.Time // receives from tickc trigger uploads.
	stopc chan struct{}    // closing stopc indicates that the BatchLogger should shut down.

	// retries groups log events by the number of retries remaining after the next upload attempt.
	retries *ringBuffer

	// pending is a list of log events that have not yet had an upload attempt.
	mu      sync.Mutex
	pending []*logpb.LogRequestLite_LogEventLite
}

type syncLogger interface {
	LogSync(context.Context, ...*logpb.LogRequestLite_LogEventLite) error
}

// NewBatchLogger constructs a new BatchLogger.
// The supplied context will be used when logging entries supplied via the Log method.
// Its Close method must be called when it is no longer needed.
func NewBatchLogger(ctx context.Context, logger *Logger, uploadTicker <-chan time.Time) *BatchLogger {
	return newBatchLogger(ctx, logger, uploadTicker)
}

func newBatchLogger(ctx context.Context, logger syncLogger, uploadTicker <-chan time.Time) *BatchLogger {
	bl := &BatchLogger{
		logger: logger,
		stopc:  make(chan struct{}),
		tickc:  uploadTicker,
	}

	bl.wg.Add(1)
	go func() {
		defer bl.wg.Done()
		for {
			select {
			case <-bl.tickc:
				bl.upload()
			case <-bl.stopc:
				return
			}
		}
	}()
	return bl
}

// Close flushes any pending logs and releases any resources held by the logger.
// Once Close has been returned, no more retry attempts will be made.
// Close should be called when the logger is no longer needed.
func (bl *BatchLogger) Close() {
	close(bl.stopc)
	bl.wg.Wait()

	// Final upload.
	// TODO(mcgreevy): flush outstanding events with exponential backoff.
	bl.upload()
}

// Log stages events to be logged to the eventlog service.
// The EventTime in each event must have been obtained from time.Now.
// Log returns immediately, and batches of events will be sent to the eventlog server periodically.
func (bl *BatchLogger) Log(events ...*logpb.LogRequestLite_LogEventLite) {
	bl.mu.Lock()
	defer bl.mu.Unlock()
	bl.pending = append(bl.pending, events...)
}

func (bl *BatchLogger) upload() {
	// TODO: split uploads into multiple requests if size threshold is exceeded.
	bl.mu.Lock()
	pending := bl.pending
	bl.pending = nil
	bl.mu.Unlock()

	if len(pending) == 0 && bl.retries == nil {
		return
	}

	// Normal case: nothing to retry.
	if bl.retries == nil {
		if err := bl.logger.LogSync(bl.ctx, pending...); ShouldRetry(err) {
			bl.retries = &ringBuffer{}
			bl.retries.Push(pending)
		}
		return
	}

	// get a slice of elements for which this is our final attempt.
	lastChance := bl.retries.Push(pending)
	toUpload := bl.retries.AppendAll(lastChance)

	err := bl.logger.LogSync(bl.ctx, toUpload...)
	if err == nil {
		bl.retries = nil
		return
	}

	if dropped := len(lastChance); dropped > 0 {
		log.Printf("eventlog: WARNING: retries exhausted. Dropping %d events.", dropped)
	}

	if !ShouldRetry(err) {
		bl.retries = nil
	}
}

// ring Buffer is a fixed capacity buffer. When at capacity, a Push into the buffer causes an element to be displaced.
type ringBuffer struct {
	buf [numRetries][]*logpb.LogRequestLite_LogEventLite
	i   int // the next place to insert.
}

// Push inserts a batch of entries into the ring buffer, and returns the batch which it displaces.
func (rb *ringBuffer) Push(entry []*logpb.LogRequestLite_LogEventLite) []*logpb.LogRequestLite_LogEventLite {
	displaced := rb.buf[rb.i]
	rb.buf[rb.i] = entry
	rb.i++
	if rb.i == len(rb.buf) {
		rb.i = 0
	}

	return displaced
}

// AppendAll appends the contents of all of the batches in the ring buffer to dst, returning the result.
func (rb *ringBuffer) AppendAll(dst []*logpb.LogRequestLite_LogEventLite) []*logpb.LogRequestLite_LogEventLite {
	for _, entry := range rb.buf {
		dst = append(dst, entry...)
	}
	return dst
}
