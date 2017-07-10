// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package logging

import (
	"fmt"
	"testing"

	"github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/memlogger"

	"google.golang.org/grpc/grpclog"

	. "github.com/smartystreets/goconvey/convey"
)

// TestGRPCLogger assumes that it has exclusive ownership of the grpclog
// package. Each test MUST execute in series, as they install a global logger
// into the grpclog package.
func TestGRPCLogger(t *testing.T) {
	Convey(`Testing gRPC logger adapter`, t, func() {
		var base memlogger.MemLogger

		// Override "fatalExit" so our Fatal* tests don't actually exit.
		exitCalls := 0
		fatalExit = func() {
			exitCalls++
		}

		for _, l := range []logging.Level{
			logging.Debug,
			logging.Info,
			logging.Warning,
			logging.Error,
			Suppress,
		} {
			var name string
			if l == Suppress {
				name = "SUPPRESSED"
			} else {
				name = l.String()
			}
			Convey(fmt.Sprintf(`At logging level %q`, name), func() {
				Convey(`Logs correctly`, func() {
					Install(&base, l)

					// expected will accumulate during logging based on the current test
					// log level, "l".
					var expected []memlogger.LogEntry

					// Info
					grpclog.Info("info", "foo", "bar")
					grpclog.Infof("infof(%q, %q)", "foo", "bar")
					grpclog.Infoln("infoln", "foo", "bar")
					switch l {
					case Suppress, logging.Error, logging.Warning, logging.Info:
					default:
						expected = append(expected, []memlogger.LogEntry{
							{logging.Debug, "info foo bar", nil, 3},
							{logging.Debug, `infof("foo", "bar")`, nil, 3},
							{logging.Debug, "infoln foo bar", nil, 3},
						}...)
					}

					// Warning
					grpclog.Warning("warning", "foo", "bar")
					grpclog.Warningf("warningf(%q, %q)", "foo", "bar")
					grpclog.Warningln("warningln", "foo", "bar")
					switch l {
					case Suppress, logging.Error:
					default:
						expected = append(expected, []memlogger.LogEntry{
							{logging.Warning, "warning foo bar", nil, 3},
							{logging.Warning, `warningf("foo", "bar")`, nil, 3},
							{logging.Warning, "warningln foo bar", nil, 3},
						}...)
					}

					// Error
					grpclog.Error("error", "foo", "bar")
					grpclog.Errorf("errorf(%q, %q)", "foo", "bar")
					grpclog.Errorln("errorln", "foo", "bar")
					switch l {
					case Suppress:
					default:
						expected = append(expected, []memlogger.LogEntry{
							{logging.Error, "error foo bar", nil, 3},
							{logging.Error, `errorf("foo", "bar")`, nil, 3},
							{logging.Error, "errorln foo bar", nil, 3},
						}...)
					}

					So(base.Messages(), ShouldResemble, expected)

					for i := 0; i < 3; i++ {
						exp := i <= translateLevel(l)
						t.Logf("Testing %q V(%d) => %v", name, i, exp)
						So(grpclog.V(i), ShouldEqual, exp)
					}
				})

				// XXX: disabled, pending https://github.com/grpc/grpc-go/issues/1360
				SkipConvey(`Handles Fatal calls`, func() {
					grpclog.Fatal("fatal", "foo", "bar")
					grpclog.Fatalf("fatalf(%q, %q)", "foo", "bar")
					grpclog.Fatalln("fatalln", "foo", "bar")

					So(base.Messages(), ShouldResemble, []memlogger.LogEntry{
						{logging.Error, "fatal foo bar", nil, 3},
						{logging.Error, `fatalf("foo", "bar")`, nil, 3},
						{logging.Error, "fatalln foo bar", nil, 3},
					})
					So(exitCalls, ShouldEqual, 3)
				})
			})
		}
	})
}
