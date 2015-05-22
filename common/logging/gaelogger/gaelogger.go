// Copyright 2014 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gaelogger

import (
	"golang.org/x/net/context"

	"appengine"

	"infra/libs/logging"
)

// Use adds a logging.Logger implementation to the context which logs to
// appengine's log handler.
func Use(c context.Context, gaeCtx appengine.Context) context.Context {
	return logging.Set(c, func(context.Context) logging.Logger { return gaeCtx })
}
