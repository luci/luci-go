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

package should

import (
	stderrors "errors"
	"fmt"
	"strings"
	"testing"

	"go.chromium.org/luci/common/errors"
	"go.chromium.org/luci/common/errors/errtag"
	"go.chromium.org/luci/common/errors/errtag/stacktag"
	"go.chromium.org/luci/common/testing/truth/failure"
)

type customErr struct{}

func (*customErr) Error() string { return "I am an error" }

var (
	tagA = errtag.Make("TagA", true)
	tagB = errtag.Make("TagB", "val")
)

func findFinding(f *failure.Summary, name string) *failure.Finding {
	if f == nil {
		return nil
	}
	for _, finding := range f.Findings {
		if finding.Name == name {
			return finding
		}
	}
	return nil
}

func TestErrLike(t *testing.T) {
	t.Parallel()

	t.Run("nil", shouldPass(ErrLike(nil)(nil)))

	err := errors.New("something")
	t.Run("substring", shouldPass(ErrLike("thing")(err)))

	t.Run("self", shouldPass(ErrLike(err)(err)))
	t.Run("derived", shouldPass(ErrLike(err)(fmt.Errorf("derived: %w", err))))

	t.Run("expect nil but isn't", shouldFail(ErrLike(nil)(err), "arguments", "nil"))

	t.Run("missing substring", shouldFail(ErrLike("wow")(err), "missing substring"))
	t.Run("different error", shouldFail(ErrLike(err)(errors.New("else")), "does not contain"))

	// So(err, ShouldBeNil) would get confused by this in goconvey.
	t.Run("nil custom error", shouldFail(ErrLike(nil)((*customErr)(nil)), "actual.Error()", "I am an error"))

	t.Run("bad type", func(t *testing.T) {
		mustPanicLike(t, "got `int`", func() {
			ErrLike(100)
		})
	})
}

