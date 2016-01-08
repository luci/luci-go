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

	"github.com/golang/protobuf/proto"
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

	logs  map[types.MessageIndex]*protocol.LogEntry
	err   error
	panic bool

	terminal types.MessageIndex
	maxCount int
	maxBytes int64

	history []int
}

func newTestSource() *testSource {
	return &testSource{
		terminal: -1,
		logs:     map[types.MessageIndex]*protocol.LogEntry{},
	}
}

func (ts *testSource) LogEntries(c context.Context, req *LogRequest) ([]*protocol.LogEntry, types.MessageIndex, error) {
	ts.Lock()
	defer ts.Unlock()

	if ts.err != nil {
		if ts.panic {
			panic(ts.err)
		}
		return nil, 0, ts.err
	}

	maxCount := req.Count
	if ts.maxCount > 0 && maxCount > ts.maxCount {
		maxCount = ts.maxCount
	}
	if maxCount <= 0 {
		maxCount = len(ts.logs)
	}

	maxBytes := req.Bytes
	if ts.maxBytes > 0 && maxBytes > ts.maxBytes {
		maxBytes = ts.maxBytes
	}

	var logs []*protocol.LogEntry
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

func (ts *testSource) setError(err error, panic bool) {
	ts.Lock()
	defer ts.Unlock()
	ts.err, ts.panic = err, panic
}

func (ts *testSource) setTerminal(idx types.MessageIndex) {
	ts.Lock()
	defer ts.Unlock()
	ts.terminal = idx
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

func newFetcher(c context.Context, o Options) *Fetcher {
	f := New(c, o)
	//
	// All message byte sizes will be 5.
	f.sizeFunc = func(proto.Message) int {
		return 5
	}
	return f
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

		reap := func(f *Fetcher) {
			cancelFunc()
			loadLogs(f, 0)
		}

		// By default, when delaying advance time by that delay.
		tc.SetTimerCallback(func(d time.Duration, t clock.Timer) {
			tc.Add(d)
		})

		Convey(`Uses defaults values when not overridden, and stops when cancelled.`, func() {
			f := newFetcher(c, o)
			defer reap(f)

			So(f.o.BufferCount, ShouldEqual, 0)
			So(f.o.BufferBytes, ShouldEqual, DefaultBufferBytes)
			So(f.o.PrefetchFactor, ShouldEqual, 1)
		})

		Convey(`With a Count limit of 3.`, func() {
			o.BufferCount = 3
			f := newFetcher(c, o)
			defer reap(f)

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
				fs.setError(errors.New("test error"), false)

				_, err := loadLogs(f, 0)
				So(err, assertions.ShouldErrLike, "test error")
			})

			Convey(`When an error is countered fetching logs, returns the error.`, func() {
				fs.addLogs(0, 1, 2)
				fs.setError(errors.New("test error"), false)

				_, err := loadLogs(f, 0)
				So(err, assertions.ShouldErrLike, "test error")
			})

			Convey(`If the source panics, it is caught and returned as an error.`, func() {
				fs.setError(errors.New("test error"), true)

				_, err := loadLogs(f, 0)
				So(err, assertions.ShouldErrLike, "panic during fetch")
			})
		})

		Convey(`With a byte limit of 15`, func() {
			o.BufferBytes = 15
			o.PrefetchFactor = 2
			f := newFetcher(c, o)
			defer reap(f)

			fs.setTerminal(6)
			fs.addLogs(0, 1, 2, 3, 4, 5, 6)

			// First fetch should have asked for 30 bytes (2*15), so 6 logs. After
			// first log was kicked, there is a deficit of one log.
			logs, err := loadLogs(f, 0)
			So(err, ShouldEqual, io.EOF)
			So(logs, ShouldResemble, []types.MessageIndex{0, 1, 2, 3, 4, 5, 6})

			So(fs.getHistory(), ShouldResemble, []int{6, 1})
		})

		Convey(`With an index of 1 and a maximum count of 1, fetches exactly 1 log.`, func() {
			o.Index = 1
			o.Count = 1
			f := newFetcher(c, o)
			defer reap(f)

			fs.setTerminal(6)
			fs.addLogs(0, 1, 2, 3, 4, 5, 6)

			// First fetch will ask for exactly one log.
			logs, err := loadLogs(f, 0)
			So(err, ShouldEqual, io.EOF)
			So(logs, ShouldResemble, []types.MessageIndex{1})

			So(fs.getHistory(), ShouldResemble, []int{1})
		})
	})
}
