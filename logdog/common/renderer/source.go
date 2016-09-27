// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package renderer

import (
	"io"

	"github.com/luci/luci-go/logdog/api/logpb"
)

// Source returns successive LogEntry records for processing.
type Source interface {
	// NextLogEntry returns the next successive LogEntry record to render, or an
	// error if it could not be retrieved.
	//
	// If there are no more log entries in the stream, NextLogEntry should return
	// io.EOF as an error.
	//
	// It is valid to return a LogEntry and an error (including io.EOF) at the
	// same time. The LogEntry will be processed before the error.
	NextLogEntry() (*logpb.LogEntry, error)
}

// StaticSource is a Source that returns successive log entries from a slice
// of log entries. The slice is mutated during processing.
type StaticSource []*logpb.LogEntry

var _ Source = (*StaticSource)(nil)

// NextLogEntry implements Source.
func (s *StaticSource) NextLogEntry() (le *logpb.LogEntry, err error) {
	// Pop the next entry, mutating s.
	if len(*s) > 0 {
		le, *s = (*s)[0], (*s)[1:]
	}
	if len(*s) == 0 {
		err = io.EOF
	}
	return
}
