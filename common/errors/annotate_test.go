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

	. "github.com/smartystreets/goconvey/convey"
)

var (
	fixSkip        = regexp.MustCompile(`skipped \d+ frames`)
	fixNum         = regexp.MustCompile(`^#\d+`)
	fixTestingLine = regexp.MustCompile(`(testing/\w+.go|\.\/_testmain\.go):\d+`)
	fixSelfLN      = regexp.MustCompile(`(annotate_test\.go):\d+`)

	excludedPkgs = []string{
		`runtime`,
		`github.com/jtolds/gls`,
		`github.com/smartystreets/goconvey/convey`,
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

	Convey("Test annotation struct", t, func() {
		e := Annotate(New("bad thing"), "%d some error: %q", 20, "stringy").
			InternalReason("extra(%.3f)", 8.2).Err()
		ae := e.(*annotatedError)

		Convey("annotation can render itself for public usage", func() {
			So(ae.Error(), ShouldEqual, `20 some error: "stringy": bad thing`)
		})

		Convey("annotation can render itself for internal usage", func() {
			lines := RenderStack(e, excludedPkgs...)
			FixForTest(lines)

			So(lines, ShouldResemble, []string{
				`original error: bad thing`,
				``,
				`GOROUTINE LINE`,
				`#? go.chromium.org/luci/common/errors/annotate_test.go:XX - errors.TestAnnotation.func1()`,
				`  reason: 20 some error: "stringy"`,
				`  internal reason: extra(8.200)`,
				``,
				`... skipped SOME frames in pkg "github.com/smartystreets/goconvey/convey"...`,
				`... skipped SOME frames in pkg "github.com/jtolds/gls"...`,
				`... skipped SOME frames in pkg "github.com/smartystreets/goconvey/convey"...`,
				``,
				`#? go.chromium.org/luci/common/errors/annotate_test.go:XX - errors.TestAnnotation()`,
				`#? testing/testing.go:XXX - testing.tRunner()`,
				`... skipped SOME frames in pkg "runtime"...`,
			})
		})

		Convey("can render whole stack", func() {
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
				`    internal reason: extra(8.200)`,
				``,
				`... skipped SOME frames in pkg "github.com/smartystreets/goconvey/convey"...`,
				`... skipped SOME frames in pkg "github.com/jtolds/gls"...`,
				`... skipped SOME frames in pkg "github.com/smartystreets/goconvey/convey"...`,
				``,
				`#? go.chromium.org/luci/common/errors/annotate_test.go:XX - errors.TestAnnotation()`,
				`#? testing/testing.go:XXX - testing.tRunner()`,
				`... skipped SOME frames in pkg "runtime"...`,
			}
			So(lines, ShouldResemble, expectedLines)

			Convey("via Log", func() {
				ctx := memlogger.Use(context.Background())
				Log(ctx, e, excludedPkgs...)
				ml := logging.Get(ctx).(*memlogger.MemLogger)
				msgs := ml.Messages()
				So(msgs, ShouldHaveLength, 1)
				lines := strings.Split(msgs[0].Msg, "\n")
				FixForTest(lines)
				So(lines, ShouldResemble, expectedLines)
			})
			Convey("via Log in chunks", func() {
				maxLogEntrySize = 200
				ctx := memlogger.Use(context.Background())
				Log(ctx, e, excludedPkgs...)
				ml := logging.Get(ctx).(*memlogger.MemLogger)
				var lines []string
				for i, m := range ml.Messages() {
					So(len(m.Msg), ShouldBeLessThan, maxLogEntrySize)
					mLines := strings.Split(m.Msg, "\n")
					if i > 0 {
						So(mLines[0], ShouldEqual, "(continuation of error log)")
						mLines = mLines[1:]
					}
					lines = append(lines, mLines...)
				}
				FixForTest(lines)
				So(lines, ShouldResemble, expectedLines)
			})
		})

		Convey(`can render external errors with Unwrap and no inner error`, func() {
			So(RenderStack(emptyWrapper("hi")), ShouldResemble, []string{"hi"})
		})

		Convey(`can render external errors with Unwrap`, func() {
			So(RenderStack(fmt.Errorf("outer: %w", fmt.Errorf("inner"))), ShouldResemble, []string{"outer: inner"})
		})

		Convey(`can render external errors using Unwrap when Annotated`, func() {
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
				`... skipped SOME frames in pkg "github.com/smartystreets/goconvey/convey"...`,
				`... skipped SOME frames in pkg "github.com/jtolds/gls"...`,
				`... skipped SOME frames in pkg "github.com/smartystreets/goconvey/convey"...`,
				``,
				`#? go.chromium.org/luci/common/errors/annotate_test.go:XX - errors.TestAnnotation.func1()`,
				`... skipped SOME frames in pkg "github.com/smartystreets/goconvey/convey"...`,
				`... skipped SOME frames in pkg "github.com/jtolds/gls"...`,
				`... skipped SOME frames in pkg "github.com/smartystreets/goconvey/convey"...`,
				``,
				`#? go.chromium.org/luci/common/errors/annotate_test.go:XX - errors.TestAnnotation()`,
				`#? testing/testing.go:XXX - testing.tRunner()`,
				`... skipped SOME frames in pkg "runtime"...`,
			}
			So(lines, ShouldResemble, expectedLines)
		})
	})
}
