// Copyright 2016 The LUCI Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package logging implements a gRPC glog.Logger implementation backed
// by a go.chromium.org/luci/common/logging Logger.
//
// The logger can be installed by calling Install.
package logging

import (
	"os"
	"runtime"
	"strings"

	"google.golang.org/grpc/grpclog"

	"go.chromium.org/luci/common/logging"
)

// Suppress is a sentinel logging level that instructs the logger to suppress
// all non-fatal logging output. This is NOT a valid logging.Level, and should
// not be used as such.
var Suppress = logging.Level(logging.Error + 1)

type grpcLogger struct {
	base   logging.Logger
	vLevel int
}

// Install installs a logger as the gRPC library's logger. The installation is
// not protected by a mutex, so this must be set somewhere that atomic access is
// guaranteed.
//
// A special logging level, "Suppress", can be provided to suppress all
// non-fatal logging output .
//
// gRPC V=level and error terminology translation is as follows:
// - V=0, ERROR (low verbosity) is logged at logging.ERROR level.
// - V=1, WARNING (medium verbosity) is logged at logging.WARNING level.
// - V=2, INFO (high verbosity) is logged at logging.DEBUG level.
func Install(base logging.Logger, level logging.Level) {
	grpclog.SetLoggerV2(&grpcLogger{
		base:   base,
		vLevel: translateLevel(level),
	})
}

func (gl *grpcLogger) Info(args ...any) {
	if gl.V(2) {
		gl.base.LogCall(logging.Debug, 2, makeArgFormatString(args), args)
	}
}

func (gl *grpcLogger) Infof(format string, args ...any) {
	if gl.V(2) {
		gl.base.LogCall(logging.Debug, 2, format, args)
	}
}

func (gl *grpcLogger) Infoln(args ...any) {
	if gl.V(2) {
		gl.base.LogCall(logging.Debug, 2, makeArgFormatString(args), args)
	}
}

func (gl *grpcLogger) Warning(args ...any) {
	if gl.V(1) {
		gl.base.LogCall(logging.Warning, 2, makeArgFormatString(args), args)
	}
}

func (gl *grpcLogger) Warningf(format string, args ...any) {
	if gl.V(1) {
		gl.base.LogCall(logging.Warning, 2, format, args)
	}
}

func (gl *grpcLogger) Warningln(args ...any) {
	if gl.V(1) {
		gl.base.LogCall(logging.Warning, 2, makeArgFormatString(args), args)
	}
}

func (gl *grpcLogger) Error(args ...any) {
	if gl.V(0) {
		gl.base.LogCall(logging.Error, 2, makeArgFormatString(args), args)
	}
}

func (gl *grpcLogger) Errorf(format string, args ...any) {
	if gl.V(0) {
		gl.base.LogCall(logging.Error, 2, format, args)
	}
}

func (gl *grpcLogger) Errorln(args ...any) {
	if gl.V(0) {
		gl.base.LogCall(logging.Error, 2, makeArgFormatString(args), args)
	}
}

func (gl *grpcLogger) Fatal(args ...any) {
	gl.base.LogCall(logging.Error, 2, makeArgFormatString(args), args)
	gl.logStackTraceAndDie()
}

func (gl *grpcLogger) Fatalf(format string, args ...any) {
	gl.base.LogCall(logging.Error, 2, format, args)
	gl.logStackTraceAndDie()
}

func (gl *grpcLogger) Fatalln(args ...any) {
	gl.base.LogCall(logging.Error, 2, makeArgFormatString(args), args)
	gl.logStackTraceAndDie()
}

func (gl *grpcLogger) V(l int) bool {
	return gl.vLevel >= l
}

func (gl *grpcLogger) logStackTraceAndDie() {
	gl.base.LogCall(logging.Error, 3, "Stack Trace:\n%s", []any{stacks(true)})
	fatalExit()
}

// fatalExit is an operating system exit function used by "Fatal" logs. It is a
// variable here so it can be stubbed for testing, but will exit with a non-zero
// return code by default.
var fatalExit = func() {
	os.Exit(1)
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

func makeArgFormatString(args []any) string {
	if len(args) == 0 {
		return ""
	}
	return strings.TrimSuffix(strings.Repeat("%v ", len(args)), " ")
}

// translateLevel translates a "verbose level" to a logging level.
func translateLevel(l logging.Level) int {
	switch l {
	case Suppress:
		return -1
	case logging.Error:
		return 0
	case logging.Warning, logging.Info:
		return 1
	default:
		return 2
	}
}
