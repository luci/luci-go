// Copyright 2015 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package coordinator

import (
	"github.com/luci/luci-go/logdog/api/endpoints/coordinator/logs/v1"
	"github.com/luci/luci-go/logdog/common/types"
)

// getParamsInst is an internal struct that accumulates a Get request and
// associated instructions from a series of iterative GetParam applications.
type getParamsInst struct {
	// r is the Get request to populate.
	r logdog.GetRequest

	// stateP is the stream state pointer. It is set by State, and, if supplied,
	// will cause the log request to return stream state.
	stateP *LogStream
}

// tailParamsInst is an internal struct that accumulates a Tail request and
// associated instructions from a series of iterative TailParam applications.
type tailParamsInst struct {
	// r is the Tail request to populate.
	r logdog.TailRequest

	// stateP is the stream state pointer. It is set by State, and, if supplied,
	// will cause the log request to return stream state.
	stateP *LogStream

	// complete instructs the tail call to fetch a complete entry, instead of just
	// the last log record.
	complete bool
}

// GetParam is a condition or parameter to apply to a Get request.
type GetParam interface {
	applyGet(p *getParamsInst)
}

// TailParam is a condition or parameter to apply to a Tail request.
type TailParam interface {
	applyTail(p *tailParamsInst)
}

type loadStateParam struct {
	stateP *LogStream
}

// WithState returns a Get/Tail parameter that loads the log stream's state into
// the supplied LogState pointer.
func WithState(stateP *LogStream) interface {
	GetParam
	TailParam
} {
	return &loadStateParam{stateP}
}

func (p *loadStateParam) applyGet(param *getParamsInst) {
	param.stateP = p.stateP
	param.r.State = (p.stateP != nil)
}

func (p *loadStateParam) applyTail(param *tailParamsInst) {
	param.stateP = p.stateP
	param.r.State = (p.stateP != nil)
}

type indexGetParam struct {
	index types.MessageIndex
}

// Index returns a stream Get parameter that causes the Get request to
// retrieve logs starting at the requested stream index instead of the default,
// zero.
func Index(i types.MessageIndex) GetParam { return &indexGetParam{i} }

func (p *indexGetParam) applyGet(param *getParamsInst) { param.r.Index = int64(p.index) }

type limitBytesGetParam struct {
	limit int
}

// LimitBytes applies a byte constraint to the returned logs. If the supplied
// limit is <= 0, then no byte constraint will be applied and the server will
// choose how many logs to return.
func LimitBytes(limit int) GetParam {
	if limit < 0 {
		limit = 0
	}
	return &limitBytesGetParam{limit}
}

func (p *limitBytesGetParam) applyGet(param *getParamsInst) { param.r.ByteCount = int32(p.limit) }

type limitCountGetParam struct {
	limit int
}

// LimitCount applies a count constraint to the returned logs. If the supplied
// limit is <= 0, then no count constraint will be applied and the server will
// choose how many logs to return.
func LimitCount(limit int) GetParam {
	if limit < 0 {
		limit = 0
	}
	return &limitCountGetParam{limit}
}

func (p *limitCountGetParam) applyGet(param *getParamsInst) { param.r.LogCount = int32(p.limit) }

type nonContiguousGetParam struct{}

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
func NonContiguous() GetParam { return nonContiguousGetParam{} }

func (nonContiguousGetParam) applyGet(param *getParamsInst) { param.r.NonContiguous = true }

type completeTailParam struct{}

// Complete instructs the Tail call to retrieve a complete record.
//
// If frgmented, the resulting record will be manufactured from its composite
// parts, and will not actually represent any single record in the log stream.
// The time offset, prefix and stream indices, sequence number, and content will
// be derived from the initial log entry in the composite set.
//
// If the log stream is a TEXT or BINARY stream, no behavior change will
// occur, and the last log record will be returned.
//
// If the log stream is a DATAGRAM stream and the Tail record is parked partial,
// additional log entries will be fetched via Get and the full log stream will
// be assembled. If the partial datagram entry is the "last" in its sequeence,
// the full datagram ending with it will be returned. If it's partial in the
// middle of a sequence, the previous complete datagram will be returned.
func Complete() TailParam { return completeTailParam{} }

func (completeTailParam) applyTail(param *tailParamsInst) { param.complete = true }
