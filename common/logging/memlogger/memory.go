// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memlogger

import (
	"fmt"
	"sync"

	"golang.org/x/net/context"

	"github.com/luci/luci-go/common/logging"
)

// LogLevel indicates the severity of a LogEntry.
type LogLevel uint

// 4 different log levels. These are automatically recorded in LogEntry by the
// various MemLogger.* methods.
const (
	LogError LogLevel = iota
	LogWarn
	LogInfo
	LogDebug
)

func (l LogLevel) String() string {
	switch l {
	case LogError:
		return "ERR"
	case LogWarn:
		return "WRN"
	case LogInfo:
		return "IFO"
	case LogDebug:
		return "DBG"
	default:
		return "???"
	}
}

// LogEntry is a single entry in a MemLogger, containing a message and a
// severity.
type LogEntry struct {
	Level LogLevel
	Msg   string
	Data  map[string]interface{}
}

// MemLogger is an implementation of Logger.
type MemLogger struct {
	lock   *sync.Mutex
	data   *[]LogEntry
	fields map[string]interface{}
}

var _ logging.Logger = (*MemLogger)(nil)

func (m *MemLogger) inner(lvl LogLevel, format string, args []interface{}) {
	m.lock.Lock()
	defer m.lock.Unlock()
	*m.data = append(*m.data, LogEntry{lvl, fmt.Sprintf(format, args...), m.fields})
}

func (m *MemLogger) Debugf(format string, args ...interface{})   { m.inner(LogDebug, format, args) }
func (m *MemLogger) Infof(format string, args ...interface{})    { m.inner(LogInfo, format, args) }
func (m *MemLogger) Warningf(format string, args ...interface{}) { m.inner(LogWarn, format, args) }
func (m *MemLogger) Errorf(format string, args ...interface{})   { m.inner(LogError, format, args) }

// Use adds a memory backed Logger to Context, with concrete type
// *MemLogger. Casting to the concrete type can be used to inspect the
// log output after running a test case, for example.
func Use(c context.Context) context.Context {
	lock := sync.Mutex{}
	data := []LogEntry{}
	return logging.SetFactory(c, func(ic context.Context) logging.Logger {
		return &MemLogger{&lock, &data, logging.FieldsToMap(logging.GetFields(ic))}
	})
}
