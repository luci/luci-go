// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build appengine

package deflogger

import (
	"golang.org/x/net/context"
)

// UseIfUnset in appengine mode doesn't do anything, because we need an
// appengine.Context to correctly initialize gaelogger.
//
// logging.Get will return a nullLogger if the application didn't set a
// logger in this context already.
func UseIfUnset(c context.Context) context.Context {
	return c
}
