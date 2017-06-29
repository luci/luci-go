// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package storage

import (
	"github.com/luci/luci-go/common/errors"
	"github.com/luci/luci-go/logdog/api/logpb"
	"github.com/luci/luci-go/logdog/common/types"

	"github.com/golang/protobuf/proto"
)

// Entry is a logpb.LogEntry wrapper that lazily evaluates / unmarshals the
// underlying LogEntry data as needed.
//
// Entry is not goroutine-safe.
type Entry struct {
	D []byte

	streamIndex types.MessageIndex
	le          *logpb.LogEntry
}

// MakeEntry creates a new Entry.
//
// All Entry must be backed by data. The index, "idx", is optional. If <0, it
// will be calculated by unmarshalling the data into a LogEntry and pulling the
// value from there.
func MakeEntry(d []byte, idx types.MessageIndex) *Entry {
	return &Entry{
		D:           d,
		streamIndex: idx,
	}
}

// GetStreamIndex returns the LogEntry's stream index.
//
// If this needs to be calculated by unmarshalling the LogEntry, this will be
// done. If this fails, an error will be returned.
//
// If GetLogEntry has succeeded, subsequent GetStreamIndex calls will always
// be fast and succeed.
func (e *Entry) GetStreamIndex() (types.MessageIndex, error) {
	if e.streamIndex < 0 {
		le, err := e.GetLogEntry()
		if err != nil {
			return -1, err
		}

		e.streamIndex = types.MessageIndex(le.StreamIndex)
	}

	return e.streamIndex, nil
}

// GetLogEntry returns the unmarshalled LogEntry data.
//
// The first time this is called, the LogEntry will be unmarshalled from its
// underlying data.
func (e *Entry) GetLogEntry() (*logpb.LogEntry, error) {
	if e.le == nil {
		if e.D == nil {
			// This can happen with keys-only results.
			return nil, errors.New("no log entry data")
		}

		var le logpb.LogEntry
		if err := proto.Unmarshal(e.D, &le); err != nil {
			return nil, errors.Annotate(err, "failed to unmarshal").Err()
		}
		e.le = &le
	}

	return e.le, nil
}
