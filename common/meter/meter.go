// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meter

import (
	"errors"

	"github.com/luci/luci-go/common/clock"
	log "github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

var (
	// ErrFull is an error returned by Add if the input queue is full.
	ErrFull = errors.New("meter: input queue is full")
)

// The default size of the work buffer.
const initialWorkBufferSize = 128

// An Meter accepts units of Work and clusters them based on a clustering
// configuration.
//
// The meter can cluster work by:
//   - Count: When the pool of work reaches a specified amount.
//   - Time: After a specified amount of time.
//
// The meter can also be triggered to purge externally via Flush.
//
// A Meter may be configured to discard work when it is being consumed too
// slowly. By default, the Meter will block input Work until output Work is
// consumed.
type Meter interface {
	// Add adds new Work to the Meter. If the Meter's ingest channel is full,
	// Add will return with ErrFull.
	Add(w interface{}) error

	// AddWait adds new Work to the Meter, blocking until the Work has been
	// enqueued.
	AddWait(w interface{})

	// Flush externally instructs the meter to dump any buffered work.
	Flush()

	// Stop shuts down the Meter, stopping further input processing and
	// blocking until the current enqueued work has finished.
	Stop()
}

// meter is a Meter implementation. An instance can be created via New().
type meter struct {
	*Config

	ctx       context.Context
	workC     chan interface{} // Channel to forward work into.
	flushNowC chan bool        // Structure to trigger a work dump.
	closeAckC chan struct{}    // Closed when consumeWork() finishes.
}

var _ Meter = (*meter)(nil)

// New instantiates and starts a new Meter.
func New(ctx context.Context, config Config) Meter {
	m := newImpl(ctx, &config)

	// This will run until "workC" is closed. When it finishes, it will close
	// "closeAckC".
	go m.consumeWork()

	return m
}

func newImpl(ctx context.Context, config *Config) *meter {
	if config.Callback == nil {
		panic("A callback must be provided.")
	}

	return &meter{
		Config: config,

		ctx:       ctx,
		workC:     make(chan interface{}, config.getAddBufferSize()),
		flushNowC: make(chan bool, 1),
		closeAckC: make(chan struct{}),
	}
}

func (m *meter) Add(w interface{}) error {
	select {
	case m.workC <- w:
		return nil

	default:
		return ErrFull
	}
}

func (m *meter) AddWait(w interface{}) {
	m.workC <- w
}

func (m *meter) Stop() {
	close(m.workC)
	<-m.closeAckC
}

func (m *meter) Flush() {
	select {
	case m.flushNowC <- true:
		break
	default:
		break
	}
}

// Main buffering function, which runs in a goroutine.
func (m *meter) consumeWork() {
	// Acknowledge when this goroutine finishes.
	defer close(m.closeAckC)

	ctx := log.SetFilter(m.ctx, "meter")

	timerRunning := false
	flushTimer := clock.NewTimer(ctx)
	defer func() {
		flushTimer.Stop()
	}()

	flushCount := m.Count
	flushTime := m.Delay

	// Build our work buffer.
	workBufferSize := initialWorkBufferSize
	if flushCount > 0 {
		// Will never buffer more than this much Work.
		workBufferSize = flushCount
	}
	bufferedWork := make([]interface{}, 0, workBufferSize)

	log.Debugf(ctx, "Starting work loop.")
	active := true
	for active {
		flushRound := false

		select {
		case work, ok := <-m.workC:
			if !ok {
				log.Debugf(ctx, "Received instance close signal; terminating.")

				// Don't immediately exit the loop, since there may still be buffered
				// Work to flush.
				active = false
				flushRound = true
				break
			}

			// Count the work in this group. If we're not using a given metric, try
			// and avoid wasting time collecting it.
			bufferedWork = append(bufferedWork, work)

			// Handle work count threshold. We do this first, since it's trivial to
			// setup/compute.
			if flushCount > 0 && len(bufferedWork) >= flushCount {
				flushRound = true
			}
			// Start our flush timer, if it's not already ticking. Only waste time
			// doing this if we're not already flushing, since flushing will clear the
			// timer.
			if !flushRound && flushTime > 0 && !timerRunning {
				log.Infof(log.SetFields(ctx, log.Fields{
					"flushInterval": flushTime,
				}), "Starting flush timer.")
				flushTimer.Reset(flushTime)
				timerRunning = true
			}

			// Invoke work callback, if one is set.
			if m.IngestCallback != nil {
				flushRound = m.IngestCallback(work) || flushRound
			}

		case <-m.flushNowC:
			flushRound = true

		case <-flushTimer.GetC():
			log.Infof(ctx, "Flush timer has triggered.")
			timerRunning = false
			flushRound = true
		}

		// Should we flush?
		if flushRound {
			flushTimer.Stop()
			timerRunning = false

			if len(bufferedWork) > 0 {
				// Clone bufferedWork, since we re-use it.
				workToSend := make([]interface{}, len(bufferedWork))
				copy(workToSend, bufferedWork)

				// Clear our Work slice for re-use. This does not resize the underlying
				// array, since it's likely to fill again.
				for idx := range bufferedWork {
					bufferedWork[idx] = nil
				}
				bufferedWork = bufferedWork[:0]

				// Callback with this work.
				m.Callback(workToSend)
			}
		}
	}
}
