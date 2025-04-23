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
	"fmt"
	"regexp"
	"strings"
	"testing"

	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/errors/errtag/stacktag"
	"go.chromium.org/luci/common/retry/transient"
	"go.chromium.org/luci/common/testing/ftt"
	"go.chromium.org/luci/common/testing/truth/assert"
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

type customTagValue struct {
	Field int
}

var customTag = errtag.Make("custom tag", customTagValue{})

func TestAnnotation(t *testing.T) {
	t.Parallel()

	ftt.Run("Test annotation struct", t, func(t *ftt.Test) {
		e := Annotate(New("bad thing"), "%d some error: %q", 20, "stringy").Err()

		t.Run("annotation can render itself for public usage", func(t *ftt.Test) {
			assert.Loosely(t, e.Error(), should.Equal(`20 some error: "stringy": bad thing`))
		})

		t.Run("annotation can render itself", func(t *ftt.Test) {
			e = transient.Tag.Apply(stacktag.Tag.ApplyValue(e, "I am a stack"))
			e = customTag.ApplyValue(e, customTagValue{42})
			lines := RenderStack(e, excludedPkgs...)

			assert.That(t, lines, should.Match(strings.Join([]string{
				`20 some error: "stringy": bad thing`,
				``,
				`errtags:`,
				`  "custom tag": errors.customTagValue{Field:42}`,
				`  "error is transient": true`,
				``,
				`I am a stack`,
			}, "\n")))
		})

		t.Run(`can render external errors with Unwrap and no inner error`, func(t *ftt.Test) {
			assert.That(t, RenderStack(emptyWrapper("hi")), should.Match("hi"))
		})

		t.Run(`can render external errors with Unwrap`, func(t *ftt.Test) {
			assert.That(t, RenderStack(fmt.Errorf("outer: %w", fmt.Errorf("inner"))), should.Match("outer: inner"))
		})
	})
}
