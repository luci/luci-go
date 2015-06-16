// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build appengine

package gaelogger

import (
	"fmt"

	"github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"

	"appengine"
)

// Use adds a logging.Logger implementation to the context which logs to
// appengine's log handler.
func Use(c context.Context, gaeCtx appengine.Context) context.Context {
	return logging.SetFactory(c, func(ic context.Context) logging.Logger {
		return &gaeLoggerImpl{gaeCtx, ic}
	})
}

type gaeLoggerImpl struct {
	gaeCtx appengine.Context
	c      context.Context
}

var _ logging.Logger = (*gaeLoggerImpl)(nil)

// TODO(riannucci): prefix with caller's code location.

func (g *gaeLoggerImpl) og(l logging.Level, logf func(string, ...interface{}),
	format string, args []interface{}) {
	if g.c != nil {
		if !logging.IsLogging(g.c, l) {
			return
		}
	}

	fields := logging.GetFields(g.c)
	if len(fields) > 0 {
		logf("%s ctx%s", fmt.Sprintf(format, args...), fields.String())
	} else {
		logf(format, args...)
	}
}

func (gl *gaeLoggerImpl) Debugf(format string, args ...interface{}) {
	gl.og(logging.Debug, gl.gaeCtx.Debugf, format, args)
}
func (gl *gaeLoggerImpl) Infof(format string, args ...interface{}) {
	gl.og(logging.Info, gl.gaeCtx.Infof, format, args)
}
func (gl *gaeLoggerImpl) Warningf(format string, args ...interface{}) {
	gl.og(logging.Warning, gl.gaeCtx.Warningf, format, args)
}
func (gl *gaeLoggerImpl) Errorf(format string, args ...interface{}) {
	gl.og(logging.Error,
		gl.gaeCtx.Errorf, format, args)
}

func (gl *gaeLoggerImpl) LogCall(l logging.Level, calldepth int, format string, args []interface{}) {
	switch l {
	case logging.Debug:
		gl.Debugf(format, args...)
	case logging.Info:
		gl.Infof(format, args...)
	case logging.Warning:
		gl.Warningf(format, args...)
	case logging.Error:
		gl.Errorf(format, args...)
	}
}
