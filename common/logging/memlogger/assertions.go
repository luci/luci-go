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
	"reflect"
	"strings"

	"go.chromium.org/luci/common/logging"
)

// ShouldHaveLog is a goconvey custom assertion which asserts that the logger has
// received a log. It takes up to 3 arguments.
//
// `actual` should either be a *MemLogger or a context.Context containing
// a *MemLogger.
//
// argument 1 (expected[0]) is the log level.
// argument 2 (expected[1]) is a substring the message. If omitted or the empty string, this value is not checked.
// argument 3 (expected[2]) is the fields data. If omitted or nil, this value is not checked.
func ShouldHaveLog(actual any, expected ...any) string {
	var ok bool
	var m *MemLogger

	switch x := actual.(type) {
	case *MemLogger:
		m = x
	case context.Context:
		if m, ok = logging.Get(x).(*MemLogger); !ok {
			return "context does not contain a *MemLogger"
		}
	default:
		return fmt.Sprintf("actual value must be a *MemLogger or context.Context, not %T", actual)
	}

	level := logging.Level(0)
	msg := ""
	data := map[string]any(nil)

	switch len(expected) {
	case 3:

		data, ok = expected[2].(map[string]any)
		if !ok {
			fields, ok := expected[2].(logging.Fields)
			if !ok {
				return fmt.Sprintf(
					"Third argument to this assertion must be an map[string]any (was %T)", expected[2])
			}
			data = fields
		}
		fallthrough
	case 2:

		msg, ok = expected[1].(string)
		if !ok {
			return fmt.Sprintf(
				"Second argument to this assertion must be a string (was %T)", expected[1])
		}
		fallthrough
	case 1:
		level, ok = expected[0].(logging.Level)
		if !ok {
			return "First argument to this assertion must be a logging.Level"
		}

	default:
		return fmt.Sprintf(
			"This assertion requires at least 1 comparison value (you provided %d)", len(expected))
	}

	predicate := func(e *LogEntry) bool {
		switch {
		case e.Level != level:
			return false

		case msg != "" && !strings.Contains(e.Msg, msg):
			return false

		case data != nil && !reflect.DeepEqual(data, e.Data):
			return false

		default:
			return true
		}
	}
	if m.HasFunc(predicate) {
		return ""
	}

	logString := fmt.Sprintf("Level %s", level)
	if msg != "" {
		logString += fmt.Sprintf(" message '%s'", msg)
	}

	if data != nil {
		logString += fmt.Sprintf(" data '%v'", data)
	}

	return fmt.Sprintf("No log matching %s", logString)
}

// ShouldNotHaveLog is the inverse of ShouldHaveLog. It asserts that the logger
// has not seen such a log.
func ShouldNotHaveLog(actual any, expected ...any) string {
	res := ShouldHaveLog(actual, expected...)

	if res != "" {
		return ""
	}

	return "Found a log, but wasn't supposed to."
}
