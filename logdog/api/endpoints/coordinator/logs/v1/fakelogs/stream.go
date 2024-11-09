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

package fakelogs

import (
	"bytes"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/clock"

	services "go.chromium.org/luci/logdog/api/endpoints/coordinator/services/v1"
	"go.chromium.org/luci/logdog/api/logpb"
	logdog_types "go.chromium.org/luci/logdog/common/types"
)

// Stream represents a single logdog stream.
//
// Each invocation of Write() will append a new LogEntry to the stream
// internally. For datagram streams, this means that each Write is a single
// datagram.
//
// Once the Stream is Close()'d it will be marked as complete.
type Stream struct {
	c *Client

	pth         logdog_types.StreamPath
	streamType  logpb.StreamType
	streamID    string
	secret      []byte
	prefixIndex *uint64
	start       time.Time

	mutex       sync.Mutex
	streamIndex int64
	sequence    uint64
}

func (s *Stream) Write(bs []byte) (int, error) {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	sIdx := s.streamIndex
	s.streamIndex++

	entry := &logpb.LogEntry{
		TimeOffset:  durationpb.New(s.start.Sub(clock.Now(s.c.ctx))),
		PrefixIndex: atomic.AddUint64(s.prefixIndex, 1) - 1, // 0 based
		StreamIndex: uint64(sIdx),
		Sequence:    s.sequence,
	}

	switch s.streamType {
	case logpb.StreamType_TEXT:
		rawLines := bytes.Split(bs, []byte("\n"))
		lines := make([]*logpb.Text_Line, len(rawLines))
		for i, line := range rawLines {
			lines[i] = &logpb.Text_Line{
				Value:     append([]byte(nil), line...),
				Delimiter: "\n",
			}
		}

		entry.Content = &logpb.LogEntry_Text{Text: &logpb.Text{
			Lines: lines,
		}}
		s.sequence += uint64(len(lines))

	case logpb.StreamType_BINARY:
		entry.Content = &logpb.LogEntry_Binary{Binary: &logpb.Binary{
			Data: bs,
		}}
		s.sequence += uint64(len(bs))

	case logpb.StreamType_DATAGRAM:
		entry.Content = &logpb.LogEntry_Datagram{Datagram: &logpb.Datagram{
			Data: bs,
		}}
		s.sequence++
	}

	s.c.storage.PutEntries(s.c.ctx, Project, s.pth, entry)

	return len(bs), nil
}

// Close terminates this stream.
func (s *Stream) Close() error {
	_, err := s.c.srvServ.TerminateStream(s.c.ctx, &services.TerminateStreamRequest{
		Project:       Project,
		Id:            s.streamID,
		Secret:        s.secret,
		TerminalIndex: s.streamIndex - 1,
	})
	return err
}
