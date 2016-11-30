// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package logging

import (
	"sync"
	"testing"

	log "github.com/luci/luci-go/common/logging"
	"github.com/luci/luci-go/common/logging/memlogger"

	"google.golang.org/grpc/grpclog"

	. "github.com/smartystreets/goconvey/convey"
)

type grpcInstallMutex sync.Mutex

// TestGRPCLogger assumes that it has exclusive ownership of the grpclog
// package. Each test MUST execute in series, as they install a global logger
// into the grpclog package.
func TestGRPCLogger(t *testing.T) {
	Convey(`Testing gRPC logger adapter`, t, func() {
		var base memlogger.MemLogger

		// Stub the "os.Exit" functionality so we don't actually exit.
		var exitRC int
		osExit = func(rc int) { exitRC = rc }

		Convey(`An adapter that logs prints`, func() {
			Install(&base, true)

			Convey(`Can log a fatal message.`, func() {
				grpclog.Fatalln("foo", 1, "bar", 2)
				So(exitRC, ShouldEqual, 255)

				exitRC = 0
				grpclog.Fatal("foo", 1, "bar", 2)
				So(exitRC, ShouldEqual, 255)

				// Actually test the log messages for "Fatalf".
				base.Reset()
				exitRC = 0
				grpclog.Fatalf("foo(%d)", 42)
				So(exitRC, ShouldEqual, 255)

				msgs := base.Messages()
				So(msgs, ShouldHaveLength, 2)

				So(msgs[0].Level, ShouldEqual, log.Error)
				So(msgs[0].CallDepth, ShouldEqual, 3)
				So(msgs[0].Msg, ShouldEqual, "foo(42)")

				So(msgs[1].Level, ShouldEqual, log.Error)
				So(msgs[1].CallDepth, ShouldEqual, 4)
				So(msgs[1].Msg, ShouldStartWith, "Stack Trace:\n")

			})

			Convey(`Will log a print message.`, func() {
				grpclog.Printf("foo(%d)", 42)
				grpclog.Println("bar")
				grpclog.Print("baz", 1337)
				So(exitRC, ShouldEqual, 0)

				msgs := base.Messages()
				So(msgs, ShouldResemble, []memlogger.LogEntry{
					{log.Info, "foo(42)", nil, 3},
					{log.Info, "bar", nil, 3},
					{log.Info, "baz 1337", nil, 3},
				})
			})
		})

		Convey(`An adapter that doesn't log print`, func() {
			Install(&base, false)

			Convey(`Still prints fatal messages and exits.`, func() {
				grpclog.Fatal("Something weng wrong!")
				So(exitRC, ShouldEqual, 255)
				So(base.Messages(), ShouldHaveLength, 2)
			})

			Convey(`Doesn't forwawrd Print messages.`, func() {
				grpclog.Printf("foo(%d)", 42)
				grpclog.Println("bar")
				grpclog.Print("baz", 1337)
				So(exitRC, ShouldEqual, 0)
				So(base.Messages(), ShouldHaveLength, 0)
			})
		})
	})
}