func TestErrorFindings(t *testing.T) {
	t.Parallel()

	t.Run("simple untagged error", func(t *testing.T) {
		err := stderrors.New("something went wrong")
		summary := ErrLike(nil)(err)
		if summary == nil {
			t.Fatal("expected failure, got pass")
		}

		treeFinding := findFinding(summary, "errors.ParseTree")
		if treeFinding == nil {
			t.Fatal("expected 'errors.ParseTree' finding")
		}
		if val := strings.Join(treeFinding.Value, "\n"); val != "something went wrong" {
			t.Fatalf("unexpected errors.ParseTree value: %q", val)
		}

		if tagsFinding := findFinding(summary, "errtag.Collect"); tagsFinding != nil {
			t.Fatalf("expected no 'errtag.Collect' finding, got: %v", tagsFinding)
		}

		if stackFinding := findFinding(summary, "stacktag.Tag.Value"); stackFinding != nil {
			t.Fatalf("expected no 'stacktag.Tag.Value' finding, got: %v", stackFinding)
		}
	})

	t.Run("tagged error without stacktag", func(t *testing.T) {
		err := tagA.Apply(tagB.Apply(stderrors.New("bad input")))
		summary := ErrLike(nil)(err)
		if summary == nil {
			t.Fatal("expected failure, got pass")
		}

		treeFinding := findFinding(summary, "errors.ParseTree")
		if treeFinding == nil {
			t.Fatal("expected 'errors.ParseTree' finding")
		}
		expectedTree := "bad input\n ├ [TagA]\n └ [TagB]"
		if val := strings.Join(treeFinding.Value, "\n"); val != expectedTree {
			t.Fatalf("unexpected errors.ParseTree value: %q, expected: %q", val, expectedTree)
		}

		tagsFinding := findFinding(summary, "errtag.Collect")
		if tagsFinding == nil {
			t.Fatal("expected 'errtag.Collect' finding")
		}
		expectedTags := "\"TagA\": true\n\"TagB\": \"val\""
		if val := strings.Join(tagsFinding.Value, "\n"); val != expectedTags {
			t.Fatalf("unexpected errtag.Collect value: %q, expected: %q", val, expectedTags)
		}

		if stackFinding := findFinding(summary, "stacktag.Tag.Value"); stackFinding != nil {
			t.Fatalf("expected no 'stacktag.Tag.Value' finding, got: %v", stackFinding)
		}
	})

	t.Run("error with stacktag only", func(t *testing.T) {
		err := stacktag.Capture(stderrors.New("bad input"), 0)
		summary := ErrLike(nil)(err)
		if summary == nil {
			t.Fatal("expected failure, got pass")
		}

		treeFinding := findFinding(summary, "errors.ParseTree")
		if treeFinding == nil {
			t.Fatal("expected 'errors.ParseTree' finding")
		}
		if val := strings.Join(treeFinding.Value, "\n"); !strings.Contains(val, "[stacktag.Capture]") {
			t.Fatalf("expected '[stacktag.Capture]' in errors.ParseTree, got: %q", val)
		}

		// stacktag should be excluded from errtag.Collect
		if tagsFinding := findFinding(summary, "errtag.Collect"); tagsFinding != nil {
			t.Fatalf("expected no 'errtag.Collect' finding when only stacktag is present, got: %v", tagsFinding)
		}

		stackFinding := findFinding(summary, "stacktag.Tag.Value")
		if stackFinding == nil {
			t.Fatal("expected 'stacktag.Tag.Value' finding")
		}
		if val := strings.Join(stackFinding.Value, "\n"); !strings.Contains(val, "stacktag.Capture") && !strings.Contains(val, "TestErrorFindings") {
			t.Fatalf("expected stack trace in 'stacktag.Tag.Value' finding, got: %q", val)
		}
		if stackFinding.Level != failure.FindingLogLevel_Warn {
			t.Fatalf("expected Warn level on stack finding, got: %v", stackFinding.Level)
		}
	})

	t.Run("error with tags and stacktag", func(t *testing.T) {
		err := stacktag.Capture(tagA.Apply(stderrors.New("bad input")), 0)
		summary := ErrLike(nil)(err)
		if summary == nil {
			t.Fatal("expected failure, got pass")
		}

		treeFinding := findFinding(summary, "errors.ParseTree")
		if treeFinding == nil {
			t.Fatal("expected 'errors.ParseTree' finding")
		}
		treeVal := strings.Join(treeFinding.Value, "\n")
		if !strings.Contains(treeVal, "[stacktag.Capture]") || !strings.Contains(treeVal, "[TagA]") {
			t.Fatalf("expected both tags in errors.ParseTree, got: %q", treeVal)
		}

		tagsFinding := findFinding(summary, "errtag.Collect")
		if tagsFinding == nil {
			t.Fatal("expected 'errtag.Collect' finding")
		}
		if val := strings.Join(tagsFinding.Value, "\n"); val != "\"TagA\": true" {
			t.Fatalf("unexpected errtag.Collect value (should not contain stacktag): %q", val)
		}

		stackFinding := findFinding(summary, "stacktag.Tag.Value")
		if stackFinding == nil {
			t.Fatal("expected 'stacktag.Tag.Value' finding")
		}
	})

	t.Run("errors.New from luci common/errors includes stacktag", func(t *testing.T) {
		err := errors.New("luci error")
		summary := ErrLike(nil)(err)
		if summary == nil {
			t.Fatal("expected failure, got pass")
		}

		treeFinding := findFinding(summary, "errors.ParseTree")
		if treeFinding == nil {
			t.Fatal("expected 'errors.ParseTree' finding")
		}
		if val := strings.Join(treeFinding.Value, "\n"); !strings.Contains(val, "[stacktag.Capture]") {
			t.Fatalf("expected '[stacktag.Capture]' in errors.ParseTree for errors.New, got: %q", val)
		}

		if tagsFinding := findFinding(summary, "errtag.Collect"); tagsFinding != nil {
			t.Fatalf("expected no 'errtag.Collect' finding for plain errors.New, got: %v", tagsFinding)
		}

		if stackFinding := findFinding(summary, "stacktag.Tag.Value"); stackFinding == nil {
			t.Fatal("expected 'stacktag.Tag.Value' finding for errors.New")
		}
	})

	t.Run("ErrLikeString failure findings", func(t *testing.T) {
		err := tagA.Apply(errors.New("hello world"))
		summary := ErrLikeString("nonexistent")(err)
		if summary == nil {
			t.Fatal("expected failure, got pass")
		}

		if tree := findFinding(summary, "errors.ParseTree"); tree == nil {
			t.Fatal("expected 'errors.ParseTree' finding in ErrLikeString failure")
		}
		if tags := findFinding(summary, "errtag.Collect"); tags == nil {
			t.Fatal("expected 'errtag.Collect' finding in ErrLikeString failure")
		}
	})

	t.Run("UnwrapToErrStringLike failure findings", func(t *testing.T) {
		err := tagA.Apply(errors.New("hello world"))
		summary := UnwrapToErrStringLike("nonexistent")(err)
		if summary == nil {
			t.Fatal("expected failure, got pass")
		}

		if tree := findFinding(summary, "errors.ParseTree"); tree == nil {
			t.Fatal("expected 'errors.ParseTree' finding in UnwrapToErrStringLike failure")
		}
		if tags := findFinding(summary, "errtag.Collect"); tags == nil {
			t.Fatal("expected 'errtag.Collect' finding in UnwrapToErrStringLike failure")
		}
	})

	t.Run("ErrLikeError mismatch failure findings", func(t *testing.T) {
		target := errors.New("expected error")
		err := tagA.Apply(errors.New("actual error"))
		summary := ErrLikeError(target)(err)
		if summary == nil {
			t.Fatal("expected failure, got pass")
		}

		if tree := findFinding(summary, "errors.ParseTree"); tree == nil {
			t.Fatal("expected 'errors.ParseTree' finding in ErrLikeError failure")
		}
		if tags := findFinding(summary, "errtag.Collect"); tags == nil {
			t.Fatal("expected 'errtag.Collect' finding in ErrLikeError failure")
		}
	})
}
