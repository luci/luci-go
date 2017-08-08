// Copyright 2016 The LUCI Authors.
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

package cli

import (
	"errors"
	"sync"
	"time"

	"go.chromium.org/luci/common/clock"
	log "go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/client/coordinator"
	"go.chromium.org/luci/logdog/common/fetcher"
	"go.chromium.org/luci/logdog/common/types"
	"golang.org/x/net/context"
)

const (
	// defaultBytes is the default maximum number of bytes to request per fetch
	// round.
	defaultBytes = 1024 * 1024 * 1 // 1 MB

	noStreamDelay = 5 * time.Second
)

// coordinatorSource is a fetcher.Source implementation that uses the
// Coordiantor API.
type coordinatorSource struct {
	sync.Mutex

	stream    *coordinator.Stream
	tidx      types.MessageIndex
	tailFirst bool

	streamState *coordinator.LogStream
}

func (s *coordinatorSource) LogEntries(c context.Context, req *fetcher.LogRequest) (
	[]*logpb.LogEntry, types.MessageIndex, error) {
	s.Lock()
	defer s.Unlock()

	params := append(make([]coordinator.GetParam, 0, 4),
		coordinator.LimitBytes(int(req.Bytes)),
		coordinator.LimitCount(req.Count),
		coordinator.Index(req.Index),
	)

	// If we haven't terminated, use this opportunity to fetch/update our stream
	// state.
	var streamState coordinator.LogStream
	reqStream := (s.streamState == nil || s.streamState.State.TerminalIndex < 0)
	if reqStream {
		params = append(params, coordinator.WithState(&streamState))
	}

	for {
		logs, err := s.stream.Get(c, params...)
		switch err {
		case nil:
			if reqStream {
				s.streamState = &streamState
				s.tidx = streamState.State.TerminalIndex
			}
			return logs, s.tidx, nil

		case coordinator.ErrNoSuchStream:
			log.WithError(err).Warningf(c, "Stream does not exist. Sleeping pending registration.")

			// Delay, interrupting if our Context is interrupted.
			if tr := <-clock.After(c, noStreamDelay); tr.Incomplete() {
				return nil, 0, tr.Err
			}

		default:
			if err != nil {
				return nil, 0, err
			}
		}
	}
}

func (s *coordinatorSource) descriptor() (*logpb.LogStreamDescriptor, error) {
	if s.streamState != nil {
		return &s.streamState.Desc, nil
	}
	return nil, errors.New("no stream state loaded")
}
