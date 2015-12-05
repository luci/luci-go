// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fetcher

import (
	"io"
	"sync"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logdog/protocol"
	"github.com/luci/luci-go/common/logdog/types"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

const (
	// DefaultCount is the default number of LogEntry to prefetch and buffer.
	DefaultCount = 200

	// DefaultBatch is the default fetcher Batch value.
	DefaultBatch = 1024

	// DefaultDelay is the default Delay value.
	DefaultDelay = 5 * time.Second
)

// LogRequest is a structure used by the Fetcher to request a range of logs
// from its Source.
type LogRequest struct {
	Logs  []*protocol.LogEntry
	Index types.MessageIndex
}

// Source is the souce of log stream and log information.
type Source interface {
	// TerminalIndex returns the terminated index of the log stream.
	//
	// If the log stream isn't terminated, this should return a number that is
	// less than zero.
	TerminalIndex(context.Context) (types.MessageIndex, error)

	// LogEntries populates the supplied LogRequest with available sequential
	// log entries as available. This will block until at least one entry is
	// available.
	LogEntries(c context.Context, req *LogRequest) error
}

// Options is the set of configuration parameters for a Fetcher.
type Options struct {
	// Source is the log stream source.
	Source Source

	// StartIndex is the starting stream index to retrieve.
	StartIndex types.MessageIndex

	// Count is the target amount of LogEntry records to buffer. The Fetcher may
	// buffer more or fewer depending on availability. If more records are
	// available, they will not be fetched until the buffered amount drops below
	// this value.
	//
	// If zero, DefaultCount will be used.
	Count int

	// Batch is the number of LogEntry records to request from the Source in
	// a single request.
	//
	// If zero, DefaultBatch will be used.
	Batch int

	// Delay is the amount of time to wait in between unsuccessful log requests.
	Delay time.Duration
}

// A Fetcher buffers LogEntry records by querying the Source for log data.
// It attmepts to maintain a steady stream of records by prefetching available
// records in advance of consumption.
//
// A Fetcher is not goroutine-safe.
type Fetcher struct {
	// o is the set of Options.
	o *Options

	// ctx is the retained context. The Fetcher must respond to it being closed.
	ctx context.Context

	// errMu protects reading and writing to the err field.
	errMu sync.Mutex
	// err is the retained error state. If not nil, fetching has stopped and all
	// Fetcher methods will return this error.
	err error
	// errSignalC will be closed when err is set.
	errSignalC chan struct{}

	logC chan *protocol.LogEntry
	// doneC is a signal channel to indicate that the goroutine has finished.
	doneC chan struct{}
}

// New instantiates a new Fetcher instance.
//
// The Fetcher can be cancelled by cancelling the supplied context.
func New(ctx context.Context, o Options) *Fetcher {
	if o.Count <= 0 {
		o.Count = DefaultCount
	}
	if o.Batch <= 0 {
		o.Batch = DefaultBatch
	}
	if o.Delay <= 0 {
		o.Delay = DefaultDelay
	}

	f := Fetcher{
		o:          &o,
		ctx:        ctx,
		errSignalC: make(chan struct{}),
		logC:       make(chan *protocol.LogEntry, o.Count),
		doneC:      make(chan struct{}),
	}
	go f.fetch()
	return &f
}

func (f *Fetcher) getError() error {
	f.errMu.Lock()
	defer f.errMu.Unlock()
	return f.err
}

// NextLogEntry returns the next buffered LogEntry, blocking until it becomes
// available.
//
// If the end of the log stream is encountered, NextLogEntry will return
// io.EOF.
//
// If the Fetcher is cancelled, a context.Canceled error will be returned.
func (f *Fetcher) NextLogEntry() (*protocol.LogEntry, error) {
	select {
	case <-f.ctx.Done():
		return nil, f.ctx.Err()
	case <-f.errSignalC:
		return nil, f.getError()
	case le, ok := <-f.logC:
		if err := f.getError(); err != nil {
			return nil, err
		}
		if !ok {
			return nil, io.EOF
		}
		return le, nil
	}
}

// Wait blocks until the Fetcher has finished, either when it is cancelled or
// it has completed its log stream.
func (f *Fetcher) Wait() {
	<-f.doneC
}

// fetch is run in a separate goroutine. It continuously fetches LogEntry from
// the Source until the end of the stream is encountered or the context is
// cancelled.
func (f *Fetcher) fetch() {
	defer close(f.doneC)
	defer close(f.logC)

	if err := f.fetchImpl(); err != nil {
		// Set our error and signal that it's set.
		f.errMu.Lock()
		defer f.errMu.Unlock()

		f.err = err
		close(f.errSignalC)
	}
}

func (f *Fetcher) fetchImpl() error {
	tidx := types.MessageIndex(-1)
	nextIndex := f.o.StartIndex
	logs := make([]*protocol.LogEntry, f.o.Batch)

	for {
		// Test if the context has been cancelled.
		select {
		case <-f.ctx.Done():
			// We've been cancelled. There is no need to propagate the error, as
			// NextLogEntry will do its own check.
			return f.ctx.Err()
		default:
			// Not cancelled!
			break
		}

		// Request the next chunk of logs from our Source.
		req := LogRequest{
			Logs:  logs,
			Index: nextIndex,
		}
		if err := f.o.Source.LogEntries(f.ctx, &req); err != nil {
			log.Fields{
				log.ErrorKey: err,
				"index":      req.Index,
			}.Errorf(f.ctx, "Failed to get log stream data.")
			return err
		}

		// We will request the terminated index if the stream hasn't terminated yet.
		if tidx < 0 {
			i, err := f.o.Source.TerminalIndex(f.ctx)
			if err != nil {
				log.Fields{
					log.ErrorKey: err,
				}.Errorf(f.ctx, "Failed to get stream terminated index.")
				return err
			}

			tidx = i
		}

		// We will delay the next fetch if our log count is zero.
		delay := (len(req.Logs) == 0)

		for _, le := range req.Logs {
			leIndex := types.MessageIndex(le.StreamIndex)

			// If this log is beyond our terminal index, error.
			if tidx >= 0 && leIndex > tidx {
				log.Fields{
					"logIndex":      leIndex,
					"terminalIndex": tidx,
				}.Warningf(f.ctx, "Log beyond end of stream returned.")
				return nil
			}

			// Send the log to our caller. Note that this implicitly throttles our
			// log fetching, as when the buffer is full, we will block here until it
			// is emptied.
			log.Fields{
				"index": le.StreamIndex,
			}.Debugf(f.ctx, "Punting log entry.")
			f.logC <- le

			// If this is at or beyond our terminal log index, we're finished fetching.
			nextIndex = leIndex + 1
			if tidx >= 0 && tidx <= leIndex {
				break
			}
		}

		// If our next index is beyond our terminal log index, we're finished
		// fetching.
		if tidx >= 0 && tidx < nextIndex {
			return nil
		}

		// If we are configured to delay, do so before the next fetch.
		if delay {
			log.Fields{
				"delay": f.o.Delay,
			}.Debugf(f.ctx, "No more log records; sleeping.")
			select {
			case <-f.ctx.Done():
				// Cancelled while sleeping.
				return f.ctx.Err()

			case <-clock.After(f.ctx, f.o.Delay):
				break
			}
		}
	}
}
