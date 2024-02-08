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

package structuraldiff

import (
	"bytes"
	"reflect"

	"github.com/kylelemons/godebug/pretty"
	"github.com/sergi/go-diff/diffmatchpatch"
)

// DebugDump is an extremely abstraction-breaking function that will print a Go Value.
//
// It prints zero values and unexported field, but does NOT ignore user-defined methods for pretty-printing.
// It SHOULD ignore user-defined methods for pretty-printing, but this causes issues with infinite recursion
// when printing protos.
func DebugDump(val any) string {
	config := &pretty.Config{
		Compact:           false,
		Diffable:          true,
		IncludeUnexported: true,
		// I would prefer this to be false. I really would, but disabling this feature
		// causes an infinite loop when printing stuff.
		//
		// Long-term, I'm going to write my own thing.
		PrintStringers:      true,
		PrintTextMarshalers: false,
		SkipZeroFields:      false,
		ShortList:           30,
		Formatter:           nil,
		TrackCycles:         true,
	}
	return config.Sprint(val)
}

// A Result is either a list of diffs
type Result struct {
	diffs   []diffmatchpatch.Diff
	message string
}

// String prints a result as a single string with no coloration.
//
// Sometimes you want colors, like when printing to a terminal, but by default you don't.
// Regardless, stringifying with colors should be opt-in and should always be on.
func (result *Result) String() string {
	if result == nil {
		return ""
	}
	if result.message != "" {
		return result.message
	}
	return diffToString(result.diffs)
}

const sorry = `No difference between arguments but they are not equal.`

// DebugCompare compares two values of the same type and produces a structural diff between them.
//
// If we can't detect a difference between the two structures, return an apologetic message but
// NOT a nil result. That way we can compare the Result to nil to see if there were any problems.
func DebugCompare[T any](left T, right T) *Result {
	if reflect.DeepEqual(left, right) {
		return nil
	}
	patcher := diffmatchpatch.New()
	out := patcher.DiffMain(DebugDump(left), DebugDump(right), true)
	if len(out) == 0 {
		return &Result{
			message: sorry,
		}
	}
	return &Result{
		diffs: out,
	}
}

// diffToString writes a sequence of diffs as a string without formatting.
func diffToString(diffs []diffmatchpatch.Diff) string {
	var buf bytes.Buffer
	for _, diff := range diffs {
		if diff.Type == diffmatchpatch.DiffEqual {
			continue
		}
		_, _ = buf.WriteString(diff.Type.String())
		_, _ = buf.WriteString(" ")
		_, _ = buf.WriteString(diff.Text)
		_, _ = buf.WriteString("\n")
	}
	return buf.String()
}
