// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logs

import (
	"time"

	lep "github.com/luci/luci-go/appengine/logdog/coordinator/endpoints"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
)

// LogEntry is an endpoints-exported version of the logpb.LogEntry protobuf.
type LogEntry struct {
	// Timestamp is the timestamp of this LogEntry, expressed as an RFC3339
	// string.
	Timestamp string `json:"timestamp,omitempty"`

	// PrefixIndex is the message index within the Prefix.
	PrefixIndex int64 `json:"prefix_index,string,omitempty"`
	// StreamIndex is the message index within its Stream.
	StreamIndex int64 `json:"stream_index,string,omitempty"`
	// Sequence is the sequence number of the first content entry in this
	// LogEntry.
	Sequence int64 `json:"sequence,string,omitempty"`

	// Text is the data component of a text LogEntry.
	Text []string `json:"text,omitempty"`
	// Binary is the data component of a binary LogEntry.
	Binary []byte `json:"binary,omitempty"`
	// Datagram is the data component of a datagram LogEntry.
	Datagram *LogEntryDatagram `json:"datagram,omitempty"`
}

// LogEntryDatagramPartial describes a datagram's partial status. It is only
// populated if the datagram is a fragment.
type LogEntryDatagramPartial struct {
	// Index is this datagram fragment's index in the overall composite datagram.
	Index int32 `json:"index"`
	// The size in bytes of the overall datagram.
	Size int64 `json:"size,string,omitempty"`
	// Last, if true, means that this is the last datagram in the fragment.
	Last bool `json:"last,omitempty"`
}

// LogEntryDatagram is the data component of a datagram LogEntry.
type LogEntryDatagram struct {
	// Data is the datagram data.
	Data []byte `json:"data,omitempty"`
	// Partial, if not nil, marks this datagram as a fragment and describes its
	// partial fields.
	Partial *LogEntryDatagramPartial `json:"partial,omitempty"`
}

func logEntryFromProto(le *logpb.LogEntry, timeBase time.Time, newlines bool) *LogEntry {
	res := LogEntry{
		PrefixIndex: int64(le.PrefixIndex),
		StreamIndex: int64(le.StreamIndex),
		Sequence:    int64(le.Sequence),
	}
	if to := le.TimeOffset; to != nil {
		res.Timestamp = lep.ToRFC3339(timeBase.Add(to.Duration()))
	}

	if c := le.GetText(); c != nil {
		res.Text = make([]string, len(c.Lines))
		for i, line := range c.Lines {
			if newlines {
				res.Text[i] = line.Value + line.Delimiter
			} else {
				res.Text[i] = line.Value
			}
		}
	}
	if c := le.GetBinary(); c != nil {
		res.Binary = c.Data
	}
	if c := le.GetDatagram(); c != nil {
		res.Datagram = &LogEntryDatagram{
			Data: c.Data,
		}
		if cp := c.Partial; cp != nil {
			res.Datagram.Partial = &LogEntryDatagramPartial{
				Index: int32(cp.Index),
				Last:  cp.Last,
				Size:  int64(cp.Size),
			}
		}
	}
	return &res
}
