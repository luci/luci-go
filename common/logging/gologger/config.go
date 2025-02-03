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

package gologger

import (
	"context"
	"io"
	"os"
	"sync"

	gol "github.com/op/go-logging"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/system/terminal"
)

// StdFormat is a preferred logging format to use.
//
// It is compatible with logging format used by luci-py. The zero after %{pid}
// is "thread ID" which is unavailable in go.
const StdFormat = `[%{level:.1s}%{time:2006-01-02T15:04:05.000000Z07:00} ` +
	`%{pid} 0 %{shortfile}] %{message}`

// StdFormatWithColor is same as StdFormat, except with fancy colors.
//
// Use it when logging to terminal. Note that StdConfig will pick it
// automatically if it detects that given io.Writer is an os.File and it
// is a terminal. See PickStdFormat().
const StdFormatWithColor = `%{color}[%{level:.1s}%{time:2006-01-02T15:04:05.000000Z07:00} ` +
	`%{pid} 0 %{shortfile}]%{color:reset} %{message}`

// PickStdFormat returns StdFormat for non terminal-backed files or
// StdFormatWithColor for io.Writers that are io.Files backed by a terminal.
//
// Used by default StdConfig.
func PickStdFormat(w io.Writer) string {
	if file, _ := w.(*os.File); file != nil {
		if terminal.IsTerminal(int(file.Fd())) {
			return StdFormatWithColor
		}
	}
	return StdFormat
}

// StdConfig defines default logger configuration.
//
// It logs to Stderr using default logging format compatible with luci-py, see
// StdFormat.
//
// Call StdConfig.Use(ctx) to install it as a default context logger.
var StdConfig = LoggerConfig{Out: os.Stderr}

// LoggerConfig owns a go-logging logger, configured in some way.
//
// Despite its name it is not a configuration. It is a stateful object that
// lazy-initializes go-logging logger on a first use.
//
// If you are using os.File as Out, you are responsible for closing it when you
// are done with logging.
type LoggerConfig struct {
	Out    io.Writer // where to write the log to, required
	Format string    // how to format the log, default is PickStdFormat(Out)

	Logger *gol.Logger // if set, will be used as is, overrides everything else

	initOnce sync.Once
	w        *goLoggerWrapper
}

// NewLogger returns new go-logging based logger bound to the given logging
// context.
//
// It will use the logging level and fields specified in LogContext.
//
// lc.NewLogger is in fact logging.Factory and can be used in SetFactory. That's
// the reason it also takes context.Context, even though it isn't currently
// using it.
//
// All loggers produced by LoggerConfig share single underlying go-logging
// Logger instance.
func (lc *LoggerConfig) NewLogger(_ context.Context, lctx *logging.LogContext) logging.Logger {
	lc.initOnce.Do(func() {
		logger := lc.Logger
		if logger == nil {
			fmt := lc.Format
			if fmt == "" {
				fmt = PickStdFormat(lc.Out)
			}
			// Leveled formatted file backend.
			backend := gol.AddModuleLevel(
				gol.NewBackendFormatter(
					gol.NewLogBackend(lc.Out, "", 0),
					gol.MustStringFormatter(fmt)))
			backend.SetLevel(gol.DEBUG, "")
			logger = gol.MustGetLogger("")
			logger.SetBackend(backend)
		}
		lc.w = &goLoggerWrapper{l: logger}
	})
	ret := &loggerImpl{goLoggerWrapper: lc.w}
	ret.level = lctx.Level
	if len(lctx.Fields) > 0 {
		ret.fields = lctx.Fields.String()
	}
	return ret
}

// Use registers go-logging based logger as default logger of the context.
func (lc *LoggerConfig) Use(ctx context.Context) context.Context {
	return logging.SetFactory(ctx, lc.NewLogger)
}
