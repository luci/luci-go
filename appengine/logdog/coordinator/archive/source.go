// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package archive constructs a LogDog archive out of log stream components.
// Records are read from the stream and emitted as an archive.
package archive

import (
	"errors"
	"fmt"
	"time"

	"github.com/luci/luci-go/common/proto/logdog/logpb"
)

// ErrEndOfStream is a sentinel error returned by LogEntrySource to indicate
// that the end of the log stream has been encountered.
var ErrEndOfStream = errors.New("end of stream")

// A LogEntrySource returns ordered log entries for archival.
type LogEntrySource interface {
	// NextLogEntry returns the next sequential log entry. If there are no more
	// log entries, it should return ErrEndOfStream.
	NextLogEntry() (*logpb.LogEntry, error)
}

// safeLogEntrySource wraps a LogEntrySource and guarantees that all emitted log
// entries' index parameters are >= those of the previous entry.
//
// Any LogEntry that violates this will be silently discarded.
type safeLogEntrySource struct {
	LogEntrySource
	*Manifest

	lastPrefixIndex uint64
	lastStreamIndex uint64
	lastSequence    uint64
	lastTimeOffset  time.Duration
}

func (s *safeLogEntrySource) NextLogEntry() (*logpb.LogEntry, error) {
	// Loop until we find a LogEntry to return.
	for {
		le, err := s.LogEntrySource.NextLogEntry()
		if err != nil {
			return nil, err
		}
		if le == nil {
			return nil, errors.New("source returned nil LogEntry")
		}

		// Calculate the time offset Duration once.
		timeOffset := le.TimeOffset.Duration()

		// Are any of our order constraints violated?
		switch {
		case le.PrefixIndex < s.lastPrefixIndex:
			err = fmt.Errorf("prefix index is out of order (%d < %d)", le.PrefixIndex, s.lastPrefixIndex)
		case le.StreamIndex < s.lastStreamIndex:
			err = fmt.Errorf("stream index is out of order (%d < %d)", le.StreamIndex, s.lastStreamIndex)
		case le.Sequence < s.lastSequence:
			err = fmt.Errorf("sequence is out of order (%d < %d)", le.Sequence, s.lastSequence)
		case timeOffset < s.lastTimeOffset:
			err = fmt.Errorf("time offset is out of order (%s < %s)", timeOffset, s.lastTimeOffset)
		}
		if err != nil {
			s.logger().Warningf("Discarding out-of-order log stream entry %d: %v", le.StreamIndex, err)
			continue
		}

		s.lastPrefixIndex = le.PrefixIndex
		s.lastStreamIndex = le.StreamIndex
		s.lastSequence = le.Sequence
		s.lastTimeOffset = timeOffset

		return le, nil
	}
}
