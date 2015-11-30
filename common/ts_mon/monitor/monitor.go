// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package monitor contains the code for sending metric data to monitoring
// endpoints.
package monitor

import (
	"github.com/luci/luci-go/common/ts_mon/target"
	"github.com/luci/luci-go/common/ts_mon/types"
)

// AppengineCredentials and GCECredentials are special values that can be passed
// to monitors to indicate that app assertion credentials should be used instead
// of a real credentials file.
const (
	AppengineCredentials = ":appengine"
	GCECredentials       = ":gce"
)

// A Monitor can send a collection of metrics somewhere.
type Monitor interface {
	// ChunkSize returns the maximum number of metrics this Monitor can Send in
	// one go.  If there are more metrics, they will be broken up into chunks and
	// Send will be called multiple times.  A ChunkSize of 0 disables chunking.
	ChunkSize() int

	Send(cells []types.Cell, t target.Target) error
}
