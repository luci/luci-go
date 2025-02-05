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

package memlogger

import (
	"context"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"sync"

	"go.chromium.org/luci/common/logging"
)

// LogEntry is a single entry in a MemLogger, containing a message and a
// severity.
type LogEntry struct {
	Level      logging.Level
	Msg        string
	Data       map[string]any
	StackTrace logging.StackTrace
	CallDepth  int
}

// MemLogger is an implementation of Logger.
// Zero value is a valid logger.
type MemLogger struct {
	lock *sync.Mutex
	data *[]LogEntry
	lctx *logging.LogContext
}

var _ logging.Logger = (*MemLogger)(nil)

// Debugf implements the logging.Logger interface.
func (m *MemLogger) Debugf(format string, args ...any) {
	m.LogCall(logging.Debug, 1, format, args)
}

// Infof implements the logging.Logger interface.
func (m *MemLogger) Infof(format string, args ...any) {
	m.LogCall(logging.Info, 1, format, args)
}

// Warningf implements the logging.Logger interface.
func (m *MemLogger) Warningf(format string, args ...any) {
	m.LogCall(logging.Warning, 1, format, args)
}

// Errorf implements the logging.Logger interface.
func (m *MemLogger) Errorf(format string, args ...any) {
	m.LogCall(logging.Error, 1, format, args)
}

// LogCall implements the logging.Logger interface.
func (m *MemLogger) LogCall(lvl logging.Level, calldepth int, format string, args []any) {
	if m.lock != nil {
		m.lock.Lock()
		defer m.lock.Unlock()
	}
	if m.data == nil {
		m.data = new([]LogEntry)
	}
	lctx := m.lctx
	if lctx == nil {
		lctx = &logging.LogContext{}
	}
	*m.data = append(*m.data, LogEntry{
		Level:      lvl,
		Msg:        fmt.Sprintf(format, args...),
		Data:       lctx.Fields,
		StackTrace: lctx.StackTrace,
		CallDepth:  calldepth + 1,
	})
}

// Messages returns all of the log messages that this memory logger has
// recorded.
func (m *MemLogger) Messages() []LogEntry {
	if m.lock != nil {
		m.lock.Lock()
		defer m.lock.Unlock()
	}
	if m.data == nil || len(*m.data) == 0 {
		return nil
	}
	ret := make([]LogEntry, len(*m.data))
	copy(ret, *m.data)
	return ret
}

// Reset resets the logged messages recorded so far.
func (m *MemLogger) Reset() {
	if m.lock != nil {
		m.lock.Lock()
		defer m.lock.Unlock()
	}
	if m.data != nil {
		*m.data = nil
	}
}

// GetFunc returns the first matching log entry.
func (m *MemLogger) GetFunc(f func(*LogEntry) bool) *LogEntry {
	if m.lock != nil {
		m.lock.Lock()
		defer m.lock.Unlock()
	}
	if m.data == nil {
		return nil
	}
	for _, ent := range *m.data {
		if f(&ent) {
			clone := ent
			return &clone
		}
	}
	return nil
}

// Get returns the first matching log entry.
// Note that lvl, msg and data have to match the entry precisely.
func (m *MemLogger) Get(lvl logging.Level, msg string, data map[string]any) *LogEntry {
	return m.GetFunc(func(ent *LogEntry) bool {
		return ent.Level == lvl && ent.Msg == msg && reflect.DeepEqual(data, ent.Data)
	})
}

// HasFunc returns true iff the MemLogger contains a matching log message.
func (m *MemLogger) HasFunc(f func(*LogEntry) bool) bool {
	return m.GetFunc(f) != nil
}

// Has returns true iff the MemLogger contains the specified log message.
// Note that lvl, msg and data have to match the entry precisely.
func (m *MemLogger) Has(lvl logging.Level, msg string, data map[string]any) bool {
	return m.Get(lvl, msg, data) != nil
}

// Dump dumps the current memory logger contents to the given writer in a
// human-readable format.
func (m *MemLogger) Dump(w io.Writer) (n int, err error) {
	write := func(msg string, args ...any) bool {
		var wrote int
		wrote, err = fmt.Fprintf(w, msg, args...)
		n += wrote
		return err == nil
	}
	for i, msg := range m.Messages() {
		if i == 0 {
			if !write("\nDUMP LOG:\n") {
				return
			}
		}
		if !write("  %s: %s", msg.Level, msg.Msg) {
			return
		}
		if msg.Data != nil {
			if !write(": %s", logging.Fields(msg.Data)) {
				return
			}
		}
		if stack := msg.StackTrace.ForTextLog(); stack != "" {
			if !write("\n") {
				return
			}
			for _, line := range strings.Split(stack, "\n") {
				if !write("    %s\n", line) {
					return
				}
			}
		} else {
			if !write("\n") {
				return
			}
		}
	}
	return
}

// LogTo writes each contained log to the given interface.
//
// Designed to be used with a `testing.TB` implementation:
//
//	if !check.Loosely(t, ml.Messages(), should.BeEmpty) {
//		ml.LogTo(t)
//		t.Fail()
//	}
//
// This will also detect and call the `Helper()` method, if `log` defines it.
func (m *MemLogger) LogTo(log interface{ Logf(fmt string, args ...any) }) {
	if h, ok := log.(interface{ Helper() }); ok {
		h.Helper()
	}

	for i, msg := range m.Messages() {
		if i == 0 {
			log.Logf("DUMP LOG:")
		}
		if msg.Data == nil {
			log.Logf("  %s: %s", msg.Level, msg.Msg)
		} else {
			log.Logf("  %s: %s: %s", msg.Level, msg.Msg, logging.Fields(msg.Data))
		}
	}
}

// Use adds a memory backed Logger to Context, with concrete type
// *MemLogger. Casting to the concrete type can be used to inspect the
// log output after running a test case, for example.
func Use(ctx context.Context) context.Context {
	lock := sync.Mutex{}
	data := []LogEntry{}
	return logging.SetFactory(ctx, func(ctx context.Context, lc *logging.LogContext) logging.Logger {
		return &MemLogger{
			lock: &lock,
			data: &data,
			lctx: lc,
		}
	})
}

// Reset is a convenience function to reset the current memory logger.
//
// This will panic if the current logger is not a memory logger.
func Reset(ctx context.Context) {
	logging.Get(ctx).(*MemLogger).Reset()
}

// Dump is a convenience function to dump the current contents of the memory
// logger to the writer.
//
// This will panic if the current logger is not a memory logger.
func Dump(ctx context.Context, w io.Writer) (n int, err error) {
	return logging.Get(ctx).(*MemLogger).Dump(w)
}

// MustDumpStdout is a convenience function to dump the current contents of the
// memory logger to stdout.
//
// This will panic if the current logger is not a memory logger.
func MustDumpStdout(ctx context.Context) {
	_, err := logging.Get(ctx).(*MemLogger).Dump(os.Stdout)
	if err != nil {
		panic(err)
	}
}
