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

package errors

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"go.chromium.org/luci/common/logging"
	"go.chromium.org/luci/common/logging/memlogger"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth"
	"go.chromium.org/luci/common/testing/truth/assert"
	"go.chromium.org/luci/common/testing/truth/check"
	"go.chromium.org/luci/common/testing/truth/should"
)

var (
	fixSkip        = regexp.MustCompile(`skipped \d+ frames`)
	fixNum         = regexp.MustCompile(`^#\d+`)
	fixTestingLine = regexp.MustCompile(`(testing/\w+.go|\.\/_testmain\.go):\d+`)
	fixSelfLN      = regexp.MustCompile(`(annotate_test\.go):\d+`)

	excludedPkgs = []string{
		`runtime`,
		`go.chromium.org/luci/common/testing/ftt`,
	}
)

type emptyWrapper string

func (e emptyWrapper) Error() string {
	return string(e)
}

func (e emptyWrapper) Unwrap() error {
	return nil
}

func FixForTest(lines []string) []string {
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "goroutine"):
			l = "GOROUTINE LINE"
		case strings.HasPrefix(l, "... skipped"):
			l = fixSkip.ReplaceAllLiteralString(l, "skipped SOME frames")
		}
		l = fixNum.ReplaceAllLiteralString(l, "#?")
		if strings.HasPrefix(l, "#? testing/") || strings.HasPrefix(l, "#? ./_testmain.go") {
			l = fixTestingLine.ReplaceAllString(l, "$1:XXX")
		}
		l = fixSelfLN.ReplaceAllString(l, "$1:XX")
		lines[i] = l
	}
	return lines
}

func TestAnnotation(t *testing.T) {
	t.Parallel()

	ftt.Run("Test annotation struct", t, func(t *ftt.Test) {
		e := Annotate(New("bad thing"), "%d some error: %q", 20, "stringy").Err()
		ae := e.(*annotatedError)

		t.Run("annotation can render itself for public usage", func(t *ftt.Test) {
			assert.Loosely(t, ae.Error(), should.Equal(`20 some error: "stringy": bad thing`))
		})

		t.Run("annotation can render itself", func(t *ftt.Test) {
			lines := RenderStack(e, excludedPkgs...)
			FixForTest(lines)

			assert.Loosely(t, lines, should.Match([]string{
				`original error: bad thing`,
				``,
				`GOROUTINE LINE`,
				`#? go.chromium.org/luci/common/errors/annotate_test.go:XX - errors.TestAnnotation.func1()`,
				`  reason: 20 some error: "stringy"`,
				``,
				`... skipped SOME frames in pkg "go.chromium.org/luci/common/testing/ftt"...`,
				``,
				`#? testing/testing.go:XXX - testing.tRunner()`,
				`... skipped SOME frames in pkg "runtime"...`,
			}))
		})

		t.Run("can render whole stack", func(t *ftt.Test) {
			e = Annotate(e, "outer frame %s", "outer").Err()
			lines := RenderStack(e, excludedPkgs...)
			FixForTest(lines)

			expectedLines := []string{
				`original error: bad thing`,
				``,
				`GOROUTINE LINE`,
				`#? go.chromium.org/luci/common/errors/annotate_test.go:XX - errors.TestAnnotation.func1()`,
				`  annotation #0:`,
				`    reason: outer frame outer`,
				`  annotation #1:`,
				`    reason: 20 some error: "stringy"`,
				``,
				`... skipped SOME frames in pkg "go.chromium.org/luci/common/testing/ftt"...`,
				``,
				`#? testing/testing.go:XXX - testing.tRunner()`,
				`... skipped SOME frames in pkg "runtime"...`,
			}
			assert.Loosely(t, lines, should.Match(expectedLines))

			t.Run("via Log", func(t *ftt.Test) {
				ctx := memlogger.Use(context.Background())
				Log(ctx, e, excludedPkgs...)
				ml := logging.Get(ctx).(*memlogger.MemLogger)
				msgs := ml.Messages()
				assert.Loosely(t, msgs, should.HaveLength(1))
				lines := strings.Split(msgs[0].Msg+"\n\n"+msgs[0].StackTrace.Textual, "\n")
				FixForTest(lines)
				assert.Loosely(t, lines, should.Match(expectedLines))
			})
		})

		t.Run(`can render external errors with Unwrap and no inner error`, func(t *ftt.Test) {
			assert.Loosely(t, RenderStack(emptyWrapper("hi")), should.Match([]string{"hi"}))
		})

		t.Run(`can render external errors with Unwrap`, func(t *ftt.Test) {
			assert.Loosely(t, RenderStack(fmt.Errorf("outer: %w", fmt.Errorf("inner"))), should.Match([]string{"outer: inner"}))
		})

		t.Run(`can render external errors using Unwrap when Annotated`, func(t *ftt.Test) {
			e := Annotate(fmt.Errorf("outer: %w", fmt.Errorf("inner")), "annotate").Err()
			lines := RenderStack(e, excludedPkgs...)
			FixForTest(lines)

			expectedLines := []string{
				`original error: outer: inner`,
				``,
				`GOROUTINE LINE`,
				`#? go.chromium.org/luci/common/errors/annotate_test.go:XX - errors.TestAnnotation.func1.6()`,
				`  reason: annotate`,
				``,
				`... skipped SOME frames in pkg "go.chromium.org/luci/common/testing/ftt"...`,
				``,
				`#? go.chromium.org/luci/common/errors/annotate_test.go:XX - errors.TestAnnotation.func1()`,
				`... skipped SOME frames in pkg "go.chromium.org/luci/common/testing/ftt"...`,
				``,
				`#? testing/testing.go:XXX - testing.tRunner()`,
				`... skipped SOME frames in pkg "runtime"...`,
			}
			assert.Loosely(t, lines, should.Match(expectedLines))
		})
	})
}

func inner(fn func(chan<- error)) error {
	if fn == nil {
		return Reason("hello").Err()
	}

	errCh := make(chan error)
	go fn(errCh)
	return Annotate(<-errCh, "wrapped").Err()
}

func outer(fn func(chan<- error)) error {
	return inner(fn)
}

func TestAnnotateGoStack(t *testing.T) {
	err := outer(func(c chan<- error) {
		c <- outer(nil)
	})

	stack := RenderGoStack(err, false, "runtime")
	lines := strings.Split(stack, "\n")
	assert.Loosely(t, lines, should.HaveLength(16))

	toFind := []string{
		"inner()",
		"outer()",
		"TestAnnotateGoStack.func1()",
		"inner()",
		"outer()",
		"TestAnnotateGoStack()",
		"testing.tRunner()",
	}
	for _, line := range lines {
		if strings.Contains(line, toFind[0]) {
			toFind = toFind[1:]
		}

		if len(toFind) == 0 {
			break
		}
	}

	if !check.Loosely(t, toFind, should.BeEmpty, truth.Explain(
		"Stack trace order seems to be incorrect.")) {
		t.Log(stack)
	}
}
