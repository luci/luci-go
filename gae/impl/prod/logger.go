// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package prod

import (
	"fmt"

	"github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
	"google.golang.org/appengine/log"
)

// useLogging adds a logging.Logger implementation to the context which logs to
// appengine's log handler.
func useLogging(c context.Context) context.Context {
	return logging.SetFactory(c, func(ic context.Context) logging.Logger {
		return &loggerImpl{AEContext(ic), ic}
	})
}

type loggerImpl struct {
	aeCtx context.Context
	ic    context.Context
}

func (gl *loggerImpl) Debugf(format string, args ...interface{}) {
	gl.LogCall(logging.Debug, 1, format, args)
}
func (gl *loggerImpl) Infof(format string, args ...interface{}) {
	gl.LogCall(logging.Info, 1, format, args)
}
func (gl *loggerImpl) Warningf(format string, args ...interface{}) {
	gl.LogCall(logging.Warning, 1, format, args)
}
func (gl *loggerImpl) Errorf(format string, args ...interface{}) {
	gl.LogCall(logging.Error, 1, format, args)
}

// TODO(riannucci): prefix with caller's code location.
func (gl *loggerImpl) LogCall(l logging.Level, calldepth int, format string, args []interface{}) {
	if gl.aeCtx == nil || !logging.IsLogging(gl.ic, l) {
		return
	}

	var logf func(context.Context, string, ...interface{})
	switch l {
	case logging.Debug:
		logf = log.Debugf
	case logging.Info:
		logf = log.Infof
	case logging.Warning:
		logf = log.Warningf

	case logging.Error:
		fallthrough
	default:
		logf = log.Errorf
	}

	fields := logging.GetFields(gl.ic)
	if len(fields) > 0 {
		logf(gl.aeCtx, "%s :: %s", fmt.Sprintf(format, args...), fields.String())
	} else {
		logf(gl.aeCtx, format, args...)
	}
}
