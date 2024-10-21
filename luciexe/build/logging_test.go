// Copyright 2020 The LUCI Authors.
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

package build

import (
	"context"
	"testing"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestLogableNoop(t *testing.T) {
	ftt.Run(`Loggable Noop`, t, func(t *ftt.Test) {
		t.Run(`nop`, func(t *ftt.Test) {
			var nop nopStream
			n, err := nop.Write([]byte("hey"))
			assert.Loosely(t, n, should.Equal(3))
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, nop.Close(), should.BeNil)
		})

		t.Run(`nopDatagram`, func(t *ftt.Test) {
			var nop nopDatagramStream
			assert.Loosely(t, nop.WriteDatagram([]byte("hey")), should.BeNil)
			assert.Loosely(t, nop.Close(), should.BeNil)
		})

		t.Run(`loggingWriter`, func(t *ftt.Test) {
			ctx := memlogger.Use(context.Background())
			logs := logging.Get(ctx).(*memlogger.MemLogger)
			lw := makeLoggingWriter(ctx, "some log")

			t.Run(`single line`, func(t *ftt.Test) {
				n, err := lw.Write([]byte("hello world\n"))
				assert.Loosely(t, n, should.Equal(12))
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Info, "hello world", logging.Fields{
					"build.logname": "some log",
				}))

				assert.Loosely(t, lw.Close(), should.BeNil)
				assert.Loosely(t, logs.Messages(), should.HaveLength(1))
			})

			t.Run(`multi line`, func(t *ftt.Test) {
				n, err := lw.Write([]byte("hello world\ncool\nbeans\n"))
				assert.Loosely(t, n, should.Equal(23))
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Info, "hello world"))
				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Info, "cool"))
				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Info, "beans"))

				assert.Loosely(t, lw.Close(), should.BeNil)
				assert.Loosely(t, logs.Messages(), should.HaveLength(3))
			})

			t.Run(`partial line`, func(t *ftt.Test) {
				n, err := lw.Write([]byte("hello worl"))
				assert.Loosely(t, n, should.Equal(10))
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, logs.Messages(), should.HaveLength(0))

				n, err = lw.Write([]byte("d\ncool\n\n\n"))
				assert.Loosely(t, n, should.Equal(9))
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Info, "hello world"))
				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Info, "cool"))
				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Info, ""))

				assert.Loosely(t, lw.Close(), should.BeNil)
				assert.Loosely(t, logs.Messages(), should.HaveLength(4))
			})

			t.Run(`partial flush`, func(t *ftt.Test) {
				n, err := lw.Write([]byte("hello worl"))
				assert.Loosely(t, n, should.Equal(10))
				assert.Loosely(t, err, should.BeNil)

				assert.Loosely(t, lw.Close(), should.BeNil)
				assert.Loosely(t, logs.Messages(), should.HaveLength(1))

				assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Info, "hello worl"))
			})
		})

		t.Run(`loggingWriter - debug`, func(t *ftt.Test) {
			ctx := memlogger.Use(context.Background())
			ctx = logging.SetLevel(ctx, logging.Debug)
			logs := logging.Get(ctx).(*memlogger.MemLogger)
			lw := makeLoggingWriter(ctx, "$some log")

			n, err := lw.Write([]byte("hello world\n"))
			assert.Loosely(t, n, should.Equal(12))
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, logs, convey.Adapt(memlogger.ShouldHaveLog)(logging.Debug, "hello world", logging.Fields{
				"build.logname": "$some log",
			}))

			assert.Loosely(t, lw.Close(), should.BeNil)
			assert.Loosely(t, logs.Messages(), should.HaveLength(1))
		})

		t.Run(`loggingWriter - debug skip`, func(t *ftt.Test) {
			ctx := memlogger.Use(context.Background())
			ctx = logging.SetLevel(ctx, logging.Info) // ignore debug
			logs := logging.Get(ctx).(*memlogger.MemLogger)
			lw := makeLoggingWriter(ctx, "$some log")

			n, err := lw.Write([]byte("hello world\n"))
			assert.Loosely(t, n, should.Equal(12))
			assert.Loosely(t, err, should.BeNil)

			assert.Loosely(t, lw.Close(), should.BeNil)
			assert.Loosely(t, logs.Messages(), should.HaveLength(0))
		})
	})
}
