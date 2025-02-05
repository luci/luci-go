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
	"bytes"
	"fmt"
	"slices"
	"strings"
	"sync"

	gol "github.com/op/go-logging"

	"go.chromium.org/luci/common/logging"
)

const (
	logMessageFieldPadding = 44
)

// goLoggerWrapper is a synchronized wrapper around a go-logging Logger
// instance.
type goLoggerWrapper struct {
	sync.Mutex             // Lock around wrapped logger properties.
	l          *gol.Logger // Wrapped logger.
}

// loggerImpl implements logging.Logger. It optionally binds a goLoggerWrapper
// to a Context.
type loggerImpl struct {
	*goLoggerWrapper // The logger instance to log through.

	lctx   *logging.LogContext
	fields string
}

func (li *loggerImpl) Debugf(format string, args ...any) {
	li.LogCall(logging.Debug, 1, format, args)
}
func (li *loggerImpl) Infof(format string, args ...any) {
	li.LogCall(logging.Info, 1, format, args)
}
func (li *loggerImpl) Warningf(format string, args ...any) {
	li.LogCall(logging.Warning, 1, format, args)
}
func (li *loggerImpl) Errorf(format string, args ...any) {
	li.LogCall(logging.Error, 1, format, args)
}

func (li *loggerImpl) LogCall(l logging.Level, calldepth int, format string, args []any) {
	if l < li.lctx.Level {
		return
	}

	// Append the fields to the format string.
	if len(li.fields) > 0 {
		text := formatWithFields(format, li.fields, args)
		format = strings.Replace(text, "%", "%%", -1)
		args = nil
	}

	// Append the textual stack trace, if any.
	if stack := li.lctx.StackTrace.ForTextLog(); stack != "" {
		format += "\n\n%s"
		args = append(slices.Clone(args), stack)
	}

	li.Lock()
	defer li.Unlock()

	li.l.ExtraCalldepth = (calldepth + 1)
	switch l {
	case logging.Debug:
		li.l.Debugf(format, args...)
	case logging.Info:
		li.l.Infof(format, args...)
	case logging.Warning:
		li.l.Warningf(format, args...)
	case logging.Error:
		li.l.Errorf(format, args...)
	}
}

// formatWithFields renders the supplied format string, adding fields.
func formatWithFields(format string, fieldString string, args []any) string {
	buf := bytes.Buffer{}
	buf.Grow(len(format) + logMessageFieldPadding + len(fieldString))
	fmt.Fprintf(&buf, format, args...)

	padding := 44 - buf.Len()
	if padding < 1 {
		padding = 1
	}
	for i := 0; i < padding; i++ {
		buf.WriteString(" ")
	}
	buf.WriteString(fieldString)
	return buf.String()
}
