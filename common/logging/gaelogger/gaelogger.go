// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build appengine

package gaelogger

import (
	"fmt"

	"golang.org/x/net/context"

	"appengine"

	"github.com/luci/luci-go/common/logging"
)

// Use adds a logging.Logger implementation to the context which logs to
// appengine's log handler.
func Use(c context.Context, gaeCtx appengine.Context) context.Context {
	return logging.SetFactory(c, func(ic context.Context) logging.Logger {
		return &gaeLoggerImpl{gaeCtx, logging.GetFields(ic)}
	})
}

type gaeLoggerImpl struct {
	c      appengine.Context
	fields logging.Fields
}

var _ logging.Logger = (*gaeLoggerImpl)(nil)

// TODO(riannucci): prefix with caller's code location.

func (g *gaeLoggerImpl) og(logf func(string, ...interface{}), format string, args []interface{}) {
	if g.fields.Len() > 0 {
		logf("%s ctx%s", fmt.Sprintf(format, args...), g.fields)
	} else {
		logf(format, args...)
	}
}

func (l *gaeLoggerImpl) Debugf(format string, args ...interface{})   { l.og(l.c.Debugf, format, args) }
func (l *gaeLoggerImpl) Infof(format string, args ...interface{})    { l.og(l.c.Infof, format, args) }
func (l *gaeLoggerImpl) Warningf(format string, args ...interface{}) { l.og(l.c.Warningf, format, args) }
func (l *gaeLoggerImpl) Errorf(format string, args ...interface{})   { l.og(l.c.Errorf, format, args) }
