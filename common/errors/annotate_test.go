// Copyright 2016 The LUCI Authors. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package errors

import (
	"regexp"
	"strings"
	"testing"

	. "github.com/smartystreets/goconvey/convey"
)

var (
	fixSkip        = regexp.MustCompile(`skipped \d+ frames`)
	fixNum         = regexp.MustCompile(`^#\d+`)
	fixTestingLine = regexp.MustCompile(`(testing/\w+.go):\d+`)
)

func FixForTest(lines []string) []string {
	for i, l := range lines {
		switch {
		case strings.HasPrefix(l, "goroutine"):
			l = "GOROUTINE LINE"
		case strings.HasPrefix(l, "... skipped"):
			l = fixSkip.ReplaceAllLiteralString(l, "skipped SOME frames")
		}
		l = fixNum.ReplaceAllLiteralString(l, "#?")
		if strings.HasPrefix(l, "#? testing/") {
			l = fixTestingLine.ReplaceAllString(l, "$1:XXX")
		}
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
			lines := RenderStack(e, `runtime`, `github.com/jtolds/gls`,
				`github.com/smartystreets/goconvey/convey`)
			FixForTest(lines)

			So(lines, ShouldResemble, []string{
				`original error: bad thing`,
				``,
				`GOROUTINE LINE`,
				`#? github.com/luci/luci-go/common/errors/annotate_test.go:43 - errors.TestAnnotation.func1()`,
				`  reason: 20 some error: "stringy"`,
				`  internal reason: extra(8.200)`,
				``,
				`... skipped SOME frames in pkg "github.com/smartystreets/goconvey/convey"...`,
				`... skipped SOME frames in pkg "github.com/jtolds/gls"...`,
				`... skipped SOME frames in pkg "github.com/smartystreets/goconvey/convey"...`,
				``,
				`#? github.com/luci/luci-go/common/errors/annotate_test.go:99 - errors.TestAnnotation()`,
				`#? testing/testing.go:XXX - testing.tRunner()`,
				`... skipped SOME frames in pkg "runtime"...`,
			})
		})

		Convey("can render whole stack", func() {
			e = Annotate(e, "outer frame %s", "outer").Err()
			lines := RenderStack(e, `runtime`, `github.com/jtolds/gls`,
				`github.com/smartystreets/goconvey/convey`)
			FixForTest(lines)

			So(lines, ShouldResemble, []string{
				`original error: bad thing`,
				``,
				`GOROUTINE LINE`,
				`#? github.com/luci/luci-go/common/errors/annotate_test.go:43 - errors.TestAnnotation.func1()`,
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
				`#? github.com/luci/luci-go/common/errors/annotate_test.go:99 - errors.TestAnnotation()`,
				`#? testing/testing.go:XXX - testing.tRunner()`,
				`... skipped SOME frames in pkg "runtime"...`,
			})
		})
	})
}
