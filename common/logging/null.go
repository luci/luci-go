// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package logging

import (
	"golang.org/x/net/context"
)

// Null returns a logger that silently ignores all messages.
func Null() Logger {
	return nullLogger{}
}

// NullFactory is a logging Factory implementation that returns the Null()
// Logger.
func NullFactory(context.Context) Logger {
	return Null()
}

// nullLogger silently ignores all messages.
type nullLogger struct{}

var _ Logger = nullLogger{}

func (nullLogger) Debugf(string, ...interface{})             {}
func (nullLogger) Infof(string, ...interface{})              {}
func (nullLogger) Warningf(string, ...interface{})           {}
func (nullLogger) Errorf(string, ...interface{})             {}
func (nullLogger) LogCall(Level, int, string, []interface{}) {}
