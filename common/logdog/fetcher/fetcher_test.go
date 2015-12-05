// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fetcher

import (
	"errors"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/luci/luci-go/common/clock"
	"github.com/luci/luci-go/common/clock/testclock"
	"github.com/luci/luci-go/common/logdog/protocol"
	"github.com/luci/luci-go/common/logdog/types"
	"github.com/luci/luci-go/common/testing/assertions"
	. "github.com/smartystreets/goconvey/convey"
	"golang.org/x/net/context"
)

type testSource struct {
	sync.Mutex

	logs    map[types.MessageIndex]*protocol.LogEntry
	logsErr error

	terminal    types.MessageIndex
	terminalErr error
}

func newTestSource() *testSource {
	return &testSource{
		terminal: -1,
		logs:     map[types.MessageIndex]*protocol.LogEntry{},
	}
}

func (ts *testSource) TerminalIndex(context.Context) (types.MessageIndex, error) {
	ts.Lock()
	defer ts.Unlock()

	if ts.terminalErr != nil {
		return 0, ts.terminalErr
	}
	return ts.terminal, nil
}

func (ts *testSource) LogEntries(c context.Context, req *LogRequest) error {
	ts.Lock()
	defer ts.Unlock()

	if ts.logsErr != nil {
		return ts.logsErr
	}

	count := 0
	index := req.Index
	for count < len(req.Logs) {
		log, ok := ts.logs[index]
		if !ok {
			break
		}

		req.Logs[count] = log
		index++
		count++
	}

	req.Logs = req.Logs[:count]
	return nil
}

func (ts *testSource) addLogEntry(entries ...*protocol.LogEntry) {
	ts.Lock()
	defer ts.Unlock()

	for _, le := range entries {
		ts.logs[types.MessageIndex(le.StreamIndex)] = le
	}
}

// addLogs adds a text log entry for each named indices. The text entry contains
// a single line, "#x", where "x" is the log index.
func (ts *testSource) addLogs(indices ...int64) {
	entries := make([]*protocol.LogEntry, len(indices))
	for i, idx := range indices {
		entries[i] = &protocol.LogEntry{
			StreamIndex: uint64(idx),
		}
	}
	ts.addLogEntry(entries...)
}

func (ts *testSource) setLogsError(err error) {
	ts.Lock()
	defer ts.Unlock()
	ts.logsErr = err
}

func (ts *testSource) setTerminal(idx types.MessageIndex) {
	ts.Lock()
	defer ts.Unlock()
	ts.terminal = idx
}

func (ts *testSource) setTerminalError(err error) {
	ts.Lock()
	defer ts.Unlock()
	ts.terminalErr = err
}

func loadLogs(f *Fetcher, count int) (result []types.MessageIndex, err error) {
	for {
		// Specific limit, hit that limit.
		if count > 0 && len(result) >= count {
			return
		}

		var le *protocol.LogEntry
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

	Convey(`A testing log Source`, t, func() {
		c, tc := testclock.UseTime(context.Background(), testclock.TestTimeLocal)
		c, cancelFunc := context.WithCancel(c)

		fs := newTestSource()
		o := Options{
			Source: fs,
		}

		// By default, when delaying advance time by that delay.
		tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			tc.Add(d)
		})

		Convey(`Uses defaults values when not overridden, and stops when cancelled.`, func() {
			f := New(c, o)
			defer f.Wait()

			So(f.o.Count, ShouldEqual, DefaultCount)
			So(f.o.Batch, ShouldEqual, DefaultBatch)
			So(f.o.Delay, ShouldEqual, DefaultDelay)

			// Stop our Fetcher prematurely.
			cancelFunc()
		})

		Convey(`With a Batch size of 2 and a Count of 3.`, func() {
			o.Batch = 2
			o.Count = 3
			f := New(c, o)
			defer func() {
				cancelFunc()
				f.Wait()
			}()

			Convey(`Will pull 6 sequential log records.`, func() {
				fs.setTerminal(5)
				fs.addLogs(0, 1, 2, 3, 4, 5)

				logs, err := loadLogs(f, 0)
				So(err, ShouldEqual, io.EOF)
				So(logs, ShouldResemble, []types.MessageIndex{0, 1, 2, 3, 4, 5})
			})

			Convey(`Can read two log records and be cancelled.`, func() {
				fs.addLogs(0, 1)

				logs, err := loadLogs(f, 2)
				So(err, ShouldBeNil)
				So(logs, ShouldResemble, []types.MessageIndex{0, 1})

				cancelFunc()
				_, err = loadLogs(f, 0)
				So(err, ShouldEqual, context.Canceled)
			})

			Convey(`Will delay for more log records if none are available.`, func() {
				delayed := false
				tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
					// Add the remaining logs.
					delayed = true
					fs.setTerminal(2)
					fs.addLogs(1, 2)

					tc.Add(d)
				})

				fs.addLogs(0)

				logs, err := loadLogs(f, 1)
				So(err, ShouldBeNil)
				So(logs, ShouldResemble, []types.MessageIndex{0})

				logs, err = loadLogs(f, 0)
				So(err, ShouldEqual, io.EOF)
				So(logs, ShouldResemble, []types.MessageIndex{1, 2})
				So(delayed, ShouldBeTrue)
			})

			Convey(`When an error is countered getting the terminal index, returns the error.`, func() {
				fs.setTerminalError(errors.New("test error"))

				_, err := loadLogs(f, 0)
				So(err, assertions.ShouldErrLike, "test error")
			})

			Convey(`When an error is countered fetching logs, returns the error.`, func() {
				fs.addLogs(0, 1, 2)
				fs.setLogsError(errors.New("test error"))

				_, err := loadLogs(f, 0)
				So(err, assertions.ShouldErrLike, "test error")
			})
		})
	})
}
