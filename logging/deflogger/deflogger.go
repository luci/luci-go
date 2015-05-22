// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package deflogger provides the ability to inject a 'default' Logger
// implementation, based on the current platform. It's currently specialized
// for appengine and non-appengine, with the appengine implementation just
// falling back to the null Logger.
//
// For appengine apps, you'll want to use gaelogger.Use to add a real Logger
// on appengine.
package deflogger

import (
	"infra/libs/logging"

	"golang.org/x/net/context"
)

// Get returns the default logger, as you would get with a completely empty
// context.
func Get() logging.Logger {
	return logging.Get(UseIfUnset(context.TODO()))
}
