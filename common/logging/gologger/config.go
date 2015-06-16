// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gologger

import (
	"io"
	"sync"

	"github.com/luci/luci-go/common/logging"
	gol "github.com/op/go-logging"
	"golang.org/x/net/context"
)

// LoggerConfig is a logger logger configuration method.
type LoggerConfig struct {
	Format string // Logging format string.
	Out    io.Writer
	Level  gol.Level

	initOnce sync.Once        // Initialize goLoggerWrapper at most once.
	w        *goLoggerWrapper // Initialized logger wrapper instance.
}

func (lc *LoggerConfig) newGoLogger() *gol.Logger {
	// Leveled formatted file backend.
	backend := gol.AddModuleLevel(
		gol.NewBackendFormatter(
			gol.NewLogBackend(lc.Out, "", 0),
			gol.MustStringFormatter(lc.Format)))
	backend.SetLevel(lc.Level, "")
	logger := gol.MustGetLogger("")
	logger.SetBackend(backend)
	return logger
}

// getImpl returns an unbound loggerImpl instance bound to the LoggerConfig's
// logger, initializing that logger if necessary.
func (lc *LoggerConfig) getImpl() *loggerImpl {
	lc.initOnce.Do(func() {
		lc.w = &goLoggerWrapper{l: lc.newGoLogger()}
	})
	return &loggerImpl{lc.w, nil}
}

// Get returns go-logging based logger attached to the LoggerConfig instance
func (lc *LoggerConfig) Get() logging.Logger {
	return lc.getImpl()
}

// Use adds a default go-logging logger to the context.
func (lc *LoggerConfig) Use(c context.Context) context.Context {
	return logging.SetFactory(c, func(c context.Context) logging.Logger {
		l := lc.getImpl()
		l.c = c
		return l
	})
}
