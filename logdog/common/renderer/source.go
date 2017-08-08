// Copyright 2015 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package renderer

import (
	"io"

	"go.chromium.org/luci/logdog/api/logpb"
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
