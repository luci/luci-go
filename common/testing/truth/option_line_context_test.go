// Copyright 2024 The LUCI Authors.
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

package truth

import (
	"path/filepath"
	"testing"

	"go.chromium.org/luci/common/testing/truth/comparison"
	"go.chromium.org/luci/common/testing/truth/failure"
	"go.chromium.org/luci/common/testing/typed"
)

func TestLineContext(t *testing.T) {
	t.Parallel()

	mkFailure := func() *failure.Summary {
		return comparison.NewSummaryBuilder("line_context_test/TestLineContext").Summary
	}

	t.Run("non-nil", func(t *testing.T) {
		opt := LineContext(0).(summaryModifier) // Gets .../option_line_context_test.go:34
		failure := mkFailure()
		opt(failure)
		ctx := failure.SourceContext
		if len(ctx) != 1 {
			t.Fatalf("failure.SourceContext len wrong: %d", len(ctx))
		}
		atCtx := ctx[0]
		if diff := typed.Diff(atCtx.Name, "at"); diff != "" {
			t.Fatal(diff)
		}
		if len(atCtx.Frames) != 1 {
			t.Fatalf("failure.SourceContext[0].Frames len wrong: %d", len(atCtx.Frames))
		}

		// NOTE - These depend on the source code location of the LineContext
		// call at the top of this function. If you edit this test file, the line
		// number will change, which is expected.
		frame := atCtx.Frames[0]
		if diff := typed.Diff(filepath.Base(frame.Filename), "option_line_context_test.go"); diff != "" {
			t.Fatalf("unexpected filename: %s", diff)
		}
		if diff := typed.Diff(frame.Lineno, 34); diff != "" {
			t.Fatalf("unexpected line number: %s", diff)
		}
	})

	t.Run("helper", func(t *testing.T) {
		opt := func() summaryModifier {
			return LineContext(1).(summaryModifier)
		}() // Gets .../option_line_context_test.go:64
		failure := mkFailure()
		opt(failure)
		ctx := failure.SourceContext
		if len(ctx) != 1 {
			t.Fatalf("failure.SourceContext len wrong: %d", len(ctx))
		}
		atCtx := ctx[0]
		if diff := typed.Diff(atCtx.Name, "at"); diff != "" {
			t.Fatal(diff)
		}
		if len(atCtx.Frames) != 1 {
			t.Fatalf("failure.SourceContext[0].Frames len wrong: %d", len(atCtx.Frames))
		}

		// NOTE - These depend on the source code location of the LineContext
		// call at the top of this function. If you edit this test file, the line
		// number will change, which is expected.
		frame := atCtx.Frames[0]
		if diff := typed.Diff(filepath.Base(frame.Filename), "option_line_context_test.go"); diff != "" {
			t.Fatalf("unexpected filename: %s", diff)
		}
		if diff := typed.Diff(frame.Lineno, 64); diff != "" {
			t.Fatalf("unexpected line number: %s", diff)
		}
	})

}
