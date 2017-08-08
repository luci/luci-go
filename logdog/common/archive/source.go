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

// Package archive constructs a LogDog archive out of log stream components.
// Records are read from the stream and emitted as an archive.
package archive

import (
	"errors"
	"fmt"
	"time"

	"go.chromium.org/luci/common/proto/google"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/renderer"
)

// safeLogEntrySource wraps a renderer.Source and guarantees that all emitted
// log entries' index parameters are >= those of the previous entry.
//
// Any LogEntry that violates this will be silently discarded.
type safeLogEntrySource struct {
	renderer.Source
	*Manifest

	lastPrefixIndex uint64
	lastStreamIndex uint64
	lastSequence    uint64
	lastTimeOffset  time.Duration
}

func (s *safeLogEntrySource) NextLogEntry() (*logpb.LogEntry, error) {
	// Loop until we find a LogEntry to return.
	for {
		le, err := s.Source.NextLogEntry()
		if le != nil {
			// Ingest the log entry.
			if err := s.normalizeLogEntry(le); err != nil {
				s.logger().Warningf("Discarding out-of-order log stream entry %d: %v", le.StreamIndex, err)
				continue
			}
		} else if err == nil {
			err = errors.New("source returned nil LogEntry and no error")
		}
		return le, err
	}
}

func (s *safeLogEntrySource) normalizeLogEntry(le *logpb.LogEntry) error {
	// Calculate the time offset Duration once.
	timeOffset := google.DurationFromProto(le.TimeOffset)

	// Are any of our order constraints violated?
	switch {
	case le.PrefixIndex < s.lastPrefixIndex:
		return fmt.Errorf("prefix index is out of order (%d < %d)", le.PrefixIndex, s.lastPrefixIndex)
	case le.StreamIndex < s.lastStreamIndex:
		return fmt.Errorf("stream index is out of order (%d < %d)", le.StreamIndex, s.lastStreamIndex)
	case le.Sequence < s.lastSequence:
		return fmt.Errorf("sequence is out of order (%d < %d)", le.Sequence, s.lastSequence)
	}

	s.lastPrefixIndex = le.PrefixIndex
	s.lastStreamIndex = le.StreamIndex
	s.lastSequence = le.Sequence

	if timeOffset < s.lastTimeOffset {
		s.logger().Warningf("Adjusting out-of-order timestamp (%s < %s) for log stream entry %d.",
			timeOffset, s.lastTimeOffset, le.StreamIndex)

		le.TimeOffset = google.LoadDuration(le.TimeOffset, s.lastTimeOffset)
	} else {
		s.lastTimeOffset = timeOffset
	}
	return nil
}
