// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package monitor

import (
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/tsmon/target"
	"github.com/luci/luci-go/common/tsmon/types"
)

type debugMonitor struct {
	logger logging.Logger
}

// NewDebugMonitor returns a Monitor that outputs metrics to a log, and
// optionally a file on disk.
//
// TODO(dsansome): Implement file logging.
func NewDebugMonitor(logger logging.Logger) Monitor {
	return &debugMonitor{
		logger: logger,
	}
}

func (m *debugMonitor) ChunkSize() int {
	return 0
}

func (m *debugMonitor) Send(cells []types.Cell, t target.Target) error {
	collection := serializeCells(cells, t)
	m.logger.Infof("Sending tsmon metrics:\n%s", collection.String())
	return nil
}
