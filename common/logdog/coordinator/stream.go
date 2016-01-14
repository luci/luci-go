// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"errors"
	"fmt"
	"time"

	"github.com/luci/luci-go/common/api/logdog_coordinator/logs/v1"
	"github.com/luci/luci-go/common/logdog/protocol"
	"github.com/luci/luci-go/common/logdog/types"
	"golang.org/x/net/context"
)

// StreamState represents the client-side state of the log stream.
//
// It is a type-promoted version of logs.LogStreamState.
type StreamState struct {
	// Created is the time, represented as a UTC RFC3339 string, when the log
	// stream was created.
	Created time.Time
	// Updated is the time, represented as a UTC RFC3339 string, when the log
	// stream was last updated.
	Updated time.Time

	// Descriptor is the log stream's descriptor message.
	Descriptor *protocol.LogStreamDescriptor

	// TerminalIndex is the stream index of the log stream's terminal message. If
	// its value is <0, then the log stream has not terminated yet.
	// In this case, FinishedIndex is the index of that terminal message.
	TerminalIndex types.MessageIndex

	// ArchiveIndexURL is the Google Storage URL where the log stream's index is
	// archived.
	ArchiveIndexURL string
	// ArchiveStreamURL is the Google Storage URL where the log stream's raw
	// stream data is archived. If this is not empty, the log stream is considered
	// archived.
	ArchiveStreamURL string
	// ArchiveDataURL is the Google Storage URL where the log stream's assembled
	// data is archived. If this is not empty, the log stream is considered
	// archived.
	ArchiveDataURL string

	// Purged indicates the purged state of a log. A log that has been purged is
	// only acknowledged to administrative clients.
	Purged bool
}

// loadLogStreamState converts a logs.LogStreamState into a StreamState.
func loadLogStreamState(s *logs.LogStreamState) (*StreamState, error) {
	ret := StreamState{
		TerminalIndex:    types.MessageIndex(s.TerminalIndex),
		ArchiveIndexURL:  s.ArchiveIndexURL,
		ArchiveStreamURL: s.ArchiveStreamURL,
		ArchiveDataURL:   s.ArchiveDataURL,
		Purged:           s.Purged,
	}

	err := error(nil)
	if ret.Created, err = parseLogStreamStateTimestamp(s.Created); err != nil {
		return nil, fmt.Errorf("failed to parse 'Created' timestamp: %v", err)
	}
	if ret.Updated, err = parseLogStreamStateTimestamp(s.Updated); err != nil {
		return nil, fmt.Errorf("failed to parse 'Updated' timestamp: %v", err)
	}

	return &ret, nil
}

func parseLogStreamStateTimestamp(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

// IsArchived returns true if any of the mandatory archival parameters is set.
func (s *StreamState) IsArchived() bool {
	return (s.ArchiveIndexURL != "" || s.ArchiveStreamURL != "")
}

// Stream is an interface to Coordinator stream-level commands. It is bound to
// and operates on a single log stream path.
type Stream struct {
	// c is the Coordinator instance that this Stream is bound to.
	c *Client

	// path is the log stream's prefix.
	path types.StreamPath
}

// Get retrieves log stream entries from the Coordinator. The supplied
// parameters shape which entries are requested and what information is
// returned.
func (s *Stream) Get(ctx context.Context, p *StreamGetParams) ([]*protocol.LogEntry, error) {
	if p == nil {
		p = &StreamGetParams{}
	}

	req := s.c.svc.Get().Path(string(s.path))

	logs := p.logs
	if p.wantsLogs {
		if len(logs) > 0 {
			req.Count(int64(len(logs)))
		}
		if p.bytes > 0 {
			req.Bytes(p.bytes)
		}
		if p.nonContiguous {
			req.Noncontiguous(true)
		}
		if p.index > 0 {
			req.Index(p.index)
		}
	} else {
		req.Count(-1)
	}
	if p.stateP != nil {
		req.State(true)
	}
	req.Proto(true)

	resp, err := req.Context(ctx).Do()
	if err != nil {
		return nil, normalizeError(err)
	}

	if err := loadStatePointer(p.stateP, resp); err != nil {
		return nil, err
	}

	logs = logs[:0]
	for i, gle := range resp.Logs {
		le, err := decodeLogEntry(gle)
		if err != nil {
			return nil, fmt.Errorf("failed to decode LogEntry %d: %v", i, err)
		}
		logs = append(logs, le)
	}

	return logs, nil
}

// Tail performs a tail call, returning the last log entry in the stream. If
// stateP is not nil, the stream's state will be requested and loaded into the
// variable.
func (s *Stream) Tail(ctx context.Context, stateP *StreamState) (*protocol.LogEntry, error) {
	req := s.c.svc.Get().Path(string(s.path))
	req.Tail(true)
	if stateP != nil {
		req.State(true)
	}
	req.Proto(true)

	resp, err := req.Context(ctx).Do()
	if err != nil {
		return nil, normalizeError(err)
	}

	if err := loadStatePointer(stateP, resp); err != nil {
		return nil, err
	}

	var le *protocol.LogEntry
	switch len(resp.Logs) {
	case 0:
		break

	case 1:
		le, err = decodeLogEntry(resp.Logs[0])
		if err != nil {
			return nil, err
		}

	default:
		return nil, fmt.Errorf("tail call returned %d logs", len(resp.Logs))
	}
	return le, nil
}

func loadStatePointer(stateP *StreamState, resp *logs.GetResponse) error {
	if stateP == nil {
		return nil
	}

	if resp.State == nil {
		return errors.New("Requested state was not returned")
	}
	state, err := loadLogStreamState(resp.State)
	if err != nil {
		return err
	}

	desc := protocol.LogStreamDescriptor{}
	if err := b64Decode(resp.DescriptorProto, &desc); err != nil {
		return fmt.Errorf("failed to base64-decode LogStreamDescriptor: %v", err)
	}
	state.Descriptor = &desc

	*stateP = *state
	return nil
}

func decodeLogEntry(gle *logs.GetLogEntry) (*protocol.LogEntry, error) {
	le := protocol.LogEntry{}
	if err := b64Decode(gle.Proto, &le); err != nil {
		return nil, err
	}
	return &le, nil
}
