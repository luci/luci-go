// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package logging implements Logger, a gRPC glog.Logger implementation backed
// by a github.com/luci/luci-go/common/logging Logger.
package logging

import (
	"os"
	"runtime"
	"strings"

	log "github.com/luci/luci-go/common/logging"

	"google.golang.org/grpc/grpclog"
)

type grpcLogger struct {
	// Base is the base logger instance.
	base log.Logger

	// logPrints is true if Print statements should be logged.
	logPrints bool
}

// Install installs a logger as the gRPC library's logger. The installation is
// not protected by a mutex, so this must be set somewhere that atomic access is
// guaranteed.
func Install(base log.Logger, logPrints bool) {
	grpclog.SetLogger(&grpcLogger{
		base:      base,
		logPrints: logPrints,
	})
}

func makeArgFormatString(args []interface{}) string {
	if len(args) == 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("%v ", len(args)), " ")
}

func (gl *grpcLogger) Fatal(args ...interface{}) {
	gl.base.LogCall(log.Error, 2, makeArgFormatString(args), args)
	gl.logStackTraceAndDie()
}

func (gl *grpcLogger) Fatalf(format string, args ...interface{}) {
	gl.base.LogCall(log.Error, 2, format, args)
	gl.logStackTraceAndDie()
}

func (gl *grpcLogger) Fatalln(args ...interface{}) {
	gl.base.LogCall(log.Error, 2, makeArgFormatString(args), args)
	gl.logStackTraceAndDie()
}

func (gl *grpcLogger) Print(args ...interface{}) {
	if !gl.logPrints {
		return
	}
	gl.base.LogCall(log.Info, 2, makeArgFormatString(args), args)
}

func (gl *grpcLogger) Printf(format string, args ...interface{}) {
	if !gl.logPrints {
		return
	}
	gl.base.LogCall(log.Info, 2, format, args)
}

func (gl *grpcLogger) Println(args ...interface{}) {
	if !gl.logPrints {
		return
	}
	gl.base.LogCall(log.Info, 2, makeArgFormatString(args), args)
}

func (gl *grpcLogger) logStackTraceAndDie() {
	gl.base.LogCall(log.Error, 3, "Stack Trace:\n%s", []interface{}{stacks(true)})
	osExit(255)
}

// osExit is an operating system exit function used by "Fatal" logs. It is a
// variable here so it can be stubbed for testing.
var osExit = func(rc int) {
	os.Exit(rc)
}

// stacks is a wrapper for runtime.Stack that attempts to recover the data for
// all goroutines.
//
// This was copied from the glog library:
// https://github.com/golang/glog @ / 23def4e6c14b4da8ac2ed8007337bc5eb5007998
func stacks(all bool) []byte {
	// We don't know how big the traces are, so grow a few times if they don't fit.
	// Start large, though.
	n := 10000
	if all {
		n = 100000
	}
	var trace []byte
	for i := 0; i < 5; i++ {
		trace = make([]byte, n)
		nbytes := runtime.Stack(trace, all)
		if nbytes < len(trace) {
			return trace[:nbytes]
		}
		n *= 2
	}
	return trace
}
