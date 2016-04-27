// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"errors"
	"time"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/logging"
)

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Flush sends all the metrics that are registered in the application.
func Flush(c context.Context) error {
	mon := Monitor(c)
	if mon == nil {
		return errors.New("no tsmon Monitor is configured")
	}

	// Run any callbacks that have been registered to populate values in callback
	// metrics.
	runCallbacks(c)

	cells := Store(c).GetAll(c)
	if len(cells) == 0 {
		return nil
	}

	logging.Debugf(c, "Starting tsmon flush: %d cells", len(cells))
	defer logging.Debugf(c, "Tsmon flush finished")

	// Split up the payload into chunks if there are too many cells.
	chunkSize := mon.ChunkSize()
	if chunkSize == 0 {
		chunkSize = len(cells)
	}

	total := len(cells)
	sent := 0
	for len(cells) > 0 {
		count := minInt(chunkSize, len(cells))
		if err := mon.Send(c, cells[:count]); err != nil {
			logging.Errorf(
				c, "Sent %d cells out of %d, skipping the rest due to error - %s",
				sent, total, err)
			return err
		}
		cells = cells[count:]
		sent += count
	}
	return nil
}

// autoFlusher knows how to periodically call 'Flush'.
type autoFlusher struct {
	killed chan struct{}
	cancel context.CancelFunc

	flush func(context.Context) error // mocked in unit tests
}

func (f *autoFlusher) start(c context.Context, interval time.Duration) {
	flush := f.flush
	if flush == nil {
		flush = Flush
	}

	// 'killed' is closed when timer goroutine exits.
	killed := make(chan struct{})
	f.killed = killed

	c, f.cancel = context.WithCancel(c)
	go func() {
		defer close(killed)

		for {
			if tr := <-clock.After(c, interval); tr.Incomplete() {
				return
			}
			if err := flush(c); err != nil && err != context.Canceled {
				logging.Warningf(c, "Failed to flush tsmon metrics: %v", err)
			}
		}
	}()
}

func (f *autoFlusher) stop() {
	f.cancel()
	<-f.killed
	f.cancel = nil
	f.killed = nil
}
