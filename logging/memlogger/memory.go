// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package memlogger

import (
	"fmt"

	"golang.org/x/net/context"

	"infra/libs/logging"
)

// LogLevel indicates the severity of a LogEntry.
type LogLevel uint

// 4 different log levels. These are automatically recorded in LogEntry by the
// various MemLogger.* methods.
const (
	LogError LogLevel = iota
	LogWarn
	LogInfo
)

func (l LogLevel) String() string {
	switch l {
	case LogError:
		return "ERR"
	case LogWarn:
		return "WRN"
	case LogInfo:
		return "IFO"
	default:
		return "???"
	}
}

// LogEntry is a single entry in a MemLogger, containing a message and a
// severity.
type LogEntry struct {
	Level LogLevel
	Msg   string
}

// MemLogger is an implementation of Logger.
type MemLogger []LogEntry

// Infof adds a new LogEntry at the LogInfo level
func (m *MemLogger) Infof(format string, args ...interface{}) {
	*m = append(*m, LogEntry{LogInfo, fmt.Sprintf(format, args...)})
}

// Warningf adds a new LogEntry at the LogWarn level
func (m *MemLogger) Warningf(format string, args ...interface{}) {
	*m = append(*m, LogEntry{LogWarn, fmt.Sprintf(format, args...)})
}

// Errorf adds a new LogEntry at the LogError level
func (m *MemLogger) Errorf(format string, args ...interface{}) {
	*m = append(*m, LogEntry{LogError, fmt.Sprintf(format, args...)})
}

// Use adds a memory backed Logger to Context, with concrete type
// *MemLogger. Casting to the concrete type can be used to inspect the
// log output after running a test case, for example.
func Use(c context.Context) context.Context {
	ml := &MemLogger{}
	return logging.Set(c, func(ic context.Context) logging.Logger {
		return ml
	})
}
