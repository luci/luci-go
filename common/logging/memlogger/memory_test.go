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

package memlogger

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/convey"
	"go.chromium.org/luci/common/testing/truth/should"
)

func TestLogger(t *testing.T) {
	ftt.Run("Zero", t, func(t *ftt.Test) {
		var l MemLogger
		l.Debugf("test %s", logging.Debug)
		l.Infof("test %s", logging.Info)

		assert.Loosely(t, &l, convey.Adapt(ShouldHaveLog)(logging.Debug, "test debug"))
		assert.Loosely(t, &l, convey.Adapt(ShouldHaveLog)(logging.Info, "test info"))
	})
	ftt.Run("logger", t, func(t *ftt.Test) {
		c := Use(logging.SetLevel(context.Background(), logging.Debug))
		l := logging.Get(c)
		assert.Loosely(t, l, should.NotBeNil)
		l.Debugf("test %s", logging.Debug)
		l.Infof("test %s", logging.Info)
		l.Warningf("test %s", logging.Warning)
		l.Errorf("test %s", logging.Error)
		l.Errorf("test WAT: %s", logging.Level(9001))
		ml := l.(*MemLogger)

		assert.Loosely(t, ml, convey.Adapt(ShouldHaveLog)(logging.Debug, "test debug"))
		assert.Loosely(t, ml, convey.Adapt(ShouldHaveLog)(logging.Info, "test info"))
		assert.Loosely(t, ml, convey.Adapt(ShouldHaveLog)(logging.Warning, "test warning"))
		assert.Loosely(t, ml, convey.Adapt(ShouldHaveLog)(logging.Error, "test error"))
		assert.Loosely(t, ml, convey.Adapt(ShouldHaveLog)(logging.Error, "test WAT: unknown"))
	})

	ftt.Run("logger context", t, func(t *ftt.Test) {
		c := Use(context.Background())
		l := logging.Get(c)
		assert.Loosely(t, l, should.NotBeNil)
		ml := l.(*MemLogger)

		l.Infof("totally works: %s", "yes")

		assert.Loosely(t, ml, convey.Adapt(ShouldHaveLog)(logging.Info, "totally works: yes"))
		assert.Loosely(t, ml, convey.Adapt(ShouldNotHaveLog)(logging.Warning, "totally works: yes"))
	})

	ftt.Run("field data", t, func(t *ftt.Test) {
		c := Use(context.Background())
		data := map[string]any{
			"trombone": 50,
			"cat":      "amazing",
		}
		c = logging.SetFields(c, logging.NewFields(data))
		l := logging.Get(c)
		ml := l.(*MemLogger)

		l.Infof("Some unsuspecting log")
		msgs := ml.Messages()
		assert.Loosely(t, msgs[0].Data["trombone"], should.Equal(50))
		assert.Loosely(t, msgs[0].Data["cat"], should.Equal("amazing"))
	})

	ftt.Run("reset", t, func(t *ftt.Test) {
		c := Use(context.Background())
		l := logging.Get(c).(*MemLogger)

		l.Infof("hello")
		assert.Loosely(t, len(l.Messages()), should.Equal(1))

		l.Reset()
		assert.Loosely(t, len(l.Messages()), should.BeZero)

		l.Infof("shweeet")
		assert.Loosely(t, len(l.Messages()), should.Equal(1))
	})

	ftt.Run("dump", t, func(t *ftt.Test) {
		var l MemLogger
		l.lctx = &logging.LogContext{Fields: map[string]any{"key": 100}}
		l.Debugf("test %s", logging.Debug)
		l.Infof("test %s", logging.Info)

		l.lctx.StackTrace.Textual = "stack trace"
		l.Errorf("test stack")

		buf := bytes.Buffer{}
		_, _ = l.Dump(&buf)

		assert.Loosely(t, buf.String(), should.Equal(`
DUMP LOG:
  debug: test debug: {"key":100}
  info: test info: {"key":100}
  error: test stack: {"key":100}
    stack trace
`))
	})
}

func TestLoggerAssertion(t *testing.T) {
	t.Parallel()

	ftt.Run("ShouldHaveLog", t, func(t *ftt.Test) {
		t.Run("basic", func(t *ftt.Test) {
			m := &MemLogger{
				lock: &sync.Mutex{},
				data: &[]LogEntry{
					{
						Level:     logging.Error,
						Msg:       "HI THAR",
						Data:      map[string]any{"hi": 3},
						CallDepth: 47,
					},
				},
			}

			assert.Loosely(t, ShouldHaveLog(m, logging.Error, "HI THAR", map[string]any{"hi": 3}), should.BeEmpty)
			assert.Loosely(t, ShouldHaveLog(m, logging.Error, "HI THAR", map[string]any{"hi": 4}), should.NotEqual(""))
			assert.Loosely(t, ShouldHaveLog(m, logging.Error, "Hi THAR", map[string]any{"hi": 4}), should.NotEqual(""))
			assert.Loosely(t, ShouldHaveLog(m, logging.Error, "THAR", map[string]any{"hi": 4}), should.NotEqual(""))
		})

		t.Run("level and message", func(t *ftt.Test) {
			m := &MemLogger{
				lock: &sync.Mutex{},
				data: &[]LogEntry{
					{
						Level: logging.Error,
						Msg:   "HI THAR",
						Data:  map[string]any{"hi": 3},
					},
				},
			}

			assert.Loosely(t, ShouldHaveLog(m, logging.Error, "HI THAR"), should.BeEmpty)
		})

		t.Run("level only", func(t *ftt.Test) {
			m := &MemLogger{
				lock: &sync.Mutex{},
				data: &[]LogEntry{
					{
						Level:     logging.Error,
						Msg:       "HI THAR",
						Data:      nil,
						CallDepth: 47,
					},
				},
			}

			assert.Loosely(t, ShouldHaveLog(m, logging.Error, "BYE"), should.NotEqual(""))
			assert.Loosely(t, ShouldHaveLog(m, logging.Error), should.BeEmpty)
		})

		t.Run("bad logger", func(t *ftt.Test) {
			assert.Loosely(t, ShouldHaveLog(nil), should.NotEqual(""))
		})

		t.Run("bad level", func(t *ftt.Test) {
			m := &MemLogger{}

			assert.Loosely(t, ShouldHaveLog(m, "BOO"), should.NotEqual(""))
		})

		t.Run("bad message", func(t *ftt.Test) {
			m := &MemLogger{}

			assert.Loosely(t, ShouldHaveLog(m, logging.Error, 48), should.NotEqual(""))
		})

		t.Run("bad depth", func(t *ftt.Test) {
			m := &MemLogger{}

			assert.Loosely(t, ShouldHaveLog(m, logging.Error, "HI THAR", "NO BAD"), should.NotEqual(""))
		})

		t.Run("not found", func(t *ftt.Test) {
			m := &MemLogger{
				lock: &sync.Mutex{},
				data: &[]LogEntry{},
			}

			assert.Loosely(t, ShouldHaveLog(m, logging.Error, "BYE THAR", 47), should.NotEqual(""))
		})

		t.Run("need at least one argument", func(t *ftt.Test) {
			m := &MemLogger{}

			assert.Loosely(t, ShouldHaveLog(m), should.NotEqual(""))
		})
	})
}
