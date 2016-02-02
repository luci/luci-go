// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gologger

import (
	"io"
	"os"

	"github.com/luci/luci-go/common/logging"
	gol "github.com/op/go-logging"
	"golang.org/x/net/context"
)

// StandardFormat first prints
// logging level, date+time, process ID, filename:linenumber, all colored.
// Then the message.
const StandardFormat = `%{color}[%{level:.1s} %{time:2006-01-02T15:04:05.000Z07:00}` +
	` %{pid} %{shortfile}]%{color:reset} %{message}`

var (
	// standardConfig is the LoggerConfig instance used by the package-level
	// methods.
	standardConfig = LoggerConfig{
		Format: StandardFormat,
		Out:    os.Stderr,
		Level:  gol.DEBUG,
	}
)

// New creates new logging.Logger backed by go-logging library. The new logger
// writes (to the provided file) messages of a given log level (or above).
// A caller is still responsible for closing the file when no longer needed.
func New(w io.Writer, level gol.Level) logging.Logger {
	lc := LoggerConfig{
		Format: standardConfig.Format,
		Out:    w,
		Level:  level,
	}
	return &loggerImpl{&goLoggerWrapper{l: lc.newGoLogger()}, nil}
}

// Get returns default global go-logging based logger. It writes >=DEBUG message
// to stderr. Use the Get method defined on LoggerConfig for non-default values.
func Get() logging.Logger {
	return standardConfig.getImpl()
}

// Use adds a default go-logging logger to the context.
func Use(c context.Context) context.Context {
	return standardConfig.Use(c)
}
