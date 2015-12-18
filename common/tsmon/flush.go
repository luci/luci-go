// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package tsmon

import (
	"errors"

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
	if Monitor == nil {
		return errors.New("no tsmon Monitor is configured")
	}

	// Split up the payload into chunks if there are too many cells.
	cells := Store.GetAll(ctx)
	chunkSize := Monitor.ChunkSize()

	if chunkSize == 0 {
		chunkSize = len(cells)
	}
	for len(cells) > 0 {
		count := minInt(chunkSize, len(cells))
		if err := Monitor.Send(cells[:count], Target); err != nil {
			return err
		}
		cells = cells[count:]
	}
	return nil
}
