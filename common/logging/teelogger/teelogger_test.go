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

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestTeeLogger(t *testing.T) {
	ftt.Run(`A new TeeLogger instance`, t, func(t *ftt.Test) {
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
		t.Run("Set level Debug", func(t *ftt.Test) {
			ctx := Use(context.Background(), factories...)
			ctx = logging.SetLevel(ctx, logging.Debug)
			teeLog := logging.Get(ctx)
			for _, entry := range []struct {
				L logging.Level
				F func(string, ...any)
				T string
			}{
				{logging.Debug, teeLog.Debugf, "DEBU"},
				{logging.Info, teeLog.Infof, "INFO"},
				{logging.Warning, teeLog.Warningf, "WARN"},
				{logging.Error, teeLog.Errorf, "ERRO"},
			} {
				t.Run(fmt.Sprintf("Can log to %s", entry.L), func(t *ftt.Test) {
					entry.F("%s", entry.T)
					for _, logger := range []*memlogger.MemLogger{l1, l2, l3} {
						assert.Loosely(t, len(logger.Messages()), should.Equal(1))
						msg := logger.Get(entry.L, entry.T, map[string]any(nil))
						assert.Loosely(t, msg, should.NotBeNil)
						assert.Loosely(t, msg.CallDepth, should.Equal(3))
					}
				})
			}
		})
		t.Run("Set level as Warning", func(t *ftt.Test) {
			ctx := Use(context.Background(), factories...)
			ctx = logging.SetLevel(ctx, logging.Warning)
			teeLog := logging.Get(ctx)
			for _, entry := range []struct {
				L logging.Level
				F func(string, ...any)
				T string
				E bool
			}{
				{logging.Debug, teeLog.Debugf, "DEBU", false},
				{logging.Info, teeLog.Infof, "INFO", false},
				{logging.Warning, teeLog.Warningf, "WARN", true},
				{logging.Error, teeLog.Errorf, "ERRO", true},
			} {
				t.Run(fmt.Sprintf("Can log to %s", entry.L), func(t *ftt.Test) {
					entry.F("%s", entry.T)
					for _, logger := range []*memlogger.MemLogger{l1, l2, l3} {
						if entry.E {
							assert.Loosely(t, len(logger.Messages()), should.Equal(1))
							msg := logger.Get(entry.L, entry.T, map[string]any(nil))
							assert.Loosely(t, msg, should.NotBeNil)
							assert.Loosely(t, msg.CallDepth, should.Equal(3))
						} else {
							assert.Loosely(t, len(logger.Messages()), should.BeZero)
						}
					}
				})
			}
		})
		t.Run("Uses context logger", func(t *ftt.Test) {
			ctx := memlogger.Use(context.Background())
			logger := logging.Get(ctx).(*memlogger.MemLogger)

			teeCtx := Use(ctx)
			logging.Get(teeCtx).Infof("Testing 1 2")
			messages := logger.Messages()

			// Make sure context logger doesn't get called
			assert.Loosely(t, len(messages), should.Equal(1))
			msg := messages[0]
			assert.Loosely(t, msg.CallDepth, should.Equal(3))
			assert.Loosely(t, msg.Msg, should.Equal("Testing 1 2"))
		})
	})
}

func TestTeeFilteredLogger(t *testing.T) {
	ftt.Run(`A new TeeLogger instance`, t, func(t *ftt.Test) {
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
			F func(string, ...any)
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
			t.Run(fmt.Sprintf("Can log to %s", entry.L), func(t *ftt.Test) {
				entry.F("%s", entry.T)
				for _, l := range entry.GoodLogger {
					assert.Loosely(t, len(l.Messages()), should.Equal(1))
					msg := l.Get(entry.L, entry.T, map[string]any(nil))
					assert.Loosely(t, msg, should.NotBeNil)
					assert.Loosely(t, msg.CallDepth, should.Equal(3))
				}
				for _, l := range entry.BadLogger {
					assert.Loosely(t, len(l.Messages()), should.BeZero)
				}
			})
		}
		t.Run("Use context logger with context level Debug", func(t *ftt.Test) {
			ctx := memlogger.Use(context.Background())
			logger := logging.Get(ctx).(*memlogger.MemLogger)

			teeCtx := UseFiltered(ctx)
			teeCtx = logging.SetLevel(teeCtx, logging.Debug)
			l := logging.Get(teeCtx)
			assert.Loosely(t, l, should.NotBeNil)
			l.Infof("Info testing 1 2")
			messages := logger.Messages()

			// Make sure context logger doesn't get called
			assert.Loosely(t, len(messages), should.Equal(1))
			msg := messages[0]
			assert.Loosely(t, msg.CallDepth, should.Equal(3))
			assert.Loosely(t, msg.Msg, should.Equal("Info testing 1 2"))
		})
		t.Run("Use context logger with context level Warning", func(t *ftt.Test) {
			ctx := memlogger.Use(context.Background())
			logger := logging.Get(ctx).(*memlogger.MemLogger)

			teeCtx := UseFiltered(ctx)
			teeCtx = logging.SetLevel(teeCtx, logging.Warning)
			logging.Get(teeCtx).Infof("Info testing 1 2")
			messages := logger.Messages()

			// Make sure context logger doesn't have messages
			assert.Loosely(t, len(messages), should.BeZero)
		})
	})
}
