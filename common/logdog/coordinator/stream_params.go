// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coordinator

import (
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/proto/logdog/logpb"
)

// StreamGetParams is an accumulating set of Stream Get request parameters.
type StreamGetParams struct {
	// index is the starting index, set by Index.
	index int64
	// nonContiguous is true if the user called NonContiguous.
	nonContiguous bool

	// logs is the user-supplied log slice to populate.
	logs []*logpb.LogEntry
	// bytes is the maximum number of bytes of log data to return. It is set by
	// Logs if a byte constraint is specified.
	bytes int64
	// wantsLogs is true if the user has called Logs. If wantsLogs is false, the
	// request's Count field will be set to "-1", indicating that the service
	// should not return any logs.
	wantsLogs bool

	// stateP is the stream state pointer. It is set by State, and, if supplied,
	// will cause the log request to return stream state.
	stateP *StreamState
}

// NewGetParams returns a new StreamGetParams instance.
func NewGetParams() *StreamGetParams {
	return &StreamGetParams{}
}

func (p *StreamGetParams) clone() *StreamGetParams {
	clone := *p
	return &clone
}

// Index returns a stream Get parameter that causes the Get request to
// retrieve logs starting at the requested stream index instead of the default,
// zero.
func (p *StreamGetParams) Index(i types.MessageIndex) *StreamGetParams {
	p = p.clone()
	p.index = int64(i)
	return p
}

// Logs indicates that the Get request should return log entries. The log
// entries will start at index 0 (overridable via Index) and return a series
// of sequential logs (overridable via NonContiguous) starting from that
// index.
//
// It is valid to send a Get request that asks only for the log stream's state.
// This is accomplished by not calling Logs.
//
// If no logs are available, the request will be successful and none will be
// returned.
//
// If a log slice is supplied, the maximum number of returned entries will be
// the length of the slice, and it will be returned from Get sized to the number
// of logs that were returned. Otherwise, a new log array will be allocated and
// no size constraint will be supplied.
//
// If bytes is >0, the request will return no more than that many bytes of log
// data, with a minimum of one log (regardless of its size) if available.
//
// If neither logs nor bytes is supplied, the server will choose how many logs
// to return.
func (p *StreamGetParams) Logs(logs []*logpb.LogEntry, bytes int) *StreamGetParams {
	p = p.clone()

	if len(logs) > 0 {
		p.logs = logs
	}

	p.bytes = int64(bytes)
	p.wantsLogs = true
	return p
}

// NonContiguous returns a stream Get parameter that causes the Get request
// to allow non-contiguous records to be returned. By default, only contiguous
// records starting from the specific Index will be returned.
//
// By default, a log stream will return only contiguous records starting at the
// requested index. For example, if a stream had: {0, 1, 2, 4, 5} and a request
// was made for index 0, Get will return {0, 1, 2}, for index 3 {}, and for
// index 4 {4, 5}.
//
// If NonContiguous is true, a request for 0 will return {0, 1, 2, 4, 5} and so
// on.
//
// Log entries generally should not be missing, but may be if either the logs
// are still streaming (since they can be ingested out of order) or if a data
// loss or corruption occurs.
func (p *StreamGetParams) NonContiguous() *StreamGetParams {
	p = p.clone()
	p.nonContiguous = true
	return p
}

// State returns a stream Get parameter that causes the Get request to return
// its stream state and log stream descriptor.
func (p *StreamGetParams) State(stateP *StreamState) *StreamGetParams {
	p = p.clone()
	p.stateP = stateP
	return p
}
