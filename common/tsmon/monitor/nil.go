// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package monitor

import (
	"github.com/luci/luci-go/common/tsmon/target"
	"github.com/luci/luci-go/common/tsmon/types"
)

type nilMonitor struct{}

// NewNilMonitor returns a Monitor that does nothing.
func NewNilMonitor() Monitor {
	return &nilMonitor{}
}

func (m *nilMonitor) ChunkSize() int {
	return 0
}

func (m *nilMonitor) Send(cells []types.Cell, t target.Target) error {
	return nil
}
