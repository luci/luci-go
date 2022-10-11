// Copyright 2015 The LUCI Authors.
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

package teelogger

import (
	"context"
	"fmt"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
)

func TestTeeLogger(t *testing.T) {
	Convey(`A new TeeLogger instance`, t, func() {
		l1 := logging.Get(
			memlogger.Use(context.Background())).(*memlogger.MemLogger)
		l2 := logging.Get(
			memlogger.Use(context.Background())).(*memlogger.MemLogger)
		l3 := logging.Get(
			memlogger.Use(context.Background())).(*memlogger.MemLogger)
		factories := []logging.Factory{
			func(_ context.Context) logging.Logger { return l1 },
			func(_ context.Context) logging.Logger { return l2 },
			func(_ context.Context) logging.Logger { return l3 },
		}
		Convey("Set level Debug", func() {
			ctx := Use(context.Background(), factories...)
			ctx = logging.SetLevel(ctx, logging.Debug)
			teeLog := logging.Get(ctx)
			for _, entry := range []struct {
				L logging.Level
				F func(string, ...interface{})
				T string
			}{
				{logging.Debug, teeLog.Debugf, "DEBU"},
				{logging.Info, teeLog.Infof, "INFO"},
				{logging.Warning, teeLog.Warningf, "WARN"},
				{logging.Error, teeLog.Errorf, "ERRO"},
			} {
				Convey(fmt.Sprintf("Can log to %s", entry.L), func() {
					entry.F("%s", entry.T)
					for _, logger := range []*memlogger.MemLogger{l1, l2, l3} {
						So(len(logger.Messages()), ShouldEqual, 1)
						msg := logger.Get(entry.L, entry.T, map[string]interface{}(nil))
						So(msg, ShouldNotBeNil)
						So(msg.CallDepth, ShouldEqual, 3)
					}
				})
			}
		})
		Convey("Set level as Warning", func() {
			ctx := Use(context.Background(), factories...)
			ctx = logging.SetLevel(ctx, logging.Warning)
			teeLog := logging.Get(ctx)
			for _, entry := range []struct {
				L logging.Level
				F func(string, ...interface{})
				T string
				E bool
			}{
				{logging.Debug, teeLog.Debugf, "DEBU", false},
				{logging.Info, teeLog.Infof, "INFO", false},
				{logging.Warning, teeLog.Warningf, "WARN", true},
				{logging.Error, teeLog.Errorf, "ERRO", true},
			} {
				Convey(fmt.Sprintf("Can log to %s", entry.L), func() {
					entry.F("%s", entry.T)
					for _, logger := range []*memlogger.MemLogger{l1, l2, l3} {
						if entry.E {
							So(len(logger.Messages()), ShouldEqual, 1)
							msg := logger.Get(entry.L, entry.T, map[string]interface{}(nil))
							So(msg, ShouldNotBeNil)
							So(msg.CallDepth, ShouldEqual, 3)
						} else {
							So(len(logger.Messages()), ShouldEqual, 0)
						}
					}
				})
			}
		})
		Convey("Uses context logger", func() {
			ctx := memlogger.Use(context.Background())
			logger := logging.Get(ctx).(*memlogger.MemLogger)

			teeCtx := Use(ctx)
			logging.Get(teeCtx).Infof("Testing 1 2")
			messages := logger.Messages()

			// Make sure context logger doesn't get called
			So(len(messages), ShouldEqual, 1)
			msg := messages[0]
			So(msg.CallDepth, ShouldEqual, 3)
			So(msg.Msg, ShouldEqual, "Testing 1 2")
		})
	})
}

func TestTeeFilteredLogger(t *testing.T) {
	Convey(`A new TeeLogger instance`, t, func() {
		lD := logging.Get(memlogger.Use(context.Background())).(*memlogger.MemLogger)
		lI := logging.Get(memlogger.Use(context.Background())).(*memlogger.MemLogger)
		lW := logging.Get(memlogger.Use(context.Background())).(*memlogger.MemLogger)
		lE := logging.Get(memlogger.Use(context.Background())).(*memlogger.MemLogger)
		makeFactory := func(l logging.Logger) logging.Factory {
			return func(_ context.Context) logging.Logger { return l }
		}
		filtereds := []Filtered{
			{makeFactory(lD), logging.Debug},
			{makeFactory(lI), logging.Info},
			{makeFactory(lW), logging.Warning},
			{makeFactory(lE), logging.Error},
		}
		ctx := UseFiltered(context.Background(), filtereds...)
		// The context level is ignored, even we set it.
		ctx = logging.SetLevel(ctx, logging.Error)
		teeLog := logging.Get(ctx)

		for _, entry := range []struct {
			L logging.Level
			F func(string, ...interface{})
			T string
			// Loggers which have messages.
			GoodLogger []*memlogger.MemLogger
			// Loggers which do not have messages.
			BadLogger []*memlogger.MemLogger
		}{
			{logging.Debug, teeLog.Debugf, "DEBU",
				[]*memlogger.MemLogger{lD},
				[]*memlogger.MemLogger{lI, lW, lE}},
			{logging.Info, teeLog.Infof, "INFO",
				[]*memlogger.MemLogger{lD, lI},
				[]*memlogger.MemLogger{lW, lE}},
			{logging.Warning, teeLog.Warningf, "WARN",
				[]*memlogger.MemLogger{lD, lI, lW},
				[]*memlogger.MemLogger{lE}},
			{logging.Error, teeLog.Errorf, "ERRO",
				[]*memlogger.MemLogger{lD, lI, lW, lE},
				[]*memlogger.MemLogger{}},
		} {
			Convey(fmt.Sprintf("Can log to %s", entry.L), func() {
				entry.F("%s", entry.T)
				for _, l := range entry.GoodLogger {
					So(len(l.Messages()), ShouldEqual, 1)
					msg := l.Get(entry.L, entry.T, map[string]interface{}(nil))
					So(msg, ShouldNotBeNil)
					So(msg.CallDepth, ShouldEqual, 3)
				}
				for _, l := range entry.BadLogger {
					So(len(l.Messages()), ShouldEqual, 0)
				}
			})
		}
		Convey("Use context logger with context level Debug", func() {
			ctx := memlogger.Use(context.Background())
			logger := logging.Get(ctx).(*memlogger.MemLogger)

			teeCtx := UseFiltered(ctx)
			teeCtx = logging.SetLevel(teeCtx, logging.Debug)
			l := logging.Get(teeCtx)
			So(l, ShouldNotBeNil)
			l.Infof("Info testing 1 2")
			messages := logger.Messages()

			// Make sure context logger doesn't get called
			So(len(messages), ShouldEqual, 1)
			msg := messages[0]
			So(msg.CallDepth, ShouldEqual, 3)
			So(msg.Msg, ShouldEqual, "Info testing 1 2")
		})
		Convey("Use context logger with context level Warning", func() {
			ctx := memlogger.Use(context.Background())
			logger := logging.Get(ctx).(*memlogger.MemLogger)

			teeCtx := UseFiltered(ctx)
			teeCtx = logging.SetLevel(teeCtx, logging.Warning)
			logging.Get(teeCtx).Infof("Info testing 1 2")
			messages := logger.Messages()

			// Make sure context logger doesn't have messages
			So(len(messages), ShouldEqual, 0)
		})
	})
}
