// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package teelogger

import (
	"github.com/luci/luci-go/common/logging"
	"golang.org/x/net/context"
)

// New creates a new tee logger which
func New(loggers ...logging.Logger) logging.Logger {
	return teeImpl(loggers)
}

type teeImpl []logging.Logger

func (t teeImpl) Debugf(fmt string, args ...interface{}) {
	for _, logger := range t {
		logger.LogCall(logging.Debug, 1, fmt, args)
	}
}
func (t teeImpl) Infof(fmt string, args ...interface{}) {
	for _, logger := range t {
		logger.LogCall(logging.Info, 1, fmt, args)
	}
}
func (t teeImpl) Warningf(fmt string, args ...interface{}) {
	for _, logger := range t {
		logger.LogCall(logging.Warning, 1, fmt, args)
	}
}
func (t teeImpl) Errorf(fmt string, args ...interface{}) {
	for _, logger := range t {
		logger.LogCall(logging.Error, 1, fmt, args)
	}
}

func (t teeImpl) LogCall(level logging.Level, calldepth int,
	f string, args []interface{}) {
	for _, logger := range t {
		logger.LogCall(level, calldepth+1, f, args)
	}
}

// Use adds a tee logger to the context, using the logger in the context,
// as well as the other given loggers.
func Use(ctx context.Context, loggers ...logging.Logger) context.Context {
	if cur := logging.Get(ctx); cur != nil {
		loggers = append([]logging.Logger{cur}, loggers...)
	}
	return logging.Set(ctx, New(loggers...))
}
