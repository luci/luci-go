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

package fetcher

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/golang/protobuf/proto"
	"go.chromium.org/luci/common/clock"
	"go.chromium.org/luci/common/clock/testclock"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
	"go.chromium.org/luci/logdog/api/logpb"
	"go.chromium.org/luci/logdog/common/types"
)

type testSourceCommand struct {
	logEntries []*logpb.LogEntry

	err   error
	panic bool

	tidx    types.MessageIndex
	tidxSet bool
}

// addLogs adds a text log entry for each named indices. The text entry contains
// a single line, "#x", where "x" is the log index.
func (cmd *testSourceCommand) logs(indices ...int64) *testSourceCommand {
	for _, idx := range indices {
		cmd.logEntries = append(cmd.logEntries, &logpb.LogEntry{
			StreamIndex: uint64(idx),
		})
	}
	return cmd
}

func (cmd *testSourceCommand) terminalIndex(v types.MessageIndex) *testSourceCommand {
	cmd.tidx, cmd.tidxSet = v, true
	return cmd
}

func (cmd *testSourceCommand) error(err error, panic bool) *testSourceCommand {
	cmd.err, cmd.panic = err, panic
	return cmd
}

type testSource struct {
	sync.Mutex

	logs     map[types.MessageIndex]*logpb.LogEntry
	err      error
	panic    bool
	terminal types.MessageIndex

	history []int
}

func newTestSource() *testSource {
	return &testSource{
		terminal: -1,
		logs:     make(map[types.MessageIndex]*logpb.LogEntry),
	}
}

func (ts *testSource) Descriptor() *logpb.LogStreamDescriptor { return nil }

func (ts *testSource) LogEntries(c context.Context, req *LogRequest) ([]*logpb.LogEntry, types.MessageIndex, error) {
	ts.Lock()
	defer ts.Unlock()

	if ts.err != nil {
		if ts.panic {
			panic(ts.err)
		}
		return nil, 0, ts.err
	}

	// We have at least our next log. Build our return value.
	maxCount := req.Count
	if maxCount <= 0 {
		maxCount = len(ts.logs)
	}
	maxBytes := req.Bytes

	var logs []*logpb.LogEntry
	bytes := int64(0)
	index := req.Index
	for {
		if len(logs) >= maxCount {
			break
		}

		log, ok := ts.logs[index]
		if !ok {
			break
		}

		size := int64(5) // We've rigged all logs to have size 5.
		if len(logs) > 0 && maxBytes > 0 && (bytes+size) > maxBytes {
			break
		}
		logs = append(logs, log)
		index++
		bytes += size
	}
	ts.history = append(ts.history, len(logs))
	return logs, ts.terminal, nil
}

func (ts *testSource) send(cmd *testSourceCommand) {
	ts.Lock()
	defer ts.Unlock()

	if cmd.err != nil {
		ts.err, ts.panic = cmd.err, cmd.panic
	}
	if cmd.tidxSet {
		ts.terminal = cmd.tidx
	}
	for _, le := range cmd.logEntries {
		ts.logs[types.MessageIndex(le.StreamIndex)] = le
	}
}

func (ts *testSource) getHistory() []int {
	ts.Lock()
	defer ts.Unlock()

	h := make([]int, len(ts.history))
	copy(h, ts.history)
	return h
}

func loadLogs(f *Fetcher, count int) (result []types.MessageIndex, err error) {
	for {
		// Specific limit, hit that limit.
		if count > 0 && len(result) >= count {
			return
		}

		var le *logpb.LogEntry
		le, err = f.NextLogEntry()
		if le != nil {
			result = append(result, types.MessageIndex(le.StreamIndex))
		}
		if err != nil {
			return
		}
	}
}

