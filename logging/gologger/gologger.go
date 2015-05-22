// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gologger

import (
	"golang.org/x/net/context"
	"os"

	gol "github.com/op/go-logging"

	"infra/libs/logging"
)

const fmt string = "%{color}[P%{pid} %{time:15:04:05.000} %{shortfile} %{level:.4s} %{id:03x}]%{color:reset} %{message}"

type loggerImpl struct {
	l *gol.Logger
}

func (l *loggerImpl) Infof(format string, args ...interface{})    { l.l.Info(format, args...) }
func (l *loggerImpl) Warningf(format string, args ...interface{}) { l.l.Warning(format, args...) }
func (l *loggerImpl) Errorf(format string, args ...interface{})   { l.l.Error(format, args...) }

// UseFile adds a go-logging logger to the context which writes to the provided
// file.
func UseFile(c context.Context, f *os.File) context.Context {
	backend := gol.NewLogBackend(os.Stdout, "", 0)
	formatted := gol.NewBackendFormatter(backend, gol.MustStringFormatter(fmt))
	gol.SetBackend(formatted)
	log := &loggerImpl{gol.MustGetLogger("")}
	log.l.ExtraCalldepth = 1 // one layer of wrapping in loggerImpl struct above
	return logging.Set(c, func(context.Context) logging.Logger { return log })
}

// Use adds a go-logging logger to the context which writes to os.Stdout.
func Use(c context.Context) context.Context {
	return UseFile(c, os.Stdout)
}
