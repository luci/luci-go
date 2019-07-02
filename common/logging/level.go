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

package logging

import (
	"context"
	"errors"
	"flag"
)

// Level is an enumeration consisting of supported log levels.
type Level int

// Level implements flag.Value.
var _ flag.Value = (*Level)(nil)

// Defined log levels.
const (
	Debug Level = iota
	Info
	Warning
	Error
)

// DefaultLevel is the default Level value.
const DefaultLevel = Info

// Set implements flag.Value.
func (l *Level) Set(v string) error {
	switch v {
	case "debug":
		*l = Debug
	case "info":
		*l = Info
	case "warning":
		*l = Warning
	case "error":
		*l = Error

	default:
		return errors.New("unknown log level value")
	}
	return nil
}

// String implements flag.Value.
func (l Level) String() string {
	switch l {
	case Debug:
		return "debug"
	case Info:
		return "info"
	case Warning:
		return "warning"
	case Error:
		return "error"

	default:
		return "unknown"
	}
}

// SetLevel returns a new context with the given logging level.
//
// It can be retrieved with GetLevel(context).
func SetLevel(c context.Context, l Level) context.Context {
	return context.WithValue(c, levelKey, l)
}

// GetLevel returns the Level for this context. It will return DefaultLevel if
// none is defined.
func GetLevel(c context.Context) Level {
	if l, ok := c.Value(levelKey).(Level); ok {
		return l
	}
	return DefaultLevel
}
