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

package gologger

import (
	"bytes"
	"context"
	"fmt"
	"regexp"
	"testing"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/should"
)

var (
	ansiRegexp = regexp.MustCompile(`\033\[.+?m`)

	lre = regexp.MustCompile(
		`\[([A-Z])\d+\-\d+\-\d+T\d+:\d+:\d+\.\d+.* \d+ 0 (.+?):\d+\]\s+(.*)`)
)

func normalizeLog(s string) string {
	// Strip ANSI color sequences.
	return ansiRegexp.ReplaceAllString(s, "")
}

func TestGoLogger(t *testing.T) {
	ftt.Run(`A new Go Logger instance`, t, func(t *ftt.Test) {
		// Regex to pull log information from "formatString".

		buf := bytes.Buffer{}
		cfg := LoggerConfig{Out: &buf}
		l := cfg.NewLogger(context.Background(), &logging.LogContext{})

		for _, entry := range []struct {
			L logging.Level
			F func(string, ...any)
			T string
		}{
			{logging.Debug, l.Debugf, "D"},
			{logging.Info, l.Infof, "I"},
			{logging.Warning, l.Warningf, "W"},
			{logging.Error, l.Errorf, "E"},
		} {
			t.Run(fmt.Sprintf("Can log to: %s", entry.L), func(t *ftt.Test) {
				entry.F("Test logging %s", entry.L)
				matches := lre.FindAllStringSubmatch(normalizeLog(buf.String()), -1)
				assert.Loosely(t, len(matches), should.Equal(1))
				assert.Loosely(t, len(matches[0]), should.Equal(4))
				assert.Loosely(t, matches[0][1], should.Equal(entry.T))
				assert.Loosely(t, matches[0][2], should.Equal("gologger_test.go"))
				assert.Loosely(t, matches[0][3], should.Equal(fmt.Sprintf("Test logging %s", entry.L)))
			})
		}
	})

	ftt.Run(`A Go Logger instance installed in a Context at Info.`, t, func(t *ftt.Test) {
		buf := bytes.Buffer{}
		lc := &LoggerConfig{
			Format: StdConfig.Format,
			Out:    &buf,
		}
		c := logging.SetLevel(lc.Use(context.Background()), logging.Info)

		t.Run(`Should log through top-level Context methods.`, func(t *ftt.Test) {
			for _, entry := range []struct {
				L logging.Level
				F func(context.Context, string, ...any)
				T string
			}{
				{logging.Info, logging.Infof, "I"},
				{logging.Warning, logging.Warningf, "W"},
				{logging.Error, logging.Errorf, "E"},
			} {
				t.Run(fmt.Sprintf("Can log to: %s", entry.L), func(t *ftt.Test) {
					entry.F(c, "Test logging %s", entry.L)
					matches := lre.FindAllStringSubmatch(normalizeLog(buf.String()), -1)
					assert.Loosely(t, len(matches), should.Equal(1))
					assert.Loosely(t, len(matches[0]), should.Equal(4))
					assert.Loosely(t, matches[0][1], should.Equal(entry.T))
					assert.Loosely(t, matches[0][2], should.Equal("gologger_test.go"))
					assert.Loosely(t, matches[0][3], should.Equal(fmt.Sprintf("Test logging %s", entry.L)))
				})
			}
		})

		t.Run(`With Fields installed in the Context`, func(t *ftt.Test) {
			c = logging.SetFields(c, logging.Fields{
				logging.ErrorKey: "An error!",
				"reason":         "test",
			})

			t.Run(`Should log Fields.`, func(t *ftt.Test) {
				logging.Infof(c, "Here is a %s", "log")
				matches := lre.FindAllStringSubmatch(normalizeLog(buf.String()), -1)
				assert.Loosely(t, len(matches), should.Equal(1))
				assert.Loosely(t, len(matches[0]), should.Equal(4))
				assert.Loosely(t, matches[0][1], should.Equal("I"))
				assert.Loosely(t, matches[0][2], should.Equal("gologger_test.go"))
				assert.Loosely(t, matches[0][3], should.Equal(
					`Here is a log                               {"error":"An error!", "reason":"test"}`))
			})

			t.Run(`Should log fields installed immediately`, func(t *ftt.Test) {
				logging.Fields{
					"foo":    "bar",
					"reason": "override",
				}.Infof(c, "Here is another %s", "log")

				matches := lre.FindAllStringSubmatch(normalizeLog(buf.String()), -1)
				assert.Loosely(t, len(matches), should.Equal(1))
				assert.Loosely(t, len(matches[0]), should.Equal(4))
				assert.Loosely(t, matches[0][1], should.Equal("I"))
				assert.Loosely(t, matches[0][2], should.Equal("gologger_test.go"))
				assert.Loosely(t, matches[0][3], should.Equal(
					`Here is another log                         {"error":"An error!", "foo":"bar", "reason":"override"}`))
			})

			t.Run(`Will not treat format args as format.`, func(t *ftt.Test) {
				logging.Infof(c, "%s", "Here is an %s")
				matches := lre.FindAllStringSubmatch(normalizeLog(buf.String()), -1)
				assert.Loosely(t, len(matches), should.Equal(1))
				assert.Loosely(t, len(matches[0]), should.Equal(4))
				assert.Loosely(t, matches[0][1], should.Equal("I"))
				assert.Loosely(t, matches[0][2], should.Equal("gologger_test.go"))
				assert.Loosely(t, matches[0][3], should.Equal(
					`Here is an %s                               {"error":"An error!", "reason":"test"}`))
			})
		})

		t.Run(`Will not log to Debug, as it's below level.`, func(t *ftt.Test) {
			logging.Debugf(c, "Hello!")
			assert.Loosely(t, buf.Len(), should.BeZero)
		})
	})
}
