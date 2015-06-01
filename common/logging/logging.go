// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

/*
Package logging defines Logger interface and context.Context helpers to put\get
logger from context.Context.

Unfortunately standard library doesn't define any Logger interface (only
struct). And even worse: GAE logger is exposing different set of methods. Some
additional layer is needed to unify the logging. Package logging is intended to
be used from packages that support both local and GAE environments. Such
packages should not use global logger but must accept instances of Logger
interface (or even more generally context.Context) as parameters. Then callers
can pass appropriate Logger implementation  (or inject appropriate logger into
context.Context) depending on where the code is running.

Libraries under luci-go/common/ MUST use luci-go/common/logging instead of
directly instantiating concrete implementations.
*/
package logging

import (
	"fmt"
	"golang.org/x/net/context"
	"strings"
)

// Fields is a simple map of additional context values to add to Logger (see
// Logger.WithFields).
type Fields interface {
	fmt.Stringer

	// Each calls cb for each key/value pair contained by this Fields object.
	// Returning false from the callback will stop the iteration.
	Each(cb func(k string, v interface{}) bool)

	// Merge allows you to update a Fields instance with another one, returning
	// a new Fields object with the merged data. Keys in both Fields instances
	// will resolve by taking the value from 'other'. This does not mutate either
	// Fields instance. This method does not allow you to remove fields (e.g.
	// passing a key with a nil value will just cause the field to contain nil)
	Merge(other Fields) Fields

	// Len returns the number of fields currently contained by this Fields object.
	Len() int
}

// FieldsToMap returns a copy of the underlying data from a Fields. Returns nil
// if the Fields was nil, or if the Fields contained no data.
func FieldsToMap(f Fields) map[string]interface{} {
	if f == nil || f.Len() == 0 {
		return nil
	}
	return (map[string]interface{})(fields(nil).Merge(f).(fields))
}

func NewFields(v map[string]interface{}) Fields {
	return (fields(nil)).Merge(fields(v))
}

type fields map[string]interface{}

var _ Fields = fields(nil)

func (f fields) String() string {
	if f == nil {
		return "{}"
	}
	// strip away the `logging.Fields`{...}
	ret := fmt.Sprintf("%#v", f)
	return ret[strings.IndexRune(ret, '{'):]
}

func (f fields) Merge(other Fields) Fields {
	ret := make(fields, len(f)+other.Len())
	for k, v := range f {
		ret[k] = v
	}
	other.Each(func(k string, v interface{}) bool {
		ret[k] = v
		return true
	})
	return ret
}

func (f fields) Each(cb func(string, interface{}) bool) {
	for k, v := range f {
		if !cb(k, v) {
			break
		}
	}
}

func (f fields) Len() int {
	return len(f)
}

// Logger interface is ultimately implemented by underlying logging libraries
// (like go-logging or GAE logging). It is the least common denominator among
// logger implementations.
type Logger interface {
	// Debugf formats its arguments according to the format, analogous to
	// fmt.Printf and records the text as a log message at Debug level.
	Debugf(format string, args ...interface{})

	// Infof is like Debugf, but logs at Info level.
	Infof(format string, args ...interface{})

	// Warningf is like Debugf, but logs at Warning level.
	Warningf(format string, args ...interface{})

	// Errorf is like Debugf, but logs at Error level.
	Errorf(format string, args ...interface{})
}

type key int

var (
	loggerKey key = 0
	fieldsKey key = 1
)

// SetFactory sets the Logger factory for this context.
//
// The factory will be called each time Get(context) is used.
func SetFactory(c context.Context, f func(context.Context) Logger) context.Context {
	return context.WithValue(c, loggerKey, f)
}

// Set sets the logger for this context.
//
// It can be retrieved with Get(context).
func Set(c context.Context, l Logger) context.Context {
	return SetFactory(c, func(context.Context) Logger { return l })
}

// Get the current Logger, or a logger that ignores all messages if none
// is defined.
func Get(c context.Context) (ret Logger) {
	if f, ok := c.Value(loggerKey).(func(context.Context) Logger); ok {
		ret = f(c)
	}
	if ret == nil {
		ret = Null()
	}
	return
}

// SetFields adds the additional fields as context for the current Logger. The
// display of these fields depends on the implementation of the Logger. The
// new Logger will contain the combination of its current Fields, updated with
// the new ones (see Fields.UpdateWith). Specifying the new fields as nil will
// clear the currently set fields.
func SetFields(c context.Context, fields Fields) context.Context {
	return context.WithValue(c, fieldsKey, fields)
}

// SetField is a convenience method for SetFields for a single key/value
// pair.
func SetField(c context.Context, key string, value interface{}) context.Context {
	return SetFields(c, fields{key: value})
}

// GetFields returns the current Fields (used for logging implementations)
func GetFields(c context.Context) Fields {
	ret, _ := c.Value(fieldsKey).(Fields)
	if ret == nil {
		return fields(nil)
	}
	return ret
}

// Null returns logger that silently ignores all messages.
func Null() Logger {
	return nullLogger{}
}

// nullLogger silently ignores all messages.
type nullLogger struct{}

var _ Logger = nullLogger{}

func (nullLogger) Debugf(string, ...interface{})   {}
func (nullLogger) Infof(string, ...interface{})    {}
func (nullLogger) Warningf(string, ...interface{}) {}
func (nullLogger) Errorf(string, ...interface{})   {}
