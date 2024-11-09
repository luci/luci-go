// Copyright 2017 The LUCI Authors.
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

package coordinator

import (
	"context"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	log "go.chromium.org/luci/common/logging"

	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/fetcher"
	"go.chromium.org/luci/logdog/common/types"
)

// coordinatorSource is a fetcher.Source implementation that uses the
// Coordinator API.
type coordinatorSource struct {
	sync.Mutex

	stream *Stream
	tidx   types.MessageIndex

	requireCompleteStream bool

	streamState *LogStream
}

func (s *coordinatorSource) LogEntries(c context.Context, req *fetcher.LogRequest) (
	[]*logpb.LogEntry, types.MessageIndex, error) {
	s.Lock()
	defer s.Unlock()

	params := append(make([]GetParam, 0, 4),
		LimitBytes(int(req.Bytes)),
		LimitCount(req.Count),
		Index(req.Index),
	)

	// If we haven't terminated, use this opportunity to fetch/update our stream
	// state.
	var streamState LogStream
	reqStream := (s.streamState == nil || s.streamState.State.TerminalIndex < 0)
	if reqStream {
		params = append(params, WithState(&streamState))
	}

	delayTimer := clock.NewTimer(c)
	defer delayTimer.Stop()
	for {
		logs, err := s.stream.Get(c, params...)

		// TODO(iannucci,dnj): use retry module + transient tags instead
		delayTimer.Reset(5 * time.Second)
		switch err {
		case nil:
			if reqStream {
				s.streamState = &streamState
				s.tidx = streamState.State.TerminalIndex
			}
			return logs, s.tidx, nil

		case ErrNoSuchStream:
			if s.requireCompleteStream {
				return nil, 0, err
			}

			log.WithError(err).Warningf(c, "Stream does not exist. Sleeping pending registration.")

			// Delay, interrupting if our Context is interrupted.
			if tr := <-delayTimer.GetC(); tr.Incomplete() {
				return nil, 0, tr.Err
			}

		default:
			if err != nil {
				return nil, 0, err
			}
		}
	}
}

// Descriptor returns the LogStreamDescriptor for this stream, if known,
// or returns nil.
func (s *coordinatorSource) Descriptor() *logpb.LogStreamDescriptor {
	if s.streamState != nil {
		return &s.streamState.Desc
	}
	return nil
}

// Fetcher returns a Fetcher implementation for this Stream.
//
// If you pass a nil fetcher.Options, a default option set will be used. The
// o.Source field will always be overwritten to be based off this stream.
func (s *Stream) Fetcher(c context.Context, o *fetcher.Options) *fetcher.Fetcher {
	if o == nil {
		o = &fetcher.Options{}
	} else {
		o = &(*o)
	}
	o.Source = &coordinatorSource{
		stream: s, tidx: -1, requireCompleteStream: o.RequireCompleteStream}
	return fetcher.New(c, *o)
}
