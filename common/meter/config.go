// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meter

import (
	"time"
)

const (
	// The size of the work ingest channel (Add, AddWait), which holds work until
	// it's dequeued by the consumeWork loop.
	defaultAddBufferSize = 16
)

// Config is the set of meter configuration parameters.
type Config struct {
	// Count is the maximum number of work items to buffer before automatically
	// flushing.
	Count int
	// Delay is the maximum amount of time to wait after an element has been
	// buffered before automatically flushing it.
	Delay time.Duration

	// AddBufferSize is the size of the work ingest channel buffer. If zero, a
	// default size will be used.
	AddBufferSize int

	// Callback is the work callback method that is invoked with flushed Meter work.
	//
	// All callback invocations are made from the same goroutine, and thus
	// synchronous with each other. Therefore, any non-trivial work that needs to be
	// done based on the callback must be handled in a separate goroutine.
	Callback func([]interface{})

	// IngestCallback is a callback method that is invoked when the meter ingests
	// a new unit of work. If it returns true, a flush will be triggered. This is
	// intended to facilitate external accounting.
	//
	// All callback invocations are made from the same goroutine, and thus
	// synchronous with each other.
	IngestCallback func(interface{}) bool
}

// HasFlushConstraints tests if the current configuration applies any flushing
// constraints. A meter without flush constraints will flush after each message.
func (c *Config) HasFlushConstraints() bool {
	return (c.Count != 0 || c.Delay != 0)
}

// getAddBufferSize returns the configuration's AddBufferSize property, or the
// default work buffer size if the property value is zero.
func (c *Config) getAddBufferSize() int {
	if c.AddBufferSize == 0 {
		return defaultAddBufferSize
	}
	return c.AddBufferSize
}
