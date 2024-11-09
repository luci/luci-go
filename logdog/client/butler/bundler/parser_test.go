// Copyright 2015 The LUCI Authors.
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

package bundler

import (
	"fmt"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/durationpb"

	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"

	"go.chromium.org/luci/logdog/api/logpb"
)

type testData struct {
	Data
	released bool
}

func (d *testData) Release() {
	d.released = true
}

func data(ts time.Time, b ...byte) *testData {
	return &testData{
		Data: &streamData{
			buffer: b,
			ts:     ts,
		},
	}
}

func dstr(ts time.Time, s string) *testData {
	return data(ts, []byte(s)...)
}

func shouldMatchLogEntry(t testing.TB, actual, expected *logpb.LogEntry) {
	t.Helper()
	assert.Loosely(t, actual, should.NotBeNil, truth.LineContext())
	assert.Loosely(t, expected, should.NotBeNil, truth.LineContext())
	assert.That(t, actual, should.Match(expected))
}

type parserTestStream struct {
	now         time.Time
	prefixIndex int64
	streamIndex int64
	offset      time.Duration
}

func (s *parserTestStream) base() baseParser {
	return baseParser{
		counter: &counter{
			current: s.prefixIndex,
		},
		timeBase: s.now,
	}
}

func (s *parserTestStream) next(d any) *logpb.LogEntry {
	le := &logpb.LogEntry{
		TimeOffset:  durationpb.New(s.offset),
		PrefixIndex: uint64(s.prefixIndex),
		StreamIndex: uint64(s.streamIndex),
	}

	switch t := d.(type) {
	case logpb.Text:
		le.Content = &logpb.LogEntry_Text{Text: &t}

	case logpb.Binary:
		le.Content = &logpb.LogEntry_Binary{Binary: &t}

	case logpb.Datagram:
		le.Content = &logpb.LogEntry_Datagram{Datagram: &t}

	default:
		panic(fmt.Errorf("unknown content type: %T", t))
	}

	s.prefixIndex++
	s.streamIndex++
	return le
}

func (s *parserTestStream) add(d time.Duration) *parserTestStream {
	s.offset += d
	return s
}

func (s *parserTestStream) le(seq int64, d any) *logpb.LogEntry {
	le := s.next(d)
	le.Sequence = uint64(seq)
	return le
}
