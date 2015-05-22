// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

/*
Package logging defines Logger interface with implementations on top of Logrus
library and Appengine context. Unfortunately standard library doesn't define
any Logger interface (only struct). And even worse: GAE logger is exposing
different set of methods. Some additional layer is needed to unify the logging.

Package logging is intended to be used from packages that support both local and
GAE environments. Such packages should not use global logger but must accept
instances of Logger interface as parameters in functions. Then callers can pass
logrus.Logger or appengine.Context depending on where the code is running.

Libraries under infra/libs/* MUST use infra/libs/logger instead of directly
calling to Logrus library.
*/
package logging

import (
	"golang.org/x/net/context"
)

// Logger is interface implemented by both logrus.Logger and appengine.Context,
// and thus it can be used in libraries that expect to be called from both kinds
// of environments.
type Logger interface {
	// Infof formats its arguments according to the format, analogous to fmt.Printf,
	// and records the text as a log message at Info level.
	Infof(format string, args ...interface{})

	// Warningf is like Infof, but logs at Warning level.
	Warningf(format string, args ...interface{})

	// Errorf is like Infof, but logs at Error level.
	Errorf(format string, args ...interface{})
}

type key int

var loggerKey key

// Set sets the Logger factory for this context.
//
// The current Logger can be retrieved with Get(context)
func Set(c context.Context, f func(context.Context) Logger) context.Context {
	return context.WithValue(c, loggerKey, f)
}

// Get the current Logger, or a nullLogger if none is defined.
func Get(c context.Context) (ret Logger) {
	if f, ok := c.Value(loggerKey).(func(context.Context) Logger); ok {
		ret = f(c)
	}
	if ret == nil {
		ret = nullLogger{}
	}
	return
}

// IsSet returns a bool indicating if the context contains a Logger
// implementation.
func IsSet(c context.Context) bool {
	return c.Value(loggerKey) != nil
}
