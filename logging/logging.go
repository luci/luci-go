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

// DefaultLogger is logger to use if no specialized logger is provided. In local
// environment it is Logrus logger, on GAE it is null logger (since GAE logger
// requires active appengine.Context).
var DefaultLogger Logger

// IsTerminal is true if current process is attached to an interactive terminal.
var IsTerminal bool

// Infof formats its arguments according to the format, analogous to fmt.Printf,
// and records the text as a log message at Info level.
func Infof(format string, args ...interface{}) { DefaultLogger.Infof(format, args...) }

// Warningf is like Infof, but logs at Warning level.
func Warningf(format string, args ...interface{}) { DefaultLogger.Warningf(format, args...) }

// Errorf is like Infof, but logs at Error level.
func Errorf(format string, args ...interface{}) { DefaultLogger.Errorf(format, args...) }
