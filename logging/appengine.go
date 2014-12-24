// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build appengine

package logging

type nullLogger struct{}

func (*nullLogger) Debugf(string, ...interface{})   {}
func (*nullLogger) Infof(string, ...interface{})    {}
func (*nullLogger) Warningf(string, ...interface{}) {}
func (*nullLogger) Errorf(string, ...interface{})   {}

func init() {
	DefaultLogger = &nullLogger{}
	IsTerminal = false
}
