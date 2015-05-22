// Copyright 2015 The Chromium Authors. All rights reserved.
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// +build !appengine

package logging

import (
	"os"

	"github.com/op/go-logging"
)

const fmt string = "%{color}[P%{pid} %{time:15:04:05.000} %{shortfile} %{level:.4s} %{id:03x}]%{color:reset} %{message}"

type logger struct {
	l *logging.Logger
}

func (l *logger) Infof(format string, args ...interface{})    { l.l.Info(format, args...) }
func (l *logger) Warningf(format string, args ...interface{}) { l.l.Warning(format, args...) }
func (l *logger) Errorf(format string, args ...interface{})   { l.l.Error(format, args...) }

func init() {
	backend := logging.NewLogBackend(os.Stdout, "", 0)
	formatted := logging.NewBackendFormatter(backend, logging.MustStringFormatter(fmt))
	logging.SetBackend(formatted)

	log := &logger{logging.MustGetLogger("")}
	log.l.ExtraCalldepth = 1 // one layer of wrapping in logger struct above
	DefaultLogger = log
}
