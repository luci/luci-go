// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package monitor

import (
	"os"

	"github.com/golang/protobuf/proto"
	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/tsmon/types"
	"golang.org/x/net/context"
)

type debugMonitor struct {
	path string
}

// NewDebugMonitor returns a Monitor that outputs metrics to a log, and
// optionally a file on disk.
func NewDebugMonitor(path string) Monitor {
	return &debugMonitor{
		path: path,
	}
}

func (m *debugMonitor) ChunkSize() int {
	return 0
}

func (m *debugMonitor) Send(ctx context.Context, cells []types.Cell) error {
	logger := logging.Get(ctx)
	collection := SerializeCells(cells)
	str := proto.MarshalTextString(collection)
	logger.Infof("Sending ts_mon metrics:\n%s", str)

	if m.path != "" {
		file, err := os.OpenFile(m.path, os.O_RDWR|os.O_APPEND|os.O_CREATE, 0664)
		if err != nil {
			return err
		}

		defer func() {
			if err := file.Close(); err != nil {
				logger.Errorf("Failed to close file %s: %v", m.path, err)
			}
		}()

		if _, err = file.WriteString(str); err != nil {
			return err
		}
	}

	return nil
}
