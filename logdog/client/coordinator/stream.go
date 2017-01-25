// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package coordinator

import (
	"errors"
	"fmt"
	"time"

	"github.com/luci/luci-go/common/proto/google"
	"github.com/luci/luci-go/logdog/api/endpoints/coordinator/logs/v1"
	"github.com/luci/luci-go/logdog/api/logpb"
	"github.com/luci/luci-go/logdog/common/types"
	"github.com/luci/luci-go/luci_config/common/cfgtypes"

	"golang.org/x/net/context"
)

// StreamState represents the client-side state of the log stream.
//
// It is a type-promoted version of logdog.LogStreamState.
type StreamState struct {
	// Created is the time, represented as a UTC RFC3339 string, when the log
	// stream was created.
	Created time.Time
	// Updated is the time, represented as a UTC RFC3339 string, when the log
	// stream was last updated.
	Updated time.Time

	// TerminalIndex is the stream index of the log stream's terminal message. If
	// its value is <0, then the log stream has not terminated yet.
	// In this case, FinishedIndex is the index of that terminal message.
	TerminalIndex types.MessageIndex

	// Archived is true if the stream is marked as archived.
	Archived bool
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

// LogStream is returned metadata about a log stream.
type LogStream struct {
	// Project is the log stream's project.
	Project cfgtypes.ProjectName
	// Path is the path of the log stream.
	Path types.StreamPath

	// Desc is the log stream's descriptor.
	Desc logpb.LogStreamDescriptor

	// State is the stream's current state.
	State StreamState
}

func loadLogStream(proj string, path types.StreamPath, s *logdog.LogStreamState, d *logpb.LogStreamDescriptor) (
	*LogStream, error) {
	switch {
	case s == nil:
		return nil, errors.New("missing required log state")
	case d == nil:
		return nil, errors.New("missing required descriptor")
	}

	ls := LogStream{
		Project: cfgtypes.ProjectName(proj),
		Path:    path,
		Desc:    *d,
		State: StreamState{
			Created:       google.TimeFromProto(s.Created),
			TerminalIndex: types.MessageIndex(s.TerminalIndex),
			Purged:        s.Purged,
		},
	}
	if a := s.Archive; a != nil {
		ls.State.Archived = true
		ls.State.ArchiveIndexURL = a.IndexUrl
		ls.State.ArchiveStreamURL = a.StreamUrl
		ls.State.ArchiveDataURL = a.DataUrl
	}
	return &ls, nil
}

// Stream is an interface to Coordinator stream-level commands. It is bound to
// and operates on a single log stream path.
type Stream struct {
	// c is the Coordinator instance that this Stream is bound to.
	c *Client

	// project is this stream's project.
	project cfgtypes.ProjectName
	// path is the log stream's prefix.
	path types.StreamPath
}

// State fetches the LogStreamDescriptor for a given log stream.
func (s *Stream) State(ctx context.Context) (*LogStream, error) {
	req := logdog.GetRequest{
		Project:  string(s.project),
		Path:     string(s.path),
		State:    true,
		LogCount: -1, // Don't fetch any logs.
	}

	resp, err := s.c.C.Get(ctx, &req)
	if err != nil {
		return nil, normalizeError(err)
	}

	path := types.StreamPath(req.Path)
	if desc := resp.Desc; desc != nil {
		path = desc.Path()
	}

	st, err := loadLogStream(resp.Project, path, resp.State, resp.Desc)
	if err != nil {
		return nil, fmt.Errorf("failed to load stream state: %v", err)
	}
	return st, nil
}

// Get retrieves log stream entries from the Coordinator. The supplied
// parameters shape which entries are requested and what information is
// returned.
func (s *Stream) Get(ctx context.Context, params ...GetParam) ([]*logpb.LogEntry, error) {
	p := getParamsInst{
		r: logdog.GetRequest{
			Project: string(s.project),
			Path:    string(s.path),
		},
	}
	for _, param := range params {
		param.applyGet(&p)
	}

	if p.stateP != nil {
		p.r.State = true
	}

	resp, err := s.c.C.Get(ctx, &p.r)
	if err != nil {
		return nil, normalizeError(err)
	}
	if err := loadStatePointer(p.stateP, resp); err != nil {
		return nil, err
	}
	return resp.Logs, nil
}

// Tail performs a tail call, returning the last log entry in the stream. If
// stateP is not nil, the stream's state will be requested and loaded into the
// variable.
func (s *Stream) Tail(ctx context.Context, params ...TailParam) (*logpb.LogEntry, error) {
	p := tailParamsInst{
		r: logdog.TailRequest{
			Project: string(s.project),
			Path:    string(s.path),
		},
	}
	for _, param := range params {
		param.applyTail(&p)
	}

	resp, err := s.c.C.Tail(ctx, &p.r)
	if err != nil {
		return nil, normalizeError(err)
	}
	if err := loadStatePointer(p.stateP, resp); err != nil {
		return nil, err
	}

	switch len(resp.Logs) {
	case 0:
		return nil, nil

	case 1:
		le := resp.Logs[0]
		if p.complete {
			if dg := le.GetDatagram(); dg != nil && dg.Partial != nil {
				// This is a partial; datagram. Fetch and assemble the full datagram.
				return s.fetchFullDatagram(ctx, le, true)
			}
		}
		return le, nil

	default:
		return nil, fmt.Errorf("tail call returned %d logs", len(resp.Logs))
	}
}

func (s *Stream) fetchFullDatagram(ctx context.Context, le *logpb.LogEntry, fetchIfMid bool) (*logpb.LogEntry, error) {
	// Re-evaluate our partial state.
	dg := le.GetDatagram()
	if dg == nil {
		return nil, fmt.Errorf("entry is not a datagram")
	}

	p := dg.Partial
	if p == nil {
		// Not partial, return the full message.
		return le, nil
	}

	if uint64(p.Index) > le.StreamIndex {
		// Something is wrong. The datagram identifies itself as an index in the
		// stream that exceeds the actual number of entries in the stream.
		return nil, fmt.Errorf("malformed partial datagram; index (%d) > stream index (%d)",
			p.Index, le.StreamIndex)
	}

	if !p.Last {
		// This is the last log entry (b/c we Tail'd), but it is part of a larger
		// datagram. We can't fetch the full datagram since presumably the remainder
		// doesn't exist. Therefore, fetch the previous datagram.
		switch {
		case !fetchIfMid:
			return nil, fmt.Errorf("mid-fragment partial datagram not allowed")

		case uint64(p.Index) == le.StreamIndex:
			// If we equal the stream index, then we are the first datagram in the
			// stream, so return nil.
			return nil, nil

		default:
			// Perform a Get on the previous entry in the stream.
			prevIdx := le.StreamIndex - uint64(p.Index) - 1
			logs, err := s.Get(ctx, Index(types.MessageIndex(prevIdx)), LimitCount(1))
			if err != nil {
				return nil, fmt.Errorf("failed to get previous datagram (%d): %s", prevIdx, err)
			}

			if len(logs) != 1 || logs[0].StreamIndex != prevIdx {
				return nil, fmt.Errorf("previous datagram (%d) not returned", prevIdx)
			}
			if le, err = s.fetchFullDatagram(ctx, logs[0], false); err != nil {
				return nil, fmt.Errorf("failed to recurse to previous datagram (%d): %s", prevIdx, err)
			}
			return le, nil
		}
	}

	// If this is "Last", but it's also index 0, then it is a partial datagram
	// with one entry. Weird ... but whatever.
	if p.Index == 0 {
		dg.Partial = nil
		return le, nil
	}

	// Get the intermediate logs.
	startIdx := types.MessageIndex(le.StreamIndex - uint64(p.Index))
	count := int(p.Index)
	logs, err := s.Get(ctx, Index(startIdx), LimitCount(count))
	if err != nil {
		return nil, fmt.Errorf("failed to get intermediate logs [%d .. %d]: %s",
			startIdx, startIdx+types.MessageIndex(count)-1, err)
	}

	if len(logs) < count {
		return nil, fmt.Errorf("incomplete intermediate logs results (%d < %d)", len(logs), count)
	}
	logs = append(logs[:count], le)

	// Construct the full datagram.
	aggregate := make([]byte, 0, int(p.Size))
	for i, ple := range logs {
		chunkDg := ple.GetDatagram()
		if chunkDg == nil {
			return nil, fmt.Errorf("intermediate datagram #%d is not a datagram", i)
		}
		chunkP := chunkDg.Partial
		if chunkP == nil {
			return nil, fmt.Errorf("intermediate datagram #%d is not partial", i)
		}
		if int(chunkP.Index) != i {
			return nil, fmt.Errorf("intermediate datagram #%d does not have a contiguous index (%d)", i, chunkP.Index)
		}
		if chunkP.Size != p.Size {
			return nil, fmt.Errorf("inconsistent datagram size (%d != %d)", chunkP.Size, p.Size)
		}
		if uint64(len(aggregate))+uint64(len(chunkDg.Data)) > p.Size {
			return nil, fmt.Errorf("appending chunk data would exceed the declared size (%d > %d)",
				len(aggregate)+len(chunkDg.Data), p.Size)
		}
		aggregate = append(aggregate, chunkDg.Data...)
	}

	if uint64(len(aggregate)) != p.Size {
		return nil, fmt.Errorf("reassembled datagram length (%d) differs from declared length (%d)", len(aggregate), p.Size)
	}

	le = logs[0]
	dg = le.GetDatagram()
	dg.Data = aggregate
	dg.Partial = nil
	return le, nil
}

func loadStatePointer(stateP *LogStream, resp *logdog.GetResponse) error {
	if stateP == nil {
		return nil
	}

	ls, err := loadLogStream(resp.Project, resp.Desc.Path(), resp.State, resp.Desc)
	if err != nil {
		return fmt.Errorf("failde to load stream state: %v", err)
	}

	*stateP = *ls
	return nil
}