func TestFetcher(t *testing.T) {
	t.Parallel()

	ftt.Run(`A testing log Source`, t, func(t *ftt.Test) {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		c, cancelFunc := context.WithCancel(c)

		ts := newTestSource()
		o := Options{
			Source: ts,

			// All message byte sizes will be 5.
			sizeFunc: func(proto.Message) int {
				return 5
			},
		}

		newFetcher := func() *Fetcher { return New(c, o) }
		reap := func(f *Fetcher) {
			cancelFunc()
			loadLogs(f, 0)
		}

		t.Run(`Uses defaults values when not overridden, and stops when cancelled.`, func(t *ftt.Test) {
			f := newFetcher()
			defer reap(f)

			assert.Loosely(t, f.o.BufferCount, should.BeZero)
			assert.Loosely(t, f.o.BufferBytes, should.Equal(DefaultBufferBytes))
			assert.Loosely(t, f.o.PrefetchFactor, should.Equal(1))
		})

		t.Run(`With a Count limit of 3.`, func(t *ftt.Test) {
			o.BufferCount = 3

			t.Run(`Will pull 6 sequential log records.`, func(t *ftt.Test) {
				var cmd testSourceCommand
				ts.send(cmd.logs(0, 1, 2, 3, 4, 5).terminalIndex(5))

				f := newFetcher()
				defer reap(f)

				logs, err := loadLogs(f, 0)
				assert.Loosely(t, err, should.Equal(io.EOF))
				assert.Loosely(t, logs, should.Resemble([]types.MessageIndex{0, 1, 2, 3, 4, 5}))
			})

			t.Run(`Will immediately bail out if RequireCompleteStream is set`, func(t *ftt.Test) {
				var cmd testSourceCommand
				ts.send(cmd.logs(0, 1, 2, 3, 4, 5).terminalIndex(-1))

				o.RequireCompleteStream = true
				f := newFetcher()
				defer reap(f)

				logs, err := loadLogs(f, 0)
				assert.Loosely(t, err, should.Equal(ErrIncompleteStream))
				assert.Loosely(t, logs, should.BeNil)
			})

			t.Run(`Can read two log records and be cancelled.`, func(t *ftt.Test) {
				var cmd testSourceCommand
				ts.send(cmd.logs(0, 1, 2, 3, 4, 5))

				f := newFetcher()
				defer reap(f)

				logs, err := loadLogs(f, 2)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, logs, should.Resemble([]types.MessageIndex{0, 1}))

				cancelFunc()
				_, err = loadLogs(f, 0)
				assert.Loosely(t, err, should.Equal(context.Canceled))
			})

			t.Run(`Will delay for more log records if none are available.`, func(t *ftt.Test) {
				delayed := false
				tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
					// Add the remaining logs.
					delayed = true

					var cmd testSourceCommand
					ts.send(cmd.logs(1, 2).terminalIndex(2))

					tc.Add(d)
				})

				var cmd testSourceCommand
				ts.send(cmd.logs(0))

				f := newFetcher()
				defer reap(f)

				logs, err := loadLogs(f, 1)
				assert.Loosely(t, err, should.BeNil)
				assert.Loosely(t, logs, should.Resemble([]types.MessageIndex{0}))

				logs, err = loadLogs(f, 0)
				assert.Loosely(t, err, should.Equal(io.EOF))
				assert.Loosely(t, logs, should.Resemble([]types.MessageIndex{1, 2}))
				assert.Loosely(t, delayed, should.BeTrue)
			})

			t.Run(`When an error is countered getting the terminal index, returns the error.`, func(t *ftt.Test) {
				var cmd testSourceCommand
				ts.send(cmd.error(errors.New("test error"), false))

				f := newFetcher()
				defer reap(f)

				_, err := loadLogs(f, 0)
				assert.Loosely(t, err, should.ErrLike("test error"))
			})

			t.Run(`When an error is countered fetching logs, returns the error.`, func(t *ftt.Test) {
				var cmd testSourceCommand
				ts.send(cmd.logs(0, 1, 2).error(errors.New("test error"), false))

				f := newFetcher()
				defer reap(f)

				_, err := loadLogs(f, 0)
				assert.Loosely(t, err, should.ErrLike("test error"))
			})

			t.Run(`If the source panics, it is caught and returned as an error.`, func(t *ftt.Test) {
				var cmd testSourceCommand
				ts.send(cmd.error(errors.New("test error"), true))

				f := newFetcher()
				defer reap(f)

				_, err := loadLogs(f, 0)
				assert.Loosely(t, err, should.ErrLike("panic during fetch"))
			})
		})

		t.Run(`With a byte limit of 15`, func(t *ftt.Test) {
			o.BufferBytes = 15
			o.PrefetchFactor = 2

			var cmd testSourceCommand
			ts.send(cmd.logs(0, 1, 2, 3, 4, 5, 6).terminalIndex(6))

			f := newFetcher()
			defer reap(f)

			// First fetch should have asked for 30 bytes (2*15), so 6 logs. After
			// first log was kicked, there is a deficit of one log.
			logs, err := loadLogs(f, 0)
			assert.Loosely(t, err, should.Equal(io.EOF))
			assert.Loosely(t, logs, should.Resemble([]types.MessageIndex{0, 1, 2, 3, 4, 5, 6}))

			assert.Loosely(t, ts.getHistory(), should.Resemble([]int{6, 1}))
		})

		t.Run(`With an index of 1 and a maximum count of 1, fetches exactly 1 log.`, func(t *ftt.Test) {
			o.Index = 1
			o.Count = 1

			var cmd testSourceCommand
			ts.send(cmd.logs(0, 1, 2, 3, 4, 5, 6).terminalIndex(6))

			f := newFetcher()
			defer reap(f)

			// First fetch will ask for exactly one log.
			logs, err := loadLogs(f, 0)
			assert.Loosely(t, err, should.Equal(io.EOF))
			assert.Loosely(t, logs, should.Resemble([]types.MessageIndex{1}))

			assert.Loosely(t, ts.getHistory(), should.Resemble([]int{1}))
		})
	})
}
