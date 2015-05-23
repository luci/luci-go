// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gologger

import (
	"os"
	"sync"

	"golang.org/x/net/context"

	gol "github.com/op/go-logging"

	"github.com/luci/luci-go/common/logging"
)

const fmt string = "%{color}" +
	"[P%{pid} %{time:15:04:05.000} %{shortfile} %{level:.4s} %{id:03x}]" +
	"%{color:reset} %{message}"

var (
	global     logging.Logger
	initGlobal sync.Once
)

type loggerImpl struct {
	l *gol.Logger
}

func (l *loggerImpl) Debugf(format string, args ...interface{})   { l.l.Debug(format, args...) }
func (l *loggerImpl) Infof(format string, args ...interface{})    { l.l.Info(format, args...) }
func (l *loggerImpl) Warningf(format string, args ...interface{}) { l.l.Warning(format, args...) }
func (l *loggerImpl) Errorf(format string, args ...interface{})   { l.l.Error(format, args...) }

// New creates new logging.Logger backed by go-logging library. The new logger
// writes (to the provided file) messages of a given log level (or above).
// A caller is still responsible for closing the file when no longer needed.
func New(f *os.File, level gol.Level) logging.Logger {
	// Leveled formatted file backend.
	backend := gol.AddModuleLevel(
		gol.NewBackendFormatter(
			gol.NewLogBackend(f, "", 0),
			gol.MustStringFormatter(fmt)))
	backend.SetLevel(level, "")
	logger := gol.MustGetLogger("")
	logger.SetBackend(backend)
	return Wrap(logger)
}

// Get returns default global go-logging based logger. It writes >=INFO message
// to stdout.
func Get() logging.Logger {
	initGlobal.Do(func() { global = New(os.Stdout, gol.INFO) })
	return global
}

// Use adds a default go-logging logger to the context.
func Use(c context.Context) context.Context {
	return logging.SetFactory(c, func(context.Context) logging.Logger { return Get() })
}

// Wrap takes existing go-logging Logger and returns logging.Logger. It can then
// be put into a context with logging.Set(...).
func Wrap(l *gol.Logger) logging.Logger {
	l.ExtraCalldepth += 1 // one layer of wrapping in loggerImpl struct above
	return &loggerImpl{l}
}
