// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build !appengine

package deflogger

import (
	"golang.org/x/net/context"

	"infra/libs/logging"
	"infra/libs/logging/gologger"
)

// UseIfUnset adds the 'default' non-appengine logger (gologger) to c if the
// context doesn't have any defined logger.
func UseIfUnset(c context.Context) context.Context {
	if !logging.IsSet(c) {
		return gologger.Use(c)
	}
	return c
}
