// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"errors"
	"time"

	"github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Flush sends all the metrics that are registered in the application.
func Flush(ctx context.Context) error {
	mon := Monitor()
	if mon == nil {
		return errors.New("no tsmon Monitor is configured")
	}

	// Split up the payload into chunks if there are too many cells.
	cells := Store().GetAll(ctx)

	chunkSize := mon.ChunkSize()
	if chunkSize == 0 {
		chunkSize = len(cells)
	}
	for len(cells) > 0 {
		count := minInt(chunkSize, len(cells))
		if err := mon.Send(cells[:count], Target); err != nil {
			return err
		}
		cells = cells[count:]
	}
	return nil
}

func autoFlush(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	logger := logging.Get(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := Flush(ctx); err != nil {
				logger.Warningf("Failed to flush tsmon metrics: %v", err)
			}
		}
	}
}
