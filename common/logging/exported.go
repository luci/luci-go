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

import "context"

// SetError returns a context with its error field set.
func SetError(c context.Context, err error) context.Context {
	return SetField(c, ErrorKey, err)
}

// IsLogging tests whether the context is configured to log at the specified
// level.
//
// Individual Logger implementations are supposed to call this function when
// deciding whether to log the message.
func IsLogging(c context.Context, l Level) bool {
	return l >= GetLevel(c)
}

// Debugf is a shorthand method to call the current logger's Errorf method.
func Debugf(c context.Context, fmt string, args ...interface{}) {
	Get(c).LogCall(Debug, 1, fmt, args)
}

// Infof is a shorthand method to call the current logger's Errorf method.
func Infof(c context.Context, fmt string, args ...interface{}) {
	Get(c).LogCall(Info, 1, fmt, args)
}

// Warningf is a shorthand method to call the current logger's Errorf method.
func Warningf(c context.Context, fmt string, args ...interface{}) {
	Get(c).LogCall(Warning, 1, fmt, args)
}

// Errorf is a shorthand method to call the current logger's Errorf method.
func Errorf(c context.Context, fmt string, args ...interface{}) {
	Get(c).LogCall(Error, 1, fmt, args)
}

// Logf is a shorthand method to call the current logger's logging method which
// corresponds to the supplied log level.
func Logf(c context.Context, l Level, fmt string, args ...interface{}) {
	Get(c).LogCall(l, 1, fmt, args)
}
