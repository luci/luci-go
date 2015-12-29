// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memlogger

import (
	"fmt"
	"reflect"
	"sync"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/logging"
)

// LogEntry is a single entry in a MemLogger, containing a message and a
// severity.
type LogEntry struct {
	Level     logging.Level
	Msg       string
	Data      map[string]interface{}
	CallDepth int
}

// MemLogger is an implementation of Logger.
// Zero value is a valid logger.
type MemLogger struct {
	lock   *sync.Mutex
	data   *[]LogEntry
	fields map[string]interface{}
}

var _ logging.Logger = (*MemLogger)(nil)

// Debugf implements the logging.Logger interface.
func (m *MemLogger) Debugf(format string, args ...interface{}) {
	m.LogCall(logging.Debug, 1, format, args)
}

// Infof implements the logging.Logger interface.
func (m *MemLogger) Infof(format string, args ...interface{}) {
	m.LogCall(logging.Info, 1, format, args)
}

// Warningf implements the logging.Logger interface.
func (m *MemLogger) Warningf(format string, args ...interface{}) {
	m.LogCall(logging.Warning, 1, format, args)
}

// Errorf implements the logging.Logger interface.
func (m *MemLogger) Errorf(format string, args ...interface{}) {
	m.LogCall(logging.Error, 1, format, args)
}

// LogCall implements the logging.Logger interface.
func (m *MemLogger) LogCall(lvl logging.Level, calldepth int, format string, args []interface{}) {
	if m.lock != nil {
		m.lock.Lock()
		defer m.lock.Unlock()
	}
	if m.data == nil {
		m.data = new([]LogEntry)
	}
	*m.data = append(*m.data, LogEntry{lvl, fmt.Sprintf(format, args...), m.fields, calldepth + 1})
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

// Get returns the log entry iff the MemLogger contains the specified log message.
// Note that lvl, msg and data have to match the entry precisely.
func (m *MemLogger) Get(lvl logging.Level, msg string, data map[string]interface{}) *LogEntry {
	if m.lock != nil {
		m.lock.Lock()
		defer m.lock.Unlock()
	}
	if m.data == nil {
		return nil
	}
	for _, ent := range *m.data {
		if ent.Level == lvl && ent.Msg == msg && reflect.DeepEqual(data, ent.Data) {
			clone := ent
			return &clone
		}
	}
	return nil
}

// Has returns true iff the MemLogger contains the specified log message. Note
// that lvl, msg and data have to match the entry precisely.
func (m *MemLogger) Has(lvl logging.Level, msg string, data map[string]interface{}) bool {
	return m.Get(lvl, msg, data) != nil
}

// Use adds a memory backed Logger to Context, with concrete type
// *MemLogger. Casting to the concrete type can be used to inspect the
// log output after running a test case, for example.
func Use(c context.Context) context.Context {
	lock := sync.Mutex{}
	data := []LogEntry{}
	return logging.SetFactory(c, func(ic context.Context) logging.Logger {
		return &MemLogger{&lock, &data, logging.GetFields(ic)}
	})
}
